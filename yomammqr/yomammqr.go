// Package yomammqr integrates Yoma Bank MMQR: QR payments, status checks and callbacks.
//
// Access tokens are cached in a TokenCache; pass a shared one so they survive between processes.
package yomammqr

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments"
	"github.com/laranex/go-myanmar-payments/internal/transport"
	"github.com/laranex/go-myanmar-payments/internal/values"
)

// QRLifetime is how long a generated QR stays payable.
const QRLifetime = 120 * time.Second

var statuses = map[string]myanmarpayments.PaymentStatus{
	"SUCCESS": myanmarpayments.StatusSuccessful,
	"PENDING": myanmarpayments.StatusPending,
	"FAILED":  myanmarpayments.StatusFailed,
	"FAIL":    myanmarpayments.StatusFailed,
}

// Gateway talks to Yoma MMQR.
type Gateway struct {
	config    Config
	transport *transport.Client
	cache     myanmarpayments.TokenCache
	now       func() time.Time
}

// New returns a Gateway. A nil client uses myanmarpayments.DefaultHTTPClient and a nil cache
// uses an in-memory cache.
func New(config Config, client myanmarpayments.HTTPDoer, cache myanmarpayments.TokenCache) (*Gateway, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	if cache == nil {
		cache = myanmarpayments.NewMemoryTokenCache()
	}
	return &Gateway{config: config, transport: transport.New(client), cache: cache, now: time.Now}, nil
}

// Config returns the configuration in use.
func (g *Gateway) Config() Config { return g.config }

// Initiate checks the order out with Yoma and generates its first QR. Call it once per order.
func (g *Gateway) Initiate(ctx context.Context, data PaymentData) (*myanmarpayments.QrPayment, error) {
	if err := data.Validate(); err != nil {
		return nil, err
	}

	body, err := g.call(ctx, "payment/checkout", map[string]any{
		"merchantId":  g.config.MerchantID,
		"orderNumber": data.OrderID,
		"amount":      data.Amount.String(),
		"description": data.Description,
	}, nil)
	if err != nil {
		return nil, err
	}
	if confirmed, _ := body["checkOutStatus"].(bool); !confirmed {
		return nil, &myanmarpayments.APIError{Message: "Yoma MMQR did not confirm the checkout.", Raw: body}
	}

	return g.RenewQR(ctx, data.OrderID)
}

// RenewQR generates a new QR for an order that is already checked out, e.g. after the
// previous one expired. The previous QR's reference stops working.
func (g *Gateway) RenewQR(ctx context.Context, orderID string) (*myanmarpayments.QrPayment, error) {
	body, err := g.call(ctx, "qr/generate", map[string]any{"merchantId": g.config.MerchantID, "orderNumber": orderID}, nil)
	if err != nil {
		return nil, err
	}

	qrString, reference := values.Get(body, "qrString"), values.Get(body, "refLabel")
	if qrString == "" || reference == "" {
		return nil, &myanmarpayments.APIError{Message: "Yoma MMQR did not return a QR.", Raw: body}
	}

	return &myanmarpayments.QrPayment{
		OrderID:   orderID,
		QRImage:   qrString,
		ExpiresAt: g.now().Add(QRLifetime),
		Reference: reference,
		Raw:       body,
	}, nil
}

// Status checks a QR's payment status by its reference (QrPayment.Reference).
// An expired QR reports StatusExpired.
func (g *Gateway) Status(ctx context.Context, reference string) (*myanmarpayments.PaymentStatusResult, error) {
	body, err := g.call(ctx, "payment/check-status", map[string]any{"merchantId": g.config.MerchantID, "refLabel": reference}, []string{"QR EXPIRED"})
	if err != nil {
		return nil, err
	}

	if values.Get(body, "errorCode") == "QR EXPIRED" {
		return &myanmarpayments.PaymentStatusResult{Status: myanmarpayments.StatusExpired, GatewayStatus: "QR EXPIRED", GatewayReference: reference, Raw: body}, nil
	}

	paymentStatus := values.Trimmed(body, "paymentStatus")
	result := &myanmarpayments.PaymentStatusResult{
		Status:           myanmarpayments.ResolveStatus(statuses, strings.ToUpper(paymentStatus)),
		GatewayStatus:    paymentStatus,
		GatewayReference: reference,
		Raw:              body,
	}
	if ref := values.Get(body, "refLabel"); ref != "" {
		result.GatewayReference = ref
	}
	return result, nil
}

