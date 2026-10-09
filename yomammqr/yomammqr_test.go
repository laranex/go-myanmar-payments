package yomammqr

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/testutil"
)

var now = time.Unix(1791393600, 0)

func newGateway(t *testing.T, server *testutil.Server, cache myanmarpayments.TokenCache, mutate ...func(*Config)) *Gateway {
	t.Helper()
	config := Config{MerchantID: "M001", ClientID: "client", ClientSecret: "secret", WebhookHashKey: "hash-key"}
	if server != nil {
		config.BaseURL = server.URL
	}
	for _, m := range mutate {
		m(&config)
	}
	gateway, err := New(config, nil, cache)
	if err != nil {
		t.Fatal(err)
	}
	gateway.now = func() time.Time { return now }
	return gateway
}

func token(value string) testutil.Reply {
	return testutil.Reply{Body: map[string]any{"access_token": value, "scope": "default", "token_type": "Bearer", "expires_in": 28800}}
}

func hmacHex(message, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestInitiateChecksOutGeneratesTheQRAndReturnsItsExpiry(t *testing.T) {
	server := testutil.NewServer(t,
		token("token-1"),
		testutil.Reply{Body: map[string]any{"checkOutStatus": true, "errorCode": nil, "errorDescription": nil}},
		testutil.Reply{Body: map[string]any{"refLabel": "100000083331", "qrString": "iVBORw0KGgo=", "errorCode": nil}},
	)

	qr, err := newGateway(t, server, nil).Initiate(context.Background(), PaymentData{OrderID: "ORD-2026-001", Amount: myanmarpayments.Kyat(1000), Description: "Order 1"})
	if err != nil {
		t.Fatal(err)
	}

	tokenRequest, checkout, generate := server.Requests[0], server.Requests[1], server.Requests[2]
	form, _ := url.ParseQuery(string(tokenRequest.Body))
	if tokenRequest.Path != "/token" || tokenRequest.Header.Get("Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("client:secret")) || form.Get("grant_type") != "client_credentials" {
		t.Fatalf("unexpected token request %+v", tokenRequest)
	}
	if checkout.Path != "/payment-gateway/v1rc/api/payment/checkout" || checkout.Header.Get("Authorization") != "Bearer token-1" {
		t.Fatalf("unexpected checkout request %+v", checkout)
	}
	body := checkout.JSON(t)
	if body["merchantId"] != "M001" || body["orderNumber"] != "ORD-2026-001" || body["amount"] != "1000" || body["description"] != "Order 1" {
		t.Fatalf("unexpected checkout body %v", body)
	}
	if generate.JSON(t)["orderNumber"] != "ORD-2026-001" {
		t.Fatal("unexpected generate body")
	}
	if qr.QRImage != "iVBORw0KGgo=" || qr.QRString != "" || qr.Reference != "100000083331" || !qr.ExpiresAt.Equal(now.Add(120*time.Second)) {
		t.Fatalf("unexpected qr %+v", qr)
	}
	if qr.QRImageDataURI("") != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatal("unexpected data URI")
	}
}

func TestTokenIsReusedAcrossCalls(t *testing.T) {
	server := testutil.NewServer(t,
		token("token-1"),
		testutil.Reply{Body: map[string]any{"refLabel": "1", "qrString": "a", "errorCode": nil}},
		testutil.Reply{Body: map[string]any{"refLabel": "1", "paymentStatus": "PENDING", "errorCode": nil}},
	)
	cache := myanmarpayments.NewMemoryTokenCache()

	if _, err := newGateway(t, server, cache).RenewQR(context.Background(), "ORD-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := newGateway(t, server, cache).Status(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	if len(server.Requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(server.Requests))
	}
}

