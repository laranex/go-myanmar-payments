package wavemoney

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/testutil"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

func newGateway(t *testing.T, server *testutil.Server) *Gateway {
	t.Helper()
	config := Config{MerchantID: "testmerchantID", SecretKey: "test-secret", MerchantName: "Shop"}
	if server != nil {
		config.BaseURL = server.URL
		config.AuthenticateURL = "https://preprodpayments.wavemoney.io"
	}
	gateway, err := New(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

func paymentData() *PaymentData {
	return &PaymentData{
		OrderID: "100", CallbackURL: "https://shop.test/wave/callback", ReturnURL: "https://shop.test/done",
		Description: "Order 100", Items: []Item{{"Shoes", myanmarpayments.Kyat(600)}, {"Socks", myanmarpayments.Kyat(400)}}, MerchantReferenceID: "ref-001",
	}
}

func hmacHex(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func signedCallback(t *testing.T, payload map[string]any, secret string) *myanmarpayments.CallbackRequest {
	t.Helper()
	parts := ""
	for _, field := range callbackFields {
		if value, ok := values.String(payload[field]); ok {
			parts += value
		} else {
			parts += "null"
		}
	}
	payload["hashValue"] = hmacHex(parts, secret)
	request, err := myanmarpayments.NewCallbackRequestFromJSON(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestInitiatePostsAHashedFormAndRedirectsToAuthenticate(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"message": "success", "transaction_id": "enc/123+abc"}})

	payment, err := newGateway(t, server).Initiate(context.Background(), paymentData())
	if err != nil {
		t.Fatal(err)
	}

	recorded := server.Last(t)
	form, _ := url.ParseQuery(string(recorded.Body))
	if recorded.Path != "/payment" || recorded.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected request %s %s", recorded.Path, recorded.Header.Get("Content-Type"))
	}
	if form.Get("order_id") != "100" || form.Get("merchant_reference_id") != "ref-001" || form.Get("amount") != "1000" || form.Get("merchant_name") != "Shop" {
		t.Fatalf("unexpected form %v", form)
	}
	if form.Get("items") != `[{"name":"Shoes","amount":600},{"name":"Socks","amount":400}]` {
		t.Fatalf("unexpected items %s", form.Get("items"))
	}
	if form.Get("hash") != hmacHex("300testmerchantID1001000https://shop.test/wave/callbackref-001", "test-secret") {
		t.Fatal("request hash mismatch")
	}
	if payment.URL != "https://preprodpayments.wavemoney.io/authenticate?transaction_id=enc%2F123%2Babc" || payment.GatewayReference != "enc/123+abc" {
		t.Fatalf("unexpected payment %+v", payment)
	}
}

func TestWaveErrorsAreSurfaced(t *testing.T) {
	cases := map[string]struct {
		status int
		body   map[string]any
		code   string
	}{
		"duplicate":  {409, map[string]any{"message": "Record already exists"}, "Record already exists"},
		"bad hash":   {400, map[string]any{"message": "INVALID_HASH"}, "INVALID_HASH"},
		"validation": {422, map[string]any{"errors": map[string]any{"amount": []any{"The amount must be an integer."}}}, "VALIDATION_ERROR"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := testutil.NewServer(t, testutil.Reply{Status: tc.status, Body: tc.body})
			_, err := newGateway(t, server).Initiate(context.Background(), paymentData())
			var apiErr *myanmarpayments.APIError
			if !errors.As(err, &apiErr) || apiErr.HTTPStatus != tc.status || apiErr.GatewayCode != tc.code {
				t.Fatalf("unexpected error %v", err)
			}
		})
	}
}

