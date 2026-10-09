// Package ayapay integrates the AYA Payment Gateway (APG): one hosted checkout for AYA Pay,
// other wallets and cards, with channel listing, status enquiry and verified callbacks.
package ayapay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/transport"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

const currencyMMK = "104"

var statuses = map[string]myanmarpayments.PaymentStatus{
	"00": myanmarpayments.StatusSuccessful,
	"01": myanmarpayments.StatusPending,
	"02": myanmarpayments.StatusFailed,
	"03": myanmarpayments.StatusFailed,
	"04": myanmarpayments.StatusExpired,
}

// payloadFields is the documented field order of the decoded callback / enquiry payload.
// AYA leaves out fields that do not apply (e.g. card fields for wallet payments) and signs
// only the ones present. The payload spells currencyCode as "currenyCode".
var payloadFields = []string{
	"merchOrderId", "tranId", "amount", "currencyCode", "statusCode",
	"paymentCardNumber", "paymentMobileNumber", "cardTypeName", "cardExpiryDate", "nameOnCard",
	"approvalCode", "tranRef", "userRef1", "userRef2", "userRef3", "userRef4", "userRef5",
	"description", "dateTime",
}

// Gateway talks to the AYA Payment Gateway.
type Gateway struct {
	config    Config
	transport *transport.Client
	now       func() time.Time
}

// New returns a Gateway. A nil client uses myanmarpayments.DefaultHTTPClient.
func New(config Config, client myanmarpayments.HTTPDoer) (*Gateway, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Gateway{config: config, transport: transport.New(client), now: time.Now}, nil
}

// Config returns the configuration in use.
func (g *Gateway) Config() Config { return g.config }

// Services lists the payment channels enabled for your merchant account.
func (g *Gateway) Services(ctx context.Context) ([]Service, error) {
	timestamp := g.now().Unix()
	ts := strconv.FormatInt(timestamp, 10)
	body, err := g.post(ctx, "services", map[string]any{
		"appKey":    g.config.AppKey,
		"timestamp": timestamp,
		"checkSum":  g.checksum(g.config.AppKey, g.config.AppSecret, ts),
	})
	if err != nil {
		return nil, err
	}

	list, _ := body["data"].([]any)
	services := make([]Service, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok || values.Get(entry, "key") == "" {
			continue
		}
		service := Service{Key: values.Get(entry, "key"), Name: values.Get(entry, "name"), ImageURL: values.Get(entry, "image_url")}
		if service.Name == "" {
			service.Name = service.Key
		}
		methods, _ := entry["methods"].([]any)
		for _, m := range methods {
			name, _ := values.String(m)
			if method := Method(name); method.valid() {
				service.Methods = append(service.Methods, method)
			} else {
				service.UnknownMethods = append(service.UnknownMethods, name)
			}
		}
		services = append(services, service)
	}
	return services, nil
}

// Initiate signs the order. The customer's browser must POST the returned form to AYA's
// hosted checkout; FormPayment.HTML renders a page that does it.
func (g *Gateway) Initiate(data PaymentData) (*myanmarpayments.FormPayment, error) {
	if err := data.Validate(); err != nil {
		return nil, err
	}

	refs := make([]string, 5)
	copy(refs, data.UserRefs)
	fields := []myanmarpayments.FormField{
		{Name: "merchOrderId", Value: data.OrderID},
		{Name: "amount", Value: data.Amount.String()},
		{Name: "appKey", Value: g.config.AppKey},
		{Name: "timestamp", Value: strconv.FormatInt(g.now().Unix(), 10)},
		{Name: "userRef1", Value: refs[0]},
		{Name: "userRef2", Value: refs[1]},
		{Name: "userRef3", Value: refs[2]},
		{Name: "userRef4", Value: refs[3]},
		{Name: "userRef5", Value: refs[4]},
		{Name: "description", Value: data.Description},
		{Name: "currencyCode", Value: currencyMMK},
		{Name: "channel", Value: data.Channel},
		{Name: "method", Value: string(data.Method)},
		{Name: "overrideFrontendRedirectUrl", Value: data.ReturnURL},
	}
	parts := make([]string, len(fields))
	for i, field := range fields {
		parts[i] = field.Value
	}
	fields = append(fields, myanmarpayments.FormField{Name: "checkSum", Value: g.checksum(parts...)})

	return &myanmarpayments.FormPayment{
		OrderID: data.OrderID,
		Action:  g.config.ResolvedBaseURL() + "/v1/payment/request",
		Fields:  fields,
		Enctype: "multipart/form-data",
	}, nil
}

