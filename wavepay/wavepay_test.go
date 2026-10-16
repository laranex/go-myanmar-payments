package wavepay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/testutil"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

func newGateway(t *testing.T, server *testutil.Server) *Gateway {
	t.Helper()
	config := testConfig()
	if server != nil {
		config.BaseURL = server.URL
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

func seconds(value int) *int { return &value }

func hmacHex(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// requestBody decodes a recorded JSON request body, keeping numbers as json.Number.
func requestBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("request body is not JSON: %v (%s)", err, body)
	}
	return decoded
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

func TestInitiatePostsAHashedJSONRequestAndRedirectsToAuthenticate(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"message": "success", "transaction_id": "enc/123+abc"}})

	payment, err := newGateway(t, server).Initiate(context.Background(), paymentData())
	if err != nil {
		t.Fatal(err)
	}

	recorded := server.Last(t)
	if recorded.Method != "POST" || recorded.Path != "/payment" || recorded.Header.Get("Content-Type") != "application/json" || recorded.Header.Get("Accept") != "application/json" {
		t.Fatalf("unexpected request %s %s %s %s", recorded.Method, recorded.Path, recorded.Header.Get("Content-Type"), recorded.Header.Get("Accept"))
	}
	body := requestBody(t, recorded.Body)
	want := map[string]any{
		"merchant_id":             "testmerchantID",
		"order_id":                "100",
		"merchant_reference_id":   "ref-001",
		"frontend_result_url":     "https://shop.test/done",
		"backend_result_url":      "https://shop.test/wave/callback",
		"amount":                  json.Number("1000"),
		"time_to_live_in_seconds": json.Number("300"),
		"payment_description":     "Order 100",
		"currency":                "MMK",
		"hash":                    hmacHex("300testmerchantID1001000https://shop.test/wave/callbackref-001", "test-secret"),
		"merchant_name":           "Shop",
		"items":                   `[{"name":"Shoes","amount":600},{"name":"Socks","amount":400}]`,
	}
	if fmt.Sprint(body) != fmt.Sprint(want) {
		t.Fatalf("unexpected body\n got %v\nwant %v", body, want)
	}
	if !bytes.Contains(recorded.Body, []byte(`"amount":1000,`)) || !bytes.Contains(recorded.Body, []byte(`"time_to_live_in_seconds":300,`)) {
		t.Fatalf("amount and time_to_live_in_seconds must be JSON integers: %s", recorded.Body)
	}
	if payment.URL != ProductionURL+"/authenticate?transaction_id=enc%2F123%2Babc" || payment.GatewayReference != "enc/123+abc" {
		t.Fatalf("unexpected payment %+v", payment)
	}
}

func TestInitiateSendsTheAdditionalFieldsOnlyWhenSet(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"message": "success", "transaction_id": "T1"}})
	data := paymentData()
	data.AdditionalField3, data.AdditionalField5 = "Info 1", "Info 3"

	if _, err := newGateway(t, server).Initiate(context.Background(), data); err != nil {
		t.Fatal(err)
	}

	body := requestBody(t, server.Last(t).Body)
	if body["additional_field_3"] != "Info 1" || body["additional_field_5"] != "Info 3" {
		t.Fatalf("unexpected body %v", body)
	}
	if _, ok := body["additional_field_4"]; ok {
		t.Fatalf("an empty additional field must not be sent: %v", body)
	}
}

func TestPaymentDataEnforcesTheDocumentedFieldLimits(t *testing.T) {
	cases := []struct {
		field  string
		max    int
		mutate func(*PaymentData, string)
		value  string
	}{
		{"orderId", 100, func(d *PaymentData, v string) { d.OrderID = v }, strings.Repeat("1", 101)},
		{"merchantReferenceId", 100, func(d *PaymentData, v string) { d.MerchantReferenceID = v }, strings.Repeat("r", 101)},
		{"callbackUrl", 255, func(d *PaymentData, v string) { d.CallbackURL = v }, "https://shop.test/" + strings.Repeat("a", 238)},
		{"returnUrl", 255, func(d *PaymentData, v string) { d.ReturnURL = v }, "https://shop.test/" + strings.Repeat("a", 238)},
		{"description", 255, func(d *PaymentData, v string) { d.Description = v }, strings.Repeat("d", 256)},
		{"additionalField3", 50, func(d *PaymentData, v string) { d.AdditionalField3 = v }, strings.Repeat("x", 51)},
		{"additionalField4", 50, func(d *PaymentData, v string) { d.AdditionalField4 = v }, strings.Repeat("x", 51)},
		{"additionalField5", 50, func(d *PaymentData, v string) { d.AdditionalField5 = v }, strings.Repeat("x", 51)},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			data := paymentData()
			tc.mutate(data, tc.value[:len(tc.value)-1])
			if err := data.Validate(); err != nil {
				t.Fatalf("a value of %d bytes must pass: %v", tc.max, err)
			}
			tc.mutate(data, tc.value)
			var invalid *myanmarpayments.InvalidPaymentDataError
			want := map[string]string{tc.field: fmt.Sprintf("The %s field must not be greater than %d characters.", tc.field, tc.max)}
			if err := data.Validate(); !errors.As(err, &invalid) || fmt.Sprint(invalid.Errors) != fmt.Sprint(want) {
				t.Fatalf("expected %v, got %v", want, err)
			}
		})
	}
}

