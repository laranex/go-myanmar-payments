// Package wavepay integrates Wave Pay (Pay with Wave API v1.3): redirect payments and
// callbacks. Wave has no status API, so the callback is the only result.
package wavepay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/env"
	"github.com/laranex/go-myanmar-payments/v4/internal/transport"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

// currency is the only currency Wave accepts.
const currency = "MMK"

var statuses = map[string]myanmarpayments.PaymentStatus{
	"PAYMENT_CONFIRMED":               myanmarpayments.StatusSuccessful,
	"BILL_COLLECTION_FAILED":          myanmarpayments.StatusFailed,
	"INSUFFICIENT_BALANCE":            myanmarpayments.StatusFailed,
	"ACCOUNT_LOCKED":                  myanmarpayments.StatusFailed,
	"PAYMENT_REQUEST_CANCELLED":       myanmarpayments.StatusCanceled,
	"TRANSACTION_TIMED_OUT":           myanmarpayments.StatusExpired,
	"SCHEDULER_TRANSACTION_TIMED_OUT": myanmarpayments.StatusExpired,
}

var callbackFields = []string{
	"status", "timeToLiveSeconds", "merchantId", "orderId", "amount", "backendResultUrl",
	"merchantReferenceId", "initiatorMsisdn", "transactionId", "paymentRequestId", "requestTime",
}

// Wave's hash concatenates the values with no separator, so a verified callback must also hold
// values in their expected formats. Payments validate their ids against the same pattern.
var (
	idPattern     = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	amountPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
)

const idDescription = "letters, numbers, underscores and dashes"

// paymentRequest is the JSON body of a payment request (Pay with Wave API spec v1.3, section
// 3.2.1): amount and time_to_live_in_seconds are JSON integers, items is a JSON string and the
// additional fields are only sent when set.
type paymentRequest struct {
	MerchantID          string      `json:"merchant_id"`
	OrderID             string      `json:"order_id"`
	MerchantReferenceID string      `json:"merchant_reference_id"`
	FrontendResultURL   string      `json:"frontend_result_url"`
	BackendResultURL    string      `json:"backend_result_url"`
	Amount              json.Number `json:"amount"`
	TimeToLiveInSeconds int         `json:"time_to_live_in_seconds"`
	PaymentDescription  string      `json:"payment_description"`
	Currency            string      `json:"currency"`
	Hash                string      `json:"hash"`
	MerchantName        string      `json:"merchant_name"`
	Items               string      `json:"items"`
	AdditionalField3    string      `json:"additional_field_3,omitempty"`
	AdditionalField4    string      `json:"additional_field_4,omitempty"`
	AdditionalField5    string      `json:"additional_field_5,omitempty"`
}

// Gateway talks to Wave Pay.
type Gateway struct {
	config    Config
	transport *transport.Client
}

// New returns a Gateway. A nil client uses an *http.Client with config.TimeoutSeconds, or
// myanmarpayments.DefaultTimeoutSeconds when it is 0.
func New(config Config, client myanmarpayments.HTTPDoer) (*Gateway, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	config.TimeoutSeconds = env.TimeoutSeconds(config.TimeoutSeconds)
	return &Gateway{config: config, transport: transport.New(client, config.TimeoutSeconds)}, nil
}

// Config returns the configuration in use.
func (g *Gateway) Config() Config { return g.config }