func TestTokenIsRefreshedOnceOn401(t *testing.T) {
	server := testutil.NewServer(t,
		token("old"),
		testutil.Reply{Status: http.StatusUnauthorized, Body: map[string]any{"message": "Unauthorized"}},
		token("new"),
		testutil.Reply{Body: map[string]any{"refLabel": "1", "paymentStatus": "SUCCESS", "errorCode": nil}},
	)

	result, err := newGateway(t, server, nil).Status(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if server.Last(t).Header.Get("Authorization") != "Bearer new" || !result.IsSuccessful() {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestBusinessErrorWithHTTP200IsAnAPIError(t *testing.T) {
	server := testutil.NewServer(t, token("t"), testutil.Reply{Body: map[string]any{"checkOutStatus": false, "errorCode": "PAYMENT ALREADY EXISTS", "errorDescription": "Payment already exists"}})

	_, err := newGateway(t, server, nil).Initiate(context.Background(), PaymentData{OrderID: "ORD-1", Amount: myanmarpayments.Kyat(1000), Description: "Order 1"})
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) || apiErr.GatewayCode != "PAYMENT ALREADY EXISTS" || apiErr.HTTPStatus != 200 {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestStatusResultsAreMapped(t *testing.T) {
	cases := map[string]struct {
		body map[string]any
		want myanmarpayments.PaymentStatus
	}{
		"success": {map[string]any{"paymentStatus": "SUCCESS", "errorCode": nil}, myanmarpayments.StatusSuccessful},
		"pending": {map[string]any{"paymentStatus": "PENDING", "errorCode": nil}, myanmarpayments.StatusPending},
		"failed":  {map[string]any{"paymentStatus": "FAILED", "errorCode": nil}, myanmarpayments.StatusFailed},
		"expired": {map[string]any{"paymentStatus": nil, "errorCode": "QR EXPIRED", "errorDescription": "Qr has been expired."}, myanmarpayments.StatusExpired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tc.body["refLabel"] = "1"
			server := testutil.NewServer(t, token("t"), testutil.Reply{Body: tc.body})
			result, err := newGateway(t, server, nil).Status(context.Background(), "1")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != tc.want {
				t.Fatalf("got %s want %s", result.Status, tc.want)
			}
		})
	}
}

func callback(t *testing.T, payload map[string]any, header http.Header) *myanmarpayments.CallbackRequest {
	t.Helper()
	request, err := myanmarpayments.NewCallbackRequestFromJSON(payload, header)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestCallbackHashIsKeyedWithTheOrderNumberAndHashKey(t *testing.T) {
	for status, want := range map[string]myanmarpayments.PaymentStatus{"success": myanmarpayments.StatusSuccessful, "fail": myanmarpayments.StatusFailed} {
		t.Run(status, func(t *testing.T) {
			hash := hmacHex("orderNumber=ORD-2026-001235&status="+status, "ORD-2026-001235hash-key")
			result, err := newGateway(t, nil, nil).HandleCallback(callback(t, map[string]any{"orderNumber": "ORD-2026-001235", "status": status, "hashValue": hash}, nil))
			if err != nil {
				t.Fatal(err)
			}
			if result.OrderID != "ORD-2026-001235" || result.Status != want {
				t.Fatalf("unexpected callback %+v", result)
			}
		})
	}
}

func TestCallbackWithAWrongHashIsRejected(t *testing.T) {
	_, err := newGateway(t, nil, nil).HandleCallback(callback(t, map[string]any{
		"orderNumber": "ORD-1", "status": "success", "hashValue": hmacHex("orderNumber=ORD-1&status=fail", "ORD-1hash-key"),
	}, nil))
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureVerificationError, got %v", err)
	}
}

func TestWebhookSecretHeaderIsCheckedWhenConfigured(t *testing.T) {
	gateway := newGateway(t, nil, nil, func(c *Config) { c.WebhookSecret = "shared" })
	payload := map[string]any{"orderNumber": "ORD-1", "status": "success", "hashValue": hmacHex("orderNumber=ORD-1&status=success", "ORD-1hash-key")}

	if result, err := gateway.HandleCallback(callback(t, payload, http.Header{"X-Webhook-Secret": {"shared"}})); err != nil || !result.IsSuccessful() {
		t.Fatalf("expected success, got %+v %v", result, err)
	}
	_, err := gateway.HandleCallback(callback(t, payload, nil))
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) || !strings.Contains(sigErr.Message, "X-Webhook-Secret") {
		t.Fatalf("expected header error, got %v", err)
	}
}

func TestValidationAndDefaults(t *testing.T) {
	var invalid *myanmarpayments.InvalidPaymentDataError
	err := PaymentData{OrderID: strings.Repeat("A", 21), Amount: myanmarpayments.Kyat(1000), Description: strings.Repeat("d", 51)}.Validate()
	if !errors.As(err, &invalid) || invalid.Errors["orderId"] == "" || invalid.Errors["description"] == "" {
		t.Fatalf("expected limits to fail, got %v", err)
	}
	if (Config{Production: true}).ResolvedBaseURL() != "https://paymenthubapi.yomabank.com" || (Config{}).ResolvedBaseURL() != SandboxURL {
		t.Fatal("unexpected base URLs")
	}
}

func TestYomaRejectsDecimalAndNonPositiveAmounts(t *testing.T) {
	for name, amount := range map[string]myanmarpayments.Amount{
		"decimal":  myanmarpayments.MustParseAmount("1000.50"),
		"zero":     myanmarpayments.Kyat(0),
		"negative": myanmarpayments.Kyat(-1),
		"missing":  {},
	} {
		t.Run(name, func(t *testing.T) {
			var invalid *myanmarpayments.InvalidPaymentDataError
			err := PaymentData{OrderID: "ORD-1", Amount: amount, Description: "Order 1"}.Validate()
			if !errors.As(err, &invalid) || invalid.Errors["amount"] == "" {
				t.Fatalf("expected amount error, got %v", err)
			}
			if name == "decimal" && !strings.Contains(invalid.Errors["amount"], "Yoma MMQR does not accept decimal amounts") {
				t.Fatalf("unexpected message %q", invalid.Errors["amount"])
			}
		})
	}
}

func TestUnconfirmedCheckoutAndMissingQRAreAPIErrors(t *testing.T) {
	data := PaymentData{OrderID: "ORD-1", Amount: myanmarpayments.Kyat(1000), Description: "Order 1"}
	var apiErr *myanmarpayments.APIError

	server := testutil.NewServer(t, token("t"), testutil.Reply{Body: map[string]any{"checkOutStatus": false}})
	if _, err := newGateway(t, server, nil).Initiate(context.Background(), data); !errors.As(err, &apiErr) || apiErr.Message != "Yoma MMQR did not confirm the checkout." {
		t.Fatalf("unexpected error %v", err)
	}

	server = testutil.NewServer(t, token("t"), testutil.Reply{Body: map[string]any{"qrString": "", "refLabel": "1"}})
	if _, err := newGateway(t, server, nil).RenewQR(context.Background(), "ORD-1"); !errors.As(err, &apiErr) || apiErr.Message != "Yoma MMQR did not return a QR." {
		t.Fatalf("unexpected error %v", err)
	}

	var invalid *myanmarpayments.InvalidPaymentDataError
	if _, err := newGateway(t, nil, nil).Initiate(context.Background(), PaymentData{}); !errors.As(err, &invalid) {
		t.Fatalf("expected InvalidPaymentDataError, got %v", err)
	}
}

func TestFailedTokenRequestIsAnAPIError(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Status: http.StatusUnauthorized, Body: map[string]any{"error": "invalid_client", "error_description": "Client authentication failed"}})

	_, err := newGateway(t, server, nil).Status(context.Background(), "1")
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) || apiErr.GatewayCode != "invalid_client" || apiErr.GatewayMessage != "Client authentication failed" || apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("unexpected error %v", err)
	}
}