func TestAmountDefaultsToItemTotalAndReferenceIsUniquePerAttempt(t *testing.T) {
	server := testutil.NewServer(t,
		testutil.Reply{Body: map[string]any{"message": "success", "transaction_id": "a"}},
		testutil.Reply{Body: map[string]any{"message": "success", "transaction_id": "b"}},
	)
	gateway := newGateway(t, server)
	first := &PaymentData{OrderID: "100", CallbackURL: "https://shop.test/cb", ReturnURL: "https://shop.test/done", Description: "x", Items: []Item{{"A", myanmarpayments.Kyat(250)}}}
	second := *first

	if _, err := gateway.Initiate(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.Initiate(context.Background(), &second); err != nil {
		t.Fatal(err)
	}
	if first.ResolvedAmount().String() != "250" || first.MerchantReferenceID == "" || first.MerchantReferenceID == second.MerchantReferenceID {
		t.Fatalf("unexpected references %q %q", first.MerchantReferenceID, second.MerchantReferenceID)
	}
}

func TestCallbackHashFollowsTheDocumentedOrderWithNullAsString(t *testing.T) {
	vector := testutil.Fixture(t, "wave_money/callback.json")
	payload := vector["payload"].(map[string]any)
	payload["hashValue"] = hmacHex(vector["hash_string"].(string), vector["secret_key"].(string))
	request, _ := myanmarpayments.NewCallbackRequestFromJSON(payload, nil)

	callback, err := newGateway(t, nil).HandleCallback(request)
	if err != nil {
		t.Fatal(err)
	}
	if !callback.IsSuccessful() || callback.OrderID != "100" || callback.GatewayReference != "360" || callback.Amount != "1000" {
		t.Fatalf("unexpected callback %+v", callback)
	}
}

func TestEveryDocumentedCallbackStatusIsMapped(t *testing.T) {
	cases := map[string]myanmarpayments.PaymentStatus{
		"PAYMENT_CONFIRMED": myanmarpayments.StatusSuccessful, "INSUFFICIENT_BALANCE": myanmarpayments.StatusPending,
		"ACCOUNT_LOCKED": myanmarpayments.StatusFailed, "BILL_COLLECTION_FAILED": myanmarpayments.StatusFailed,
		"PAYMENT_REQUEST_CANCELLED": myanmarpayments.StatusCanceled, "TRANSACTION_TIMED_OUT": myanmarpayments.StatusExpired,
		"SCHEDULER_TRANSACTION_TIMED_OUT": myanmarpayments.StatusExpired, "NEW_STATUS": myanmarpayments.StatusUnknown,
	}
	for status, want := range cases {
		t.Run(status, func(t *testing.T) {
			callback, err := newGateway(t, nil).HandleCallback(signedCallback(t, map[string]any{
				"status": status, "merchantId": "testmerchantID", "orderId": "100", "amount": "1000", "merchantReferenceId": "ref-001", "timeToLiveSeconds": 300,
			}, "test-secret"))
			if err != nil {
				t.Fatal(err)
			}
			if callback.Status != want || callback.GatewayStatus != status {
				t.Fatalf("got %s want %s", callback.Status, want)
			}
		})
	}
}

func TestOrderIDFallsBackToMerchantReferenceID(t *testing.T) {
	cases := map[string]map[string]any{
		"missing": {},
		"null":    {"orderId": nil},
		"empty":   {"orderId": ""},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			payload := map[string]any{"status": "PAYMENT_CONFIRMED", "merchantReferenceId": "ref-001", "amount": "1000"}
			for key, value := range extra {
				payload[key] = value
			}
			callback, err := newGateway(t, nil).HandleCallback(signedCallback(t, payload, "test-secret"))
			if err != nil || callback.OrderID != "ref-001" {
				t.Fatalf("got %+v %v", callback, err)
			}
		})
	}
}

func TestInitiateLeavesInvalidDataUntouched(t *testing.T) {
	data := paymentData()
	data.MerchantReferenceID = ""
	data.CallbackURL = "ftp://shop.test/cb"

	var invalid *myanmarpayments.InvalidPaymentDataError
	if _, err := newGateway(t, nil).Initiate(context.Background(), data); !errors.As(err, &invalid) {
		t.Fatalf("expected InvalidPaymentDataError, got %v", err)
	}
	if data.MerchantReferenceID != "" {
		t.Fatalf("MerchantReferenceID was set to %q on invalid data", data.MerchantReferenceID)
	}
}