// HandleCallback verifies Yoma's payment callback: HMAC-SHA256 of "orderNumber=..&status=.."
// keyed with the order number followed by the webhook hash key.
func (g *Gateway) HandleCallback(request *myanmarpayments.CallbackRequest) (*myanmarpayments.PaymentCallback, error) {
	payload := request.ParsedBody()

	if g.config.WebhookSecret != "" && !hmac.Equal([]byte(g.config.WebhookSecret), []byte(request.HeaderValue("X-Webhook-Secret"))) {
		return nil, &myanmarpayments.SignatureVerificationError{Message: "Yoma MMQR callback has a missing or wrong X-Webhook-Secret header.", Raw: payload}
	}

	orderNumber := values.Get(payload, "orderNumber")
	status := values.Trimmed(payload, "status")
	mac := hmac.New(sha256.New, []byte(orderNumber+g.config.WebhookHashKey))
	mac.Write([]byte("orderNumber=" + orderNumber + "&status=" + status))
	expected := hex.EncodeToString(mac.Sum(nil))

	if orderNumber == "" || !hmac.Equal([]byte(expected), []byte(strings.ToLower(values.Get(payload, "hashValue")))) {
		return nil, &myanmarpayments.SignatureVerificationError{Message: "Yoma MMQR callback hash verification failed.", Raw: payload}
	}

	return &myanmarpayments.PaymentCallback{
		OrderID:         orderNumber,
		Status:          myanmarpayments.ResolveStatus(statuses, strings.ToUpper(status)),
		GatewayStatus:   status,
		Raw:             payload,
		Acknowledgement: myanmarpayments.DefaultAcknowledgement(),
	}, nil
}

// ForgetToken drops the cached access token, e.g. after rotating the client secret.
func (g *Gateway) ForgetToken() { g.cache.Delete(g.tokenCacheKey()) }

func (g *Gateway) call(ctx context.Context, path string, data map[string]any, allowErrors []string) (map[string]any, error) {
	endpoint := fmt.Sprintf("%s/payment-gateway/%s/api/%s", g.config.ResolvedBaseURL(), g.config.ResolvedAPIVersion(), path)

	send := func() (transport.Response, error) {
		token, err := g.token(ctx)
		if err != nil {
			return transport.Response{}, err
		}
		return g.transport.PostJSON(ctx, endpoint, data, map[string]string{"Authorization": "Bearer " + token})
	}

	response, err := send()
	if err != nil {
		return nil, err
	}
	if response.Status == http.StatusUnauthorized {
		g.ForgetToken()
		if response, err = send(); err != nil {
			return nil, err
		}
	}

	body := response.JSON()
	errorCode := values.Get(body, "errorCode")
	allowed := false
	for _, code := range allowErrors {
		allowed = allowed || code == errorCode
	}
	if !response.Successful() || (errorCode != "" && !allowed) {
		return nil, apiError(path, response.Status, body, errorCode)
	}
	return body, nil
}

func (g *Gateway) token(ctx context.Context) (string, error) {
	if token, ok := g.cache.Get(g.tokenCacheKey()); ok && token != "" {
		return token, nil
	}

	credentials := base64.StdEncoding.EncodeToString([]byte(g.config.ClientID + ":" + g.config.ClientSecret))
	response, err := g.transport.PostForm(ctx, g.config.ResolvedBaseURL()+"/token", url.Values{"grant_type": {"client_credentials"}}, map[string]string{"Authorization": "Basic " + credentials})
	if err != nil {
		return "", err
	}

	body := response.JSON()
	token := values.Get(body, "access_token")
	if !response.Successful() || token == "" {
		return "", apiError("token", response.Status, body, values.Get(body, "error"))
	}

	expiresIn, err := strconv.Atoi(values.Get(body, "expires_in"))
	if err != nil || expiresIn <= 0 {
		expiresIn = 3600
	}
	ttl := time.Duration(expiresIn-60) * time.Second
	if ttl < time.Minute {
		ttl = time.Minute
	}
	g.cache.Set(g.tokenCacheKey(), token, ttl)

	return token, nil
}

func (g *Gateway) tokenCacheKey() string {
	sum := sha256.Sum256([]byte(g.config.ResolvedBaseURL() + "|" + g.config.ClientID))
	return "go-myanmar-payments.yoma-mmqr.token." + hex.EncodeToString(sum[:])
}

func apiError(endpoint string, status int, body map[string]any, errorCode string) error {
	message := values.Get(body, "errorDescription")
	if message == "" {
		message = values.Get(body, "error_description")
	}
	text := fmt.Sprintf("Yoma MMQR %s failed with HTTP %d.", endpoint, status)
	if errorCode != "" {
		text = fmt.Sprintf("Yoma MMQR %s failed: [%s] %s", endpoint, errorCode, message)
	}
	return &myanmarpayments.APIError{Message: text, GatewayCode: errorCode, GatewayMessage: message, HTTPStatus: status, Raw: body}
}
