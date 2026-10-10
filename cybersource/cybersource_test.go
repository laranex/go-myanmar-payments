package cybersource

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

func newGateway(t *testing.T) *Gateway {
	t.Helper()
	gateway, err := New(Config{ProfileID: "profile", AccessKey: "access", SecretKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

func signature(fields map[string]string) string {
	names := strings.Split(fields["signed_field_names"], ",")
	pairs := make([]string, len(names))
	for i, name := range names {
		pairs[i] = name + "=" + fields[name]
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(strings.Join(pairs, ",")))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func callback(overrides map[string]string) *myanmarpayments.CallbackRequest {
	fields := map[string]string{
		"decision": "ACCEPT", "req_reference_number": "ORDER-1", "transaction_id": "7000000000000000000000",
		"auth_amount": "1000.00", "req_amount": "1000.00",
		"signed_field_names": "decision,req_reference_number,transaction_id,auth_amount,signed_field_names",
	}
	for key, value := range overrides {
		fields[key] = value
	}
	fields["signature"] = signature(fields)

	form := url.Values{}
	for key, value := range fields {
		form.Set(key, value)
	}
	return myanmarpayments.NewCallbackRequest([]byte(form.Encode()), nil, nil)
}

func TestInitiateSignsTheHostedCheckoutFields(t *testing.T) {
	payment, err := newGateway(t).Initiate(PaymentData{OrderID: "ORDER-1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cs/callback", ReturnURL: "https://shop.test/done", Currency: "MMK", TransactionType: Authorization, Locale: "en-us"})
	if err != nil {
		t.Fatal(err)
	}
	fields := payment.Values()
	if payment.Action != ProductionURL+"/pay" || fields["reference_number"] != "ORDER-1" || fields["amount"] != "1000" ||
		fields["transaction_type"] != "authorization" || fields["currency"] != "MMK" || fields["locale"] != "en-us" {
		t.Fatalf("unexpected fields %v", fields)
	}
	if fields["signature"] != signature(fields) {
		t.Fatal("signature mismatch")
	}
	if payment.Enctype != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected enctype %q", payment.Enctype)
	}
}

func TestDecimalAmountsAndOtherCurrencies(t *testing.T) {
	payment, err := newGateway(t).Initiate(PaymentData{OrderID: "ORDER-2", Amount: myanmarpayments.MustParseAmount("10.50"), CallbackURL: "https://shop.test/cb", Currency: "USD", TransactionType: Sale, Locale: "en-us"})
	if err != nil {
		t.Fatal(err)
	}
	if fields := payment.Values(); fields["amount"] != "10.50" || fields["currency"] != "USD" {
		t.Fatalf("unexpected fields %v", fields)
	}
}

func TestDecisionsAreMapped(t *testing.T) {
	cases := map[string]myanmarpayments.PaymentStatus{
		"ACCEPT": myanmarpayments.StatusSuccessful, "REVIEW": myanmarpayments.StatusPending, "DECLINE": myanmarpayments.StatusFailed,
		"ERROR": myanmarpayments.StatusFailed, "CANCEL": myanmarpayments.StatusCanceled,
	}
	for decision, want := range cases {
		t.Run(decision, func(t *testing.T) {
			result, err := newGateway(t).HandleCallback(callback(map[string]string{"decision": decision}))
			if err != nil {
				t.Fatal(err)
			}
			if result.OrderID != "ORDER-1" || result.GatewayReference != "7000000000000000000000" || result.Amount != "1000.00" || result.Status != want {
				t.Fatalf("unexpected callback %+v", result)
			}
		})
	}
}

func TestMissingSignedFieldIsRejected(t *testing.T) {
	form := url.Values{"decision": {"ACCEPT"}, "signed_field_names": {"decision,req_amount,signed_field_names"}, "signature": {"x"}}
	_, err := newGateway(t).HandleCallback(myanmarpayments.NewCallbackRequest([]byte(form.Encode()), nil, nil))
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureVerificationError, got %v", err)
	}
}

func TestTamperedCallbackIsRejected(t *testing.T) {
	request := callback(nil)
	form, _ := url.ParseQuery(string(request.Body))
	form.Set("auth_amount", "1.00")
	_, err := newGateway(t).HandleCallback(myanmarpayments.NewCallbackRequest([]byte(form.Encode()), nil, nil))
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureVerificationError, got %v", err)
	}
}

func TestValidationAcceptsHTTPURLs(t *testing.T) {
	data := PaymentData{OrderID: "ORDER-4", Amount: myanmarpayments.Kyat(1000), CallbackURL: "http://shop.test/cb", ReturnURL: "http://shop.test/done", CancelURL: "http://shop.test/cancel", Currency: "MMK", TransactionType: Sale, Locale: "en-us"}
	if err := data.Validate(); err != nil {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestValidationFollowsTheSecureAcceptanceFieldRules(t *testing.T) {
	base := PaymentData{OrderID: "ORDER-1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb", Currency: "MMK", TransactionType: Sale, Locale: "en-us"}
	cases := map[string]struct {
		mutate func(*PaymentData)
		field  string
	}{
		"ftp callback":         {func(d *PaymentData) { d.CallbackURL = "ftp://shop.test/cb" }, "callbackUrl"},
		"url over 255":         {func(d *PaymentData) { d.ReturnURL = "https://shop.test/" + strings.Repeat("a", 250) }, "returnUrl"},
		"amount over 15 chars": {func(d *PaymentData) { d.Amount = myanmarpayments.MustParseAmount("1234567890123.45") }, "amount"},
		"negative amount":      {func(d *PaymentData) { d.Amount = myanmarpayments.Kyat(-1) }, "amount"},
		"missing amount":       {func(d *PaymentData) { d.Amount = myanmarpayments.Amount{} }, "amount"},
		"plain en locale":      {func(d *PaymentData) { d.Locale = "en" }, "locale"},
		"order id over 50":     {func(d *PaymentData) { d.OrderID = strings.Repeat("A", 51) }, "orderId"},
		"unknown type":         {func(d *PaymentData) { d.TransactionType = "refund" }, "transactionType"},
		"missing currency":     {func(d *PaymentData) { d.Currency = "" }, "currency"},
		"missing type":         {func(d *PaymentData) { d.TransactionType = "" }, "transactionType"},
		"missing locale":       {func(d *PaymentData) { d.Locale = " " }, "locale"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			data := base
			tc.mutate(&data)
			var invalid *myanmarpayments.InvalidPaymentDataError
			if err := data.Validate(); !errors.As(err, &invalid) || invalid.Errors[tc.field] == "" {
				t.Fatalf("expected error on %s, got %v", tc.field, err)
			}
		})
	}
}

func TestCyberSourceAcceptsZeroAndAnyDecimalPlaces(t *testing.T) {
	for _, amount := range []myanmarpayments.Amount{myanmarpayments.Kyat(0), myanmarpayments.MustParseAmount("10.505")} {
		data := PaymentData{OrderID: "ORDER-1", Amount: amount, CallbackURL: "https://shop.test/cb", Currency: "MMK", TransactionType: Sale, Locale: "en-us"}
		if err := data.Validate(); err != nil {
			t.Fatalf("amount %s: unexpected error %v", amount, err)
		}
	}
}

func TestReplayedRequestFormWithUnsignedResultFieldsIsRejected(t *testing.T) {
	payment, err := newGateway(t).Initiate(PaymentData{OrderID: "ORDER-1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb", Currency: "MMK", TransactionType: Sale, Locale: "en-us"})
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{}
	for _, field := range payment.Fields {
		form.Set(field.Name, field.Value)
	}
	form.Set("decision", "ACCEPT")
	form.Set("req_reference_number", "ORDER-1")
	form.Set("auth_amount", "1000")

	_, err = newGateway(t).HandleCallback(myanmarpayments.NewCallbackRequest([]byte(form.Encode()), nil, nil))
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureVerificationError, got %v", err)
	}
}

func TestUnsignedAmountAndReferenceAreIgnored(t *testing.T) {
	request := callback(map[string]string{"signed_field_names": "decision,req_reference_number,req_amount,signed_field_names"})
	form, _ := url.ParseQuery(string(request.Body))
	form.Set("auth_amount", "1.00")
	form.Set("transaction_id", "forged")

	result, err := newGateway(t).HandleCallback(myanmarpayments.NewCallbackRequest([]byte(form.Encode()), nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if result.Amount != "1000.00" || result.GatewayReference != "" || !result.IsSuccessful() {
		t.Fatalf("unexpected callback %+v", result)
	}
}

func TestConfigFromEnvAndEndpoints(t *testing.T) {
	env := map[string]string{
		"CYBER_SOURCE_PROFILE_ID": "profile", "CYBER_SOURCE_ACCESS_KEY": "access", "CYBER_SOURCE_SECRET_KEY": "secret",
	}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	if config.ProfileID != "profile" || config.AccessKey != "access" || config.SecretKey != "secret" {
		t.Fatalf("unexpected config %+v", config)
	}
	if config.ResolvedBaseURL() != ProductionURL || (Config{BaseURL: " "}).ResolvedBaseURL() != ProductionURL || (Config{BaseURL: "https://cs.test/"}).ResolvedBaseURL() != "https://cs.test" {
		t.Fatal("unexpected base URL")
	}

	gateway, err := New(config)
	if err != nil || gateway.Config() != config {
		t.Fatalf("unexpected gateway %v", err)
	}
	var configErr *myanmarpayments.ConfigurationError
	if _, err := New(Config{ProfileID: "p", AccessKey: "a"}); !errors.As(err, &configErr) || configErr.Key != "secret_key" {
		t.Fatalf("expected missing secret_key, got %v", err)
	}
}

func TestRawKeepsOnlySignedFields(t *testing.T) {
	request := callback(nil)
	form, _ := url.ParseQuery(string(request.Body))
	form.Set("unsigned_note", "forged")

	result, err := newGateway(t).HandleCallback(myanmarpayments.NewCallbackRequest([]byte(form.Encode()), nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Raw["unsigned_note"]; ok || result.Raw["decision"] != "ACCEPT" || result.Raw["signature"] == nil || len(result.Raw) != 6 {
		t.Fatalf("unexpected raw %v", result.Raw)
	}
}