type ttlCache struct {
	*myanmarpayments.MemoryTokenCache
	ttls map[string]time.Duration
}

func (c ttlCache) Set(key, value string, ttl time.Duration) {
	c.ttls[key] = ttl
	c.MemoryTokenCache.Set(key, value, ttl)
}

func TestTokenLifetimeIsReadFromTheLeadingDigitsOfExpiresIn(t *testing.T) {
	cases := map[string]time.Duration{
		`28800`: 28740 * time.Second, `"28800"`: 28740 * time.Second, `28800.0`: 28740 * time.Second,
		`"1e5"`: time.Minute, `90`: time.Minute, `0`: 3540 * time.Second, `"abc"`: 3540 * time.Second,
		`null`: 3540 * time.Second,
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			server := testutil.NewServer(t,
				testutil.Reply{Body: map[string]any{"access_token": "tok", "expires_in": json.RawMessage(raw)}},
				testutil.Reply{Body: map[string]any{"refLabel": "REF", "paymentStatus": "PENDING"}},
			)
			cache := ttlCache{myanmarpayments.NewMemoryTokenCache(), map[string]time.Duration{}}
			gateway := newGateway(t, server, cache)
			if _, err := gateway.Status(context.Background(), "REF"); err != nil {
				t.Fatal(err)
			}
			if got := cache.ttls[gateway.tokenCacheKey()]; got != want {
				t.Fatalf("ttl = %v, want %v", got, want)
			}
		})
	}
}

func TestTokenCacheKeyIsSharedWithTheOtherSDKs(t *testing.T) {
	gateway := newGateway(t, nil, nil)
	sum := sha256.Sum256([]byte(SandboxURL + "|client"))
	if gateway.tokenCacheKey() != "myanmar-payments.yoma-mmqr.token."+hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected key %s", gateway.tokenCacheKey())
	}
}

func TestNestedStatusFailsVerification(t *testing.T) {
	body := `{"orderNumber":"ORDER1","status":{"code":"SUCCESS"},"hashValue":"` + hmacHex("orderNumber=ORDER1&status=", "ORDER1hash-key") + `"}`
	_, err := newGateway(t, nil, nil).HandleCallback(myanmarpayments.NewCallbackRequest([]byte(body), nil, nil))
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureVerificationError, got %v", err)
	}
}

func TestAPIErrorWithoutAMessageHasNoTrailingSpace(t *testing.T) {
	server := testutil.NewServer(t, token("tok"), testutil.Reply{Body: map[string]any{"errorCode": "E01"}})
	_, err := newGateway(t, server, nil).Status(context.Background(), "REF")
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "Yoma MMQR payment/check-status failed: [E01]" {
		t.Fatalf("unexpected error %#v", err)
	}
}