// Status asks AYA for the current state of an order (enquiry).
func (g *Gateway) Status(ctx context.Context, orderID string) (*myanmarpayments.PaymentStatusResult, error) {
	timestamp := g.now().Unix()
	body, err := g.post(ctx, "enquiry", map[string]any{
		"merchOrderId": orderID,
		"appKey":       g.config.AppKey,
		"timestamp":    timestamp,
		"checkSum":     g.checksum(orderID, strconv.FormatInt(timestamp, 10), g.config.AppKey),
	})
	if err != nil {
		return nil, err
	}

	data := values.Map(body, "data")
	if data == nil {
		data = map[string]any{}
	}
	payload, err := g.verifiedPayload(data, "enquiry response")
	if err != nil {
		return nil, err
	}

	statusCode := values.Trimmed(payload, "statusCode")
	result := &myanmarpayments.PaymentStatusResult{
		OrderID:          orderID,
		Status:           myanmarpayments.ResolveStatus(statuses, statusCode),
		GatewayStatus:    statusCode,
		GatewayReference: values.Get(payload, "tranId"),
		Amount:           values.Get(payload, "amount"),
		Raw:              payload,
	}
	if id := values.Get(payload, "merchOrderId"); id != "" {
		result.OrderID = id
	}
	return result, nil
}

// HandleCallback verifies AYA's backend callback.
func (g *Gateway) HandleCallback(request *myanmarpayments.CallbackRequest) (*myanmarpayments.PaymentCallback, error) {
	payload, err := g.verifiedPayload(request.Input(), "callback")
	if err != nil {
		return nil, err
	}
	return toCallback(payload), nil
}

// VerifyRedirect verifies the signed query string AYA adds when it sends the customer back
// to your return URL. Use it to show the right page; fulfill orders from the backend callback.
func (g *Gateway) VerifyRedirect(request *myanmarpayments.CallbackRequest) (*myanmarpayments.PaymentCallback, error) {
	payload, err := g.verifiedPayload(request.QueryInput(), "redirect")
	if err != nil {
		return nil, err
	}
	return toCallback(payload), nil
}

func toCallback(payload map[string]any) *myanmarpayments.PaymentCallback {
	statusCode := values.Trimmed(payload, "statusCode")
	return &myanmarpayments.PaymentCallback{
		OrderID:          values.Get(payload, "merchOrderId"),
		Status:           myanmarpayments.ResolveStatus(statuses, statusCode),
		GatewayStatus:    statusCode,
		GatewayReference: values.Get(payload, "tranId"),
		Amount:           values.Get(payload, "amount"),
		Raw:              payload,
		Acknowledgement:  myanmarpayments.DefaultAcknowledgement(),
	}
}

// verifiedPayload decodes a payload + checkSum pair and returns the payload if the checksum matches.
func (g *Gateway) verifiedPayload(input map[string]any, context string) (map[string]any, error) {
	// A "+" in the base64 payload arrives as a space when the query string is not encoded;
	// base64 never contains spaces, so map them back. The checksum is still verified.
	encoded, checkSum := strings.ReplaceAll(values.Get(input, "payload"), " ", "+"), values.Get(input, "checkSum")
	fail := &myanmarpayments.SignatureVerificationError{Message: fmt.Sprintf("AYA Pay %s checksum verification failed.", context), Raw: input}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || encoded == "" {
		return nil, fail
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return nil, fail
	}

	var parts []string
	for _, field := range payloadFields {
		key := field
		if field == "currencyCode" {
			if _, ok := payload["currenyCode"]; ok {
				key = "currenyCode"
			}
		}
		value, present := payload[key]
		if !present {
			continue
		}
		str, _ := values.String(value)
		parts = append(parts, str)
	}

	if !hmac.Equal([]byte(g.checksum(parts...)), []byte(strings.ToLower(checkSum))) {
		return nil, fail
	}
	return payload, nil
}

func (g *Gateway) checksum(parts ...string) string {
	mac := hmac.New(sha256.New, []byte(g.config.AppSecret))
	mac.Write([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(mac.Sum(nil))
}

func (g *Gateway) post(ctx context.Context, endpoint string, data map[string]any) (map[string]any, error) {
	response, err := g.transport.PostJSON(ctx, g.config.ResolvedBaseURL()+"/v1/payment/"+endpoint, data, nil)
	if err != nil {
		return nil, err
	}
	body := response.JSON()
	status := values.Get(body, "status")
	if !response.Successful() || status != "00" {
		message := values.Get(body, "message")
		text := fmt.Sprintf("AYA Pay %s failed with HTTP %d.", endpoint, response.Status)
		if status != "" {
			text = fmt.Sprintf("AYA Pay %s failed: [%s] %s", endpoint, status, message)
		}
		return nil, &myanmarpayments.APIError{Message: text, GatewayCode: status, GatewayMessage: message, HTTPStatus: response.Status, Raw: body}
	}
	return body, nil
}
