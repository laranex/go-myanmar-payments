// Package wavemoney integrates Wave Money (WavePay payment gateway): redirect payments and
// callbacks. Wave has no status API, so the callback is the only result.
package wavemoney

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments"
	"github.com/laranex/go-myanmar-payments/internal/transport"
	"github.com/laranex/go-myanmar-payments/internal/values"
)

var statuses = map[string]myanmarpayments.PaymentStatus{
	"PAYMENT_CONFIRMED":               myanmarpayments.StatusSuccessful,
	"INSUFFICIENT_BALANCE":            myanmarpayments.StatusPending,
	"ACCOUNT_LOCKED":                  myanmarpayments.StatusFailed,
	"BILL_COLLECTION_FAILED":          myanmarpayments.StatusFailed,
	"PAYMENT_REQUEST_CANCELLED":       myanmarpayments.StatusCancelled,
	"TRANSACTION_TIMED_OUT":           myanmarpayments.StatusExpired,
	"SCHEDULER_TRANSACTION_TIMED_OUT": myanmarpayments.StatusExpired,
}

var callbackFields = []string{
	"status", "timeToLiveSeconds", "merchantId", "orderId", "amount", "backendResultUrl",
	"merchantReferenceId", "initiatorMsisdn", "transactionId", "paymentRequestId", "requestTime",
}

// Gateway talks to Wave Money.
type Gateway struct {
	config    Config
	transport *transport.Client
}

// New returns a Gateway. A nil client uses myanmarpayments.DefaultHTTPClient.
func New(config Config, client myanmarpayments.HTTPDoer) (*Gateway, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Gateway{config: config, transport: transport.New(client)}, nil
}

// Config returns the configuration in use.
func (g *Gateway) Config() Config { return g.config }

// Initiate creates a payment request and returns Wave's page to redirect the customer to.
// It fills data.MerchantReferenceID with a random id when empty, so store it afterwards.
func (g *Gateway) Initiate(ctx context.Context, data *PaymentData) (*myanmarpayments.RedirectPayment, error) {
	if data.MerchantReferenceID == "" {
		data.MerchantReferenceID = randomReference()
	}
	if err := data.Validate(); err != nil {
		return nil, err
	}

	amount := data.ResolvedAmount()
	ttl := g.config.ResolvedTimeToLive()
	items, err := json.Marshal(data.Items)
	if err != nil {
		return nil, fmt.Errorf("myanmarpayments: encode items: %w", err)
	}

	form := url.Values{}
	form.Set("time_to_live_in_seconds", strconv.Itoa(ttl))
	form.Set("merchant_id", g.config.MerchantID)
	form.Set("order_id", data.OrderID)
	form.Set("merchant_reference_id", data.MerchantReferenceID)
	form.Set("frontend_result_url", data.ReturnURL)
	form.Set("backend_result_url", data.CallbackURL)
	form.Set("amount", strconv.FormatInt(amount, 10))
	form.Set("payment_description", data.Description)
	form.Set("merchant_name", g.config.MerchantName)
	form.Set("items", string(items))
	form.Set("hash", g.hash([]string{strconv.Itoa(ttl), g.config.MerchantID, data.OrderID, strconv.FormatInt(amount, 10), data.CallbackURL, data.MerchantReferenceID}))

	response, err := g.transport.PostForm(ctx, g.config.ResolvedBaseURL()+"/payment", form, nil)
	if err != nil {
		return nil, err
	}

	body := response.JSON()
	transactionID := values.Get(body, "transaction_id")
	if !response.Successful() || values.Get(body, "message") != "success" || transactionID == "" {
		message := errorMessage(body)
		code := values.Get(body, "message")
		if _, ok := body["errors"]; ok {
			code = "VALIDATION_ERROR"
		}
		return nil, &myanmarpayments.APIError{
			Message:        fmt.Sprintf("Wave Money payment request failed with HTTP %d: %s", response.Status, message),
			GatewayCode:    code,
			GatewayMessage: message,
			HTTPStatus:     response.Status,
			Raw:            body,
		}
	}

	return &myanmarpayments.RedirectPayment{
		OrderID:          data.OrderID,
		URL:              g.config.ResolvedAuthenticateURL() + "/authenticate?transaction_id=" + url.QueryEscape(transactionID),
		GatewayReference: transactionID,
		Raw:              body,
	}, nil
}

// HandleCallback verifies Wave's callback. Only PAYMENT_CONFIRMED means the customer paid.
// OrderID falls back to merchantReferenceId because Wave marks orderId as optional.
func (g *Gateway) HandleCallback(request *myanmarpayments.CallbackRequest) (*myanmarpayments.PaymentCallback, error) {
	payload := request.ParsedBody()

	parts := make([]string, len(callbackFields))
	for i, field := range callbackFields {
		value, ok := values.String(payload[field])
		if !ok {
			value = "null"
		}
		parts[i] = value
	}
	hashValue, _ := payload["hashValue"].(string)
	if !hmac.Equal([]byte(g.hash(parts)), []byte(strings.ToLower(hashValue))) {
		return nil, &myanmarpayments.SignatureVerificationError{Message: "Wave Money callback hash verification failed.", Raw: payload}
	}

	gatewayStatus := values.Trimmed(payload, "status")
	orderID := values.Get(payload, "orderId")
	if orderID == "" {
		orderID = values.Get(payload, "merchantReferenceId")
	}

	return &myanmarpayments.PaymentCallback{
		OrderID:          orderID,
		Status:           myanmarpayments.ResolveStatus(statuses, gatewayStatus),
		GatewayStatus:    gatewayStatus,
		GatewayReference: values.Get(payload, "transactionId"),
		Amount:           values.Get(payload, "amount"),
		Raw:              payload,
		Acknowledgement:  myanmarpayments.DefaultAcknowledgement(),
	}, nil
}

func (g *Gateway) hash(parts []string) string {
	mac := hmac.New(sha256.New, []byte(g.config.SecretKey))
	mac.Write([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(mac.Sum(nil))
}

func errorMessage(body map[string]any) string {
	if errs, ok := body["errors"].(map[string]any); ok {
		fields := make([]string, 0, len(errs))
		for field := range errs {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		messages := make([]string, 0, len(fields))
		for _, field := range fields {
			var texts []string
			if list, ok := errs[field].([]any); ok {
				for _, item := range list {
					if text, ok := values.String(item); ok {
						texts = append(texts, text)
					}
				}
			}
			messages = append(messages, field+": "+strings.Join(texts, " "))
		}
		return strings.Join(messages, "; ")
	}
	if message := values.Get(body, "message"); message != "" {
		return message
	}
	return "unexpected response"
}
