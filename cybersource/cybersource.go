// Package cybersource integrates CyberSource Secure Acceptance hosted checkout for card payments.
package cybersource

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

var signedFields = []string{
	"access_key", "profile_id", "transaction_uuid", "signed_field_names", "signed_date_time", "locale",
	"transaction_type", "reference_number", "amount", "currency",
	"override_custom_receipt_page", "override_backoffice_post_url", "override_custom_cancel_page",
}

var statuses = map[string]myanmarpayments.PaymentStatus{
	"ACCEPT":  myanmarpayments.StatusSuccessful,
	"REVIEW":  myanmarpayments.StatusPending,
	"DECLINE": myanmarpayments.StatusFailed,
	"ERROR":   myanmarpayments.StatusFailed,
	"CANCEL":  myanmarpayments.StatusCanceled,
}

// Gateway signs Secure Acceptance forms and verifies their results.
type Gateway struct {
	config Config
	now    func() time.Time
	uuid   func() string
}

// New returns a Gateway.
func New(config Config) (*Gateway, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Gateway{config: config, now: time.Now, uuid: randomUUID}, nil
}

// Config returns the configuration in use.
func (g *Gateway) Config() Config { return g.config }

// Initiate signs the payment fields. The customer's browser must POST the returned form to
// CyberSource; FormPayment.HTML renders a page that does it.
func (g *Gateway) Initiate(data PaymentData) (*myanmarpayments.FormPayment, error) {
	if err := data.Validate(); err != nil {
		return nil, err
	}

	fieldValues := map[string]string{
		"access_key":                   g.config.AccessKey,
		"profile_id":                   g.config.ProfileID,
		"transaction_uuid":             g.uuid(),
		"signed_field_names":           strings.Join(signedFields, ","),
		"signed_date_time":             g.now().UTC().Format("2006-01-02T15:04:05Z"),
		"locale":                       data.Locale,
		"transaction_type":             string(data.TransactionType),
		"reference_number":             data.OrderID,
		"amount":                       data.Amount.String(),
		"currency":                     data.Currency,
		"override_custom_receipt_page": data.ReturnURL,
		"override_backoffice_post_url": data.CallbackURL,
		"override_custom_cancel_page":  data.CancelURL,
	}

	fields := make([]myanmarpayments.FormField, 0, len(signedFields)+1)
	signable := make(map[string]any, len(fieldValues))
	for _, name := range signedFields {
		fields = append(fields, myanmarpayments.FormField{Name: name, Value: fieldValues[name]})
		signable[name] = fieldValues[name]
	}
	signature, _ := g.sign(signable)
	fields = append(fields, myanmarpayments.FormField{Name: "signature", Value: signature})

	return &myanmarpayments.FormPayment{OrderID: data.OrderID, Action: g.config.ResolvedBaseURL() + "/pay", Fields: fields, Enctype: "application/x-www-form-urlencoded"}, nil
}

// HandleCallback verifies CyberSource's result post. The same check works for the browser post
// to your receipt page.
//
// Only signed fields are trusted: decision and req_reference_number must be listed in
// signed_field_names, the amount and transaction id are read only when they are signed, and Raw
// keeps only the signed fields plus signature.
// Without this rule the signed request form, which the customer's browser sees, could be
// replayed with unsigned result fields added.
func (g *Gateway) HandleCallback(request *myanmarpayments.CallbackRequest) (*myanmarpayments.PaymentCallback, error) {
	payload := request.Input()
	expected, ok := g.sign(payload)
	signed := signedNames(payload)
	if !ok || !hmac.Equal([]byte(expected), []byte(values.Get(payload, "signature"))) || !signed["decision"] || !signed["req_reference_number"] {
		return nil, &myanmarpayments.SignatureVerificationError{Message: "CyberSource callback signature verification failed.", Raw: payload}
	}

	signedValue := func(name string) string {
		if !signed[name] {
			return ""
		}
		return values.Get(payload, name)
	}

	decision := strings.ToUpper(values.Trimmed(payload, "decision"))
	amount := signedValue("auth_amount")
	if amount == "" {
		amount = signedValue("req_amount")
	}

	raw := map[string]any{"signature": payload["signature"]}
	for name := range signed {
		raw[name] = payload[name]
	}

	return &myanmarpayments.PaymentCallback{
		OrderID:          values.Get(payload, "req_reference_number"),
		Status:           myanmarpayments.ResolveStatus(statuses, decision),
		GatewayStatus:    decision,
		GatewayReference: signedValue("transaction_id"),
		Amount:           amount,
		Raw:              raw,
		Acknowledgement:  myanmarpayments.DefaultAcknowledgement(),
	}, nil
}

// signedNames returns the field names listed in signed_field_names.
func signedNames(fields map[string]any) map[string]bool {
	names := map[string]bool{}
	for _, name := range strings.Split(values.Get(fields, "signed_field_names"), ",") {
		if name != "" {
			names[name] = true
		}
	}
	return names
}

// sign signs the fields listed in signed_field_names; ok is false when a listed field is missing.
func (g *Gateway) sign(fields map[string]any) (string, bool) {
	names := strings.Split(values.Get(fields, "signed_field_names"), ",")
	pairs := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		value, ok := values.String(fields[name])
		if !ok {
			return "", false
		}
		pairs = append(pairs, name+"="+value)
	}
	if len(pairs) == 0 {
		return "", false
	}

	mac := hmac.New(sha256.New, []byte(g.config.SecretKey))
	mac.Write([]byte(strings.Join(pairs, ",")))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), true
}

func randomUUID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