func TestCallbackSignedWithAnotherKeyIsRejected(t *testing.T) {
	_, err := newGateway(t, nil).HandleCallback(signedCallback(t, map[string]any{"status": "PAYMENT_CONFIRMED", "orderId": "100"}, "wrong"))
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureVerificationError, got %v", err)
	}
}

func TestValidationFollowsWaveRules(t *testing.T) {
	base := func() PaymentData {
		return PaymentData{OrderID: "100", CallbackURL: "https://shop.test/cb", ReturnURL: "https://shop.test/done", Description: "x", Items: []Item{{"A", myanmarpayments.Kyat(250)}}}
	}
	cases := map[string]struct {
		mutate func(*PaymentData)
		field  string
	}{
		"no items":           {func(d *PaymentData) { d.Items = nil }, "items"},
		"ftp callback":       {func(d *PaymentData) { d.CallbackURL = "ftp://shop.test/cb" }, "callbackUrl"},
		"zero item amount":   {func(d *PaymentData) { d.Items = []Item{{"A", myanmarpayments.Kyat(0)}} }, "items.0.amount"},
		"missing return url": {func(d *PaymentData) { d.ReturnURL = "" }, "returnUrl"},
		"decimal item":       {func(d *PaymentData) { d.Items = []Item{{"A", myanmarpayments.MustParseAmount("250.50")}} }, "items.0.amount"},
		"negative item":      {func(d *PaymentData) { d.Items = []Item{{"A", myanmarpayments.Kyat(-1)}} }, "items.0.amount"},
		"decimal total":      {func(d *PaymentData) { d.Amount = myanmarpayments.MustParseAmount("250.50") }, "amount"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			data := base()
			tc.mutate(&data)
			var invalid *myanmarpayments.InvalidPaymentDataError
			if err := data.Validate(); !errors.As(err, &invalid) || invalid.Errors[tc.field] == "" {
				t.Fatalf("expected error on %s, got %v", tc.field, err)
			}
		})
	}
}

func TestValidationAcceptsHTTPCallbackURLs(t *testing.T) {
	for _, callbackURL := range []string{"http://shop.test/cb", "https://shop.test:8443/cb"} {
		data := PaymentData{OrderID: "100", CallbackURL: callbackURL, ReturnURL: "http://shop.test/done", Description: "x", Items: []Item{{"A", myanmarpayments.Kyat(250)}}}
		if err := data.Validate(); err != nil {
			t.Fatalf("%s: unexpected error %v", callbackURL, err)
		}
	}
}

func TestDocumentedHosts(t *testing.T) {
	sandbox, production := Config{}, Config{Production: true}
	if sandbox.ResolvedBaseURL() != SandboxURL || sandbox.ResolvedAuthenticateURL() != "https://preprodpayments.wavemoney.io" {
		t.Fatal("unexpected sandbox hosts")
	}
	if production.ResolvedBaseURL() != ProductionURL || production.ResolvedAuthenticateURL() != ProductionAuthenticateURL {
		t.Fatal("unexpected production hosts")
	}
}

func TestDecimalAmountsNameWave(t *testing.T) {
	data := PaymentData{OrderID: "100", CallbackURL: "https://shop.test/cb", ReturnURL: "https://shop.test/done", Description: "x",
		Items: []Item{{"A", myanmarpayments.Kyat(250)}}, Amount: myanmarpayments.MustParseAmount("250.50")}
	var invalid *myanmarpayments.InvalidPaymentDataError
	if err := data.Validate(); !errors.As(err, &invalid) || !strings.Contains(invalid.Errors["amount"], "Wave Money does not accept decimal amounts") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestItemTotalIsExactBeyondInt64(t *testing.T) {
	data := PaymentData{Items: []Item{
		{"A", myanmarpayments.Kyat(9223372036854775807)},
		{"B", myanmarpayments.Kyat(1)},
	}}
	if got := data.ResolvedAmount().String(); got != "9223372036854775808" {
		t.Fatalf("ResolvedAmount = %s", got)
	}

	data.Items = append(data.Items, Item{"C", myanmarpayments.MustParseAmount("1.5")})
	if data.ResolvedAmount().IsSet() {
		t.Fatal("a decimal item must leave the total unset")
	}
}