func TestInitiateRequestsAtTheBaseURLAndRedirectsToTheBaseRedirectURL(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"message": "success", "transaction_id": "T1"}})
	config := testConfig()
	config.BaseURL = server.URL + "/"
	config.BaseRedirectURL = "https://preprodpayments.wavemoney.io:8107/"
	gateway, err := New(config, nil)
	if err != nil {
		t.Fatal(err)
	}

	payment, err := gateway.Initiate(context.Background(), paymentData())
	if err != nil {
		t.Fatal(err)
	}
	if recorded := server.Last(t); recorded.Path != "/payment" {
		t.Fatalf("expected the payment request at the base URL's /payment, got %s", recorded.Path)
	}
	if payment.URL != "https://preprodpayments.wavemoney.io:8107/authenticate?transaction_id=T1" {
		t.Fatalf("expected the redirect at the base redirect URL's /authenticate, got %s", payment.URL)
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
	vector := testutil.Fixture(t, "wave_pay/callback.json")
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
		"PAYMENT_CONFIRMED": myanmarpayments.StatusSuccessful, "INSUFFICIENT_BALANCE": myanmarpayments.StatusFailed,
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
		"no items":               {func(d *PaymentData) { d.Items = nil }, "items"},
		"ftp callback":           {func(d *PaymentData) { d.CallbackURL = "ftp://shop.test/cb" }, "callbackUrl"},
		"zero item amount":       {func(d *PaymentData) { d.Items = []Item{{"A", myanmarpayments.Kyat(0)}} }, "items.0.amount"},
		"missing return url":     {func(d *PaymentData) { d.ReturnURL = "" }, "returnUrl"},
		"decimal item":           {func(d *PaymentData) { d.Items = []Item{{"A", myanmarpayments.MustParseAmount("250.50")}} }, "items.0.amount"},
		"negative item":          {func(d *PaymentData) { d.Items = []Item{{"A", myanmarpayments.Kyat(-1)}} }, "items.0.amount"},
		"decimal total":          {func(d *PaymentData) { d.Amount = myanmarpayments.MustParseAmount("250.50") }, "amount"},
		"order id with a dot":    {func(d *PaymentData) { d.OrderID = "ORDER.1" }, "orderId"},
		"reference with a space": {func(d *PaymentData) { d.MerchantReferenceID = "ref 1" }, "merchantReferenceId"},
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

func TestResolvedURLsTrimTheTrailingSlashWithoutAProductionFallback(t *testing.T) {
	if (Config{}).ResolvedBaseURL() != "" || (Config{}).ResolvedBaseRedirectURL() != "" {
		t.Fatal("expected no production fallback")
	}
	config := Config{BaseURL: ProductionURL + "/", BaseRedirectURL: ProductionURL + "/"}
	if config.ResolvedBaseURL() != ProductionURL || config.ResolvedBaseRedirectURL() != ProductionURL {
		t.Fatal("unexpected production hosts")
	}
}

func TestDecimalAmountsNameWave(t *testing.T) {
	data := PaymentData{OrderID: "100", CallbackURL: "https://shop.test/cb", ReturnURL: "https://shop.test/done", Description: "x",
		Items: []Item{{"A", myanmarpayments.Kyat(250)}}, Amount: myanmarpayments.MustParseAmount("250.50")}
	var invalid *myanmarpayments.InvalidPaymentDataError
	if err := data.Validate(); !errors.As(err, &invalid) || !strings.Contains(invalid.Errors["amount"], "Wave Pay does not accept decimal amounts") {
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

func TestItemNamesAreNotHTMLEscaped(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"message": "success", "transaction_id": "T1"}})
	data := paymentData()
	data.Items = []Item{{Name: "Tea & <Cake>", Amount: myanmarpayments.Kyat(1000)}}
	data.Amount = myanmarpayments.Amount{}
	if _, err := newGateway(t, server).Initiate(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if items := requestBody(t, server.Last(t).Body)["items"]; items != `[{"name":"Tea & <Cake>","amount":1000}]` {
		t.Fatalf("unexpected items %v", items)
	}
}

func TestNestedHashedFieldFailsVerification(t *testing.T) {
	payload := map[string]any{"status": "PAYMENT_CONFIRMED", "orderId": "100", "requestTime": map[string]any{"at": "now"}}
	parts := ""
	for _, field := range callbackFields {
		value, ok := values.String(payload[field])
		if !ok {
			value = "null"
		}
		parts += value
	}
	payload["hashValue"] = hmacHex(parts, "test-secret")
	request, err := myanmarpayments.NewCallbackRequestFromJSON(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newGateway(t, nil).HandleCallback(request)
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureVerificationError, got %v", err)
	}
}

func TestPaymentOverridesReplaceTheConfiguredNameAndTimeToLive(t *testing.T) {
	cases := map[string]struct {
		merchantName string
		ttl          *int
		wantName     string
		wantTTL      string
	}{
		"defaults":          {"", nil, "Shop", "300"},
		"both overridden":   {"Tikkat", seconds(600), "Tikkat", "600"},
		"name only":         {"Tikkat", nil, "Tikkat", "300"},
		"time to live only": {"", seconds(60), "Shop", "60"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"message": "success", "transaction_id": "T1"}})
			gateway := newGateway(t, server)
			data := paymentData()
			data.MerchantName, data.TimeToLiveSeconds = tc.merchantName, tc.ttl

			if _, err := gateway.Initiate(context.Background(), data); err != nil {
				t.Fatal(err)
			}

			body := requestBody(t, server.Last(t).Body)
			if body["merchant_name"] != tc.wantName || body["time_to_live_in_seconds"] != json.Number(tc.wantTTL) {
				t.Fatalf("unexpected body %v", body)
			}
			if body["hash"] != hmacHex(tc.wantTTL+"testmerchantID1001000https://shop.test/wave/callbackref-001", "test-secret") {
				t.Fatal("request hash does not use the time to live in effect")
			}
			if gateway.Config().MerchantName != "Shop" || gateway.Config().TimeToLiveSeconds != 300 {
				t.Fatalf("the override changed the configuration %+v", gateway.Config())
			}
		})
	}
}