// Initiate creates a payment request and returns Wave's page to redirect the customer to.
// Once data passes validation it fills data.MerchantReferenceID with a random id when empty, so
// store it afterwards. Invalid data is returned untouched. A non-empty data.MerchantName and a
// non-nil data.TimeToLiveSeconds override the configured values for this payment only. The
// payment is a JSON post to Config.BaseURL's /payment (currency MMK; the additional fields are
// sent only when set) and the customer is sent to Config.BaseRedirectURL's
// /authenticate?transaction_id=....
func (g *Gateway) Initiate(ctx context.Context, data *PaymentData) (*myanmarpayments.RedirectPayment, error) {
	if err := data.Validate(); err != nil {
		return nil, err
	}
	ttl := g.config.TimeToLiveSeconds
	if data.TimeToLiveSeconds != nil {
		ttl = *data.TimeToLiveSeconds
	}
	if data.MerchantReferenceID == "" {
		data.MerchantReferenceID = randomReference()
	}

	amount := data.ResolvedAmount()
	merchantName := g.config.MerchantName
	if data.MerchantName != "" {
		merchantName = data.MerchantName
	}
	// Wave documents items as [{"name": "...", "amount": 1000}] with a numeric amount; the amount
	// text is emitted as a JSON number without passing through a float.
	type waveItem struct {
		Name   string      `json:"name"`
		Amount json.Number `json:"amount"`
	}
	wireItems := make([]waveItem, len(data.Items))
	for i, item := range data.Items {
		wireItems[i] = waveItem{Name: item.Name, Amount: json.Number(item.Amount.String())}
	}
	// Like the other Laranex SDKs, item names are not HTML-escaped (no \u003c for "<").
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(wireItems); err != nil {
		return nil, fmt.Errorf("myanmarpayments: Could not encode the items: %w", err)
	}
	items := bytes.TrimRight(encoded.Bytes(), "\n")

	payload := paymentRequest{
		MerchantID:          g.config.MerchantID,
		OrderID:             data.OrderID,
		MerchantReferenceID: data.MerchantReferenceID,
		FrontendResultURL:   data.ReturnURL,
		BackendResultURL:    data.CallbackURL,
		Amount:              json.Number(amount.String()),
		TimeToLiveInSeconds: ttl,
		PaymentDescription:  data.Description,
		Currency:            currency,
		Hash:                g.hash([]string{strconv.Itoa(ttl), g.config.MerchantID, data.OrderID, amount.String(), data.CallbackURL, data.MerchantReferenceID}),
		MerchantName:        merchantName,
		Items:               string(items),
		AdditionalField3:    data.AdditionalField3,
		AdditionalField4:    data.AdditionalField4,
		AdditionalField5:    data.AdditionalField5,
	}

	response, err := g.transport.PostJSON(ctx, g.config.ResolvedBaseURL()+"/payment", payload, nil)
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
			Message:        fmt.Sprintf("Wave Pay payment request failed with HTTP %d: %s", response.Status, message),
			GatewayCode:    code,
			GatewayMessage: message,
			HTTPStatus:     response.Status,
			Raw:            body,
		}
	}

	return &myanmarpayments.RedirectPayment{
		OrderID:          data.OrderID,
		URL:              g.config.ResolvedBaseRedirectURL() + "/authenticate?transaction_id=" + url.QueryEscape(transactionID),
		GatewayReference: transactionID,
		Raw:              body,
	}, nil
}

// HandleCallback verifies Wave's callback. Only PAYMENT_CONFIRMED means the customer paid.
// OrderID falls back to merchantReferenceId when orderId is missing, null or empty, as a
// safeguard: earlier Wave documents marked orderId optional. A callback whose amount is not a non-negative decimal, that has
// neither id, or whose orderId or merchantReferenceId holds anything but letters, numbers, "_"
// and "-", fails verification too.
func (g *Gateway) HandleCallback(request *myanmarpayments.CallbackRequest) (*myanmarpayments.PaymentCallback, error) {
	payload := request.ParsedBody()

	// Missing and null fields hash as "null". A nested value is never hashed, so a payload
	// carrying one in a hashed field is rejected rather than partly trusted.
	parts := make([]string, len(callbackFields))
	nested := false
	for i, field := range callbackFields {
		nested = nested || values.IsNested(payload[field])
		value, ok := values.String(payload[field])
		if !ok {
			value = "null"
		}
		parts[i] = value
	}
	hashValue, _ := payload["hashValue"].(string)
	orderID, referenceID := values.Get(payload, "orderId"), values.Get(payload, "merchantReferenceId")
	if nested || !hmac.Equal([]byte(g.hash(parts)), []byte(strings.ToLower(hashValue))) ||
		!amountPattern.MatchString(values.Get(payload, "amount")) || (orderID == "" && referenceID == "") ||
		(orderID != "" && !idPattern.MatchString(orderID)) || (referenceID != "" && !idPattern.MatchString(referenceID)) {
		return nil, &myanmarpayments.SignatureVerificationError{Message: "Wave Pay callback hash verification failed.", Raw: payload}
	}

	gatewayStatus := values.Trimmed(payload, "status")
	if orderID == "" {
		orderID = referenceID
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