func TestInvalidPaymentOverridesAreRejected(t *testing.T) {
	cases := map[string]struct {
		mutate  func(*PaymentData)
		field   string
		message string
	}{
		"blank merchant name":   {func(d *PaymentData) { d.MerchantName = " \t " }, "merchantName", "The merchantName field is required."},
		"zero time to live":     {func(d *PaymentData) { d.TimeToLiveSeconds = seconds(0) }, "timeToLiveSeconds", "The timeToLiveSeconds field must be greater than 0."},
		"negative time to live": {func(d *PaymentData) { d.TimeToLiveSeconds = seconds(-5) }, "timeToLiveSeconds", "The timeToLiveSeconds field must be greater than 0."},
		"negative with bad items": {func(d *PaymentData) {
			d.TimeToLiveSeconds = seconds(-1)
			d.Items = []Item{{"A", myanmarpayments.Kyat(-1)}}
		}, "timeToLiveSeconds", "The timeToLiveSeconds field must be greater than 0."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			data := paymentData()
			tc.mutate(data)
			var invalid *myanmarpayments.InvalidPaymentDataError
			if _, err := newGateway(t, nil).Initiate(context.Background(), data); !errors.As(err, &invalid) || invalid.Errors[tc.field] != tc.message {
				t.Fatalf("expected %q on %s, got %v", tc.message, tc.field, err)
			}
		})
	}
}

func TestCallbacksWithMalformedValuesFailVerification(t *testing.T) {
	cases := map[string]map[string]any{
		"missing amount":           {"amount": nil},
		"negative amount":          {"amount": "-1000"},
		"amount with a comma":      {"amount": "1,000"},
		"amount with two dots":     {"amount": "1.0.0"},
		"order id with a space":    {"orderId": "ORDER 1"},
		"order id with a colon":    {"orderId": "ORDER:1"},
		"reference with a slash":   {"merchantReferenceId": "ref/1"},
		"neither id":               {"orderId": "", "merchantReferenceId": nil},
		"fallback reference wrong": {"orderId": nil, "merchantReferenceId": "ref 1"},
	}
	for name, overrides := range cases {
		t.Run(name, func(t *testing.T) {
			payload := map[string]any{"status": "PAYMENT_CONFIRMED", "orderId": "ORDER_1", "merchantReferenceId": "ref-001", "amount": "1000"}
			for key, value := range overrides {
				payload[key] = value
			}
			var sigErr *myanmarpayments.SignatureVerificationError
			if _, err := newGateway(t, nil).HandleCallback(signedCallback(t, payload, "test-secret")); !errors.As(err, &sigErr) {
				t.Fatalf("expected SignatureVerificationError, got %v", err)
			}
		})
	}

	payload := map[string]any{"status": "PAYMENT_CONFIRMED", "orderId": "ORDER_1", "merchantReferenceId": "ref-001", "amount": "1000.50"}
	if callback, err := newGateway(t, nil).HandleCallback(signedCallback(t, payload, "test-secret")); err != nil || callback.OrderID != "ORDER_1" {
		t.Fatalf("got %+v %v", callback, err)
	}
}
