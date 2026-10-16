package myanmarpayments_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/ayapay"
	"github.com/laranex/go-myanmar-payments/v4/cybersource"
	"github.com/laranex/go-myanmar-payments/v4/internal/testutil"
	"github.com/laranex/go-myanmar-payments/v4/internal/validate"
	"github.com/laranex/go-myanmar-payments/v4/kbzpay"
	"github.com/laranex/go-myanmar-payments/v4/wavepay"
	"github.com/laranex/go-myanmar-payments/v4/yomammqr"
)

// The vectors in testdata/parity/vectors.json are shared byte for byte with php-, node- and
// python-myanmar-payments: every SDK must give the same answer for every case.

type callbackHandler interface {
	HandleCallback(*myanmarpayments.CallbackRequest) (*myanmarpayments.PaymentCallback, error)
}

// parityGateways builds every gateway from the shared config env, with the shared secrets.
func parityGateways(t *testing.T, vectors map[string]any) map[string]callbackHandler {
	t.Helper()
	secrets := vectors["secrets"].(map[string]any)
	secret := func(name string) string { return secrets[name].(string) }
	getenv := parityEnv(vectors, "", nil)
	must := func(gateway callbackHandler, err error) callbackHandler {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return gateway
	}

	kbz := kbzpay.ConfigFromEnv(getenv)
	kbz.AppKey = secret("kbz_pay_app_key")
	wave := wavepay.ConfigFromEnv(getenv)
	wave.SecretKey = secret("wave_pay_secret_key")
	aya := ayapay.ConfigFromEnv(getenv)
	aya.AppSecret = secret("aya_pay_app_secret")
	yoma := yomammqr.ConfigFromEnv(getenv)
	yoma.WebhookHashKey, yoma.WebhookSecret = secret("yoma_mmqr_webhook_hashkey"), secret("yoma_mmqr_webhook_secret")
	cyber := cybersource.ConfigFromEnv(getenv)
	cyber.SecretKey = secret("cyber_source_secret_key")

	return map[string]callbackHandler{
		"kbz_pay":      must(kbzpay.New(kbz, nil)),
		"wave_pay":     must(wavepay.New(wave, nil)),
		"aya_pay":      must(ayapay.New(aya, nil)),
		"yoma_mmqr":    must(yomammqr.New(yoma, nil, nil)),
		"cyber_source": must(cybersource.New(cyber)),
	}
}

func TestParityCallbacks(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	gateways := parityGateways(t, vectors)

	for name, cases := range vectors["callbacks"].(map[string]any) {
		gateway, ok := gateways[name]
		if !ok {
			t.Fatalf("no gateway for %s", name)
		}
		for _, item := range cases.([]any) {
			vector := item.(map[string]any)
			t.Run(name+"/"+vector["name"].(string), func(t *testing.T) {
				header := http.Header{}
				header.Set("Content-Type", vector["content_type"].(string))
				if headers, ok := vector["headers"].(map[string]any); ok {
					for name, value := range headers {
						header.Set(name, value.(string))
					}
				}
				request := myanmarpayments.NewCallbackRequest([]byte(vector["body"].(string)), header, nil)
				expected := vector["expected"].(map[string]any)

				callback, err := gateway.HandleCallback(request)
				if expected["valid"] != true {
					var sigErr *myanmarpayments.SignatureVerificationError
					if !errors.As(err, &sigErr) {
						t.Fatalf("expected a SignatureVerificationError, got %v (%+v)", err, callback)
					}
					if sigErr.Message != expected["message"] {
						t.Fatalf("message %q, want %q", sigErr.Message, expected["message"])
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				text := func(key string) string {
					value, _ := expected[key].(string)
					return value
				}
				if callback.OrderID != text("order_id") || string(callback.Status) != text("status") ||
					callback.GatewayStatus != text("gateway_status") || callback.GatewayReference != text("gateway_reference") ||
					callback.Amount != text("amount") {
					t.Fatalf("unexpected callback %+v, want %v", callback, expected)
				}
			})
		}
	}
}

func TestParityAmounts(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")

	for _, item := range vectors["amount_parse"].([]any) {
		vector := item.(map[string]any)
		amount, err := myanmarpayments.ParseAmount(vector["input"].(string))
		if want, ok := vector["value"].(string); ok {
			if err != nil || amount.String() != want {
				t.Errorf("ParseAmount(%q) = %q, %v; want %q", vector["input"], amount, err, want)
			}
		} else if err == nil {
			t.Errorf("ParseAmount(%q) = %q; want an error", vector["input"], amount)
		}
	}

	parseError := vectors["amount_parse_error"].(map[string]any)
	_, err := myanmarpayments.ParseAmount(parseError["input"].(string))
	var invalid *myanmarpayments.InvalidPaymentDataError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected InvalidPaymentDataError, got %v", err)
	}
	for field, message := range parseError["errors"].(map[string]any) {
		if invalid.Errors[field] != message {
			t.Errorf("error %s = %q, want %q", field, invalid.Errors[field], message)
		}
	}

	for _, item := range vectors["amount_equals"].([]any) {
		vector := item.(map[string]any)
		other, _ := vector["other"].(string) // A missing (null) amount is the empty string in Go.
		if got := myanmarpayments.MustParseAmount(vector["amount"].(string)).Equals(other); got != vector["equal"] {
			t.Errorf("%s equals %q = %v, want %v", vector["amount"], other, got, vector["equal"])
		}
	}
}

// parityConfigGateway builds gateway from environment variables and returns its resolved URLs
// and time settings.
func parityConfigGateway(gateway string, vars map[string]string) (map[string]string, map[string]int, error) {
	getenv := func(key string) string { return vars[key] }
	switch gateway {
	case "kbz_pay":
		g, err := kbzpay.New(kbzpay.ConfigFromEnv(getenv), nil)
		if err != nil {
			return nil, nil, err
		}
		c := g.Config()
		return map[string]string{"api_url": c.ResolvedAPIURL(), "pwa_url": c.ResolvedPWAURL()},
			map[string]int{"timeout_in_seconds": c.TimeoutSeconds}, nil
	case "wave_pay":
		g, err := wavepay.New(wavepay.ConfigFromEnv(getenv), nil)
		if err != nil {
			return nil, nil, err
		}
		c := g.Config()
		return map[string]string{"base_url": c.ResolvedBaseURL(), "base_redirect_url": c.ResolvedBaseRedirectURL()},
			map[string]int{"time_to_live_in_seconds": c.TimeToLiveSeconds, "timeout_in_seconds": c.TimeoutSeconds}, nil
	case "aya_pay":
		g, err := ayapay.New(ayapay.ConfigFromEnv(getenv), nil)
		if err != nil {
			return nil, nil, err
		}
		c := g.Config()
		return map[string]string{"base_url": c.ResolvedBaseURL()}, map[string]int{"timeout_in_seconds": c.TimeoutSeconds}, nil
	case "yoma_mmqr":
		g, err := yomammqr.New(yomammqr.ConfigFromEnv(getenv), nil, nil)
		if err != nil {
			return nil, nil, err
		}
		c := g.Config()
		return map[string]string{"base_url": c.ResolvedBaseURL()}, map[string]int{"timeout_in_seconds": c.TimeoutSeconds}, nil
	default:
		g, err := cybersource.New(cybersource.ConfigFromEnv(getenv))
		if err != nil {
			return nil, nil, err
		}
		return map[string]string{"base_url": g.Config().ResolvedBaseURL()}, map[string]int{}, nil
	}
}

func TestParityConfig(t *testing.T) {
	config := testutil.Fixture(t, "parity/vectors.json")["config"].(map[string]any)
	baseEnv := func() map[string]string {
		vars := map[string]string{}
		for key, value := range config["env"].(map[string]any) {
			vars[key] = value.(string)
		}
		return vars
	}
	seconds := config["seconds"].(map[string]any)
	uat := config["uat"].(map[string]any)
	uatEnv := baseEnv()
	for key, value := range uat["env"].(map[string]any) {
		uatEnv[key] = value.(string)
	}

	for _, check := range []struct {
		vars map[string]string
		urls map[string]any
	}{{baseEnv(), config["urls"].(map[string]any)}, {uatEnv, uat["urls"].(map[string]any)}} {
		for gateway, want := range check.urls {
			urls, times, err := parityConfigGateway(gateway, check.vars)
			if err != nil {
				t.Fatalf("%s: %v", gateway, err)
			}
			for key, url := range want.(map[string]any) {
				if urls[key] != url {
					t.Errorf("%s %s = %q, want %q", gateway, key, urls[key], url)
				}
			}
			for key, value := range times {
				if want := seconds[key].(json.Number).String(); strconv.Itoa(value) != want {
					t.Errorf("%s %s = %d, want %v", gateway, key, value, seconds[key])
				}
			}
		}
	}

	for _, item := range config["errors"].([]any) {
		vector := item.(map[string]any)
		vars := baseEnv()
		variable := vector["variable"].(string)
		if value, ok := vector["value"].(string); ok {
			vars[variable] = value
		} else {
			delete(vars, variable)
		}
		_, _, err := parityConfigGateway(vector["gateway"].(string), vars)
		var configErr *myanmarpayments.ConfigurationError
		if !errors.As(err, &configErr) {
			t.Errorf("%s %s=%v: got %v, want a ConfigurationError", vector["gateway"], variable, vector["value"], err)
			continue
		}
		if configErr.Gateway != vector["gateway"] || configErr.Key != vector["key"] || err.Error() != "myanmarpayments: "+vector["message"].(string) {
			t.Errorf("%s %s=%v: got %+v %q, want %v", vector["gateway"], variable, vector["value"], *configErr, err, vector["message"])
		}
	}
}

func TestParityDefaultTimeout(t *testing.T) {
	config := testutil.Fixture(t, "parity/vectors.json")["config"].(map[string]any)
	vector := config["default_timeout"].(map[string]any)
	variable := vector["variable"].(string)
	want := vector["timeout_in_seconds"].(json.Number).String()

	for _, value := range vector["values"].([]any) {
		vars := map[string]string{}
		for key, value := range config["env"].(map[string]any) {
			vars[key] = value.(string)
		}
		if value, ok := value.(string); ok {
			vars[variable] = value
		} else {
			delete(vars, variable)
		}
		for _, gateway := range []string{"kbz_pay", "wave_pay", "aya_pay", "yoma_mmqr"} {
			_, times, err := parityConfigGateway(gateway, vars)
			if err != nil {
				t.Fatalf("%s %s=%v: %v", gateway, variable, value, err)
			}
			if got := strconv.Itoa(times["timeout_in_seconds"]); got != want {
				t.Errorf("%s %s=%v: timeout_in_seconds = %s, want %s", gateway, variable, value, got, want)
			}
		}
	}
}

func TestParityYomaTokenCacheKey(t *testing.T) {
	vector := testutil.Fixture(t, "parity/vectors.json")["yoma_mmqr_token_cache_key"].(map[string]any)
	cache := myanmarpayments.NewMemoryTokenCache()
	cache.Set(vector["key"].(string), "cached", 0)

	gateway, err := yomammqr.New(yomammqr.Config{
		MerchantID: "merchant", ClientID: vector["client_id"].(string), ClientSecret: "secret",
		WebhookHashKey: "hash", APIVersion: "v1rc", WebhookSecret: "webhook-secret", TimeoutSeconds: 30, BaseURL: vector["base_url"].(string),
	}, nil, cache)
	if err != nil {
		t.Fatal(err)
	}
	gateway.ForgetToken()
	if _, ok := cache.Get(vector["key"].(string)); ok {
		t.Fatal("ForgetToken did not delete the shared cache key")
	}
}

func TestParityFormHTML(t *testing.T) {
	vector := testutil.Fixture(t, "parity/vectors.json")["form_html"].(map[string]any)
	payment := myanmarpayments.FormPayment{
		OrderID: vector["order_id"].(string), Action: vector["action"].(string), Enctype: vector["enctype"].(string),
	}
	for _, pair := range vector["fields"].([]any) {
		field := pair.([]any)
		payment.Fields = append(payment.Fields, myanmarpayments.FormField{Name: field[0].(string), Value: field[1].(string)})
	}
	if got := payment.HTML(); got != vector["html"] {
		t.Fatalf("html\n got %s\nwant %s", got, vector["html"])
	}
}

func TestParityMessages(t *testing.T) {
	messages := testutil.Fixture(t, "parity/vectors.json")["messages"].(map[string]any)

	// Go errors carry the package prefix before the shared text.
	goStyle := func(message string) string { return "myanmarpayments: " + message }

	invalidData := messages["invalid_payment_data"].(map[string]any)
	errs := map[string]string{}
	for field, message := range invalidData["errors"].(map[string]any) {
		errs[field] = message.(string)
	}
	if got := (&myanmarpayments.InvalidPaymentDataError{Errors: errs}).Error(); got != goStyle(invalidData["message"].(string)) {
		t.Errorf("invalid data message %q", got)
	}

	configuration := messages["configuration"].(map[string]any)
	configErr := &myanmarpayments.ConfigurationError{Gateway: configuration["gateway"].(string), Key: configuration["key"].(string)}
	if got := configErr.Error(); got != goStyle(configuration["message"].(string)) {
		t.Errorf("configuration message %q", got)
	}
	invalidConfig := messages["configuration_invalid"].(map[string]any)
	invalidErr := &myanmarpayments.ConfigurationError{Gateway: invalidConfig["gateway"].(string), Key: invalidConfig["key"].(string), Invalid: true}
	if got := invalidErr.Error(); got != goStyle(invalidConfig["message"].(string)) {
		t.Errorf("invalid configuration message %q", got)
	}

	amountError := func(err error) string {
		var invalid *myanmarpayments.InvalidPaymentDataError
		if !errors.As(err, &invalid) {
			t.Fatalf("expected InvalidPaymentDataError, got %v", err)
		}
		return invalid.Errors["amount"]
	}
	decimal := myanmarpayments.MustParseAmount("1000.505")
	wave := wavepay.PaymentData{
		OrderID: "ORDER_1", CallbackURL: "https://shop.test/cb", ReturnURL: "https://shop.test/done", Description: "Order",
		Items: []wavepay.Item{{Name: "Shoes", Amount: myanmarpayments.Kyat(1000)}}, Amount: myanmarpayments.MustParseAmount("1000.5"),
	}
	aya := ayapay.PaymentData{OrderID: "ORDER_1", Amount: decimal, Channel: "kbz_pay", Method: ayapay.MethodQR}
	kbz := kbzpay.PaymentData{OrderID: "ORDER_1", Amount: decimal, CallbackURL: "https://shop.test/cb"}
	for got, want := range map[string]any{
		amountError(wave.Validate()): messages["whole_amounts_only"],
		amountError(aya.Validate()):  messages["aya_whole_amounts_only"],
		amountError(kbz.Validate()):  messages["kbz_decimals"],
	} {
		if got != want {
			t.Errorf("amount message %q, want %q", got, want)
		}
	}
}

// jsonDoer is a fake HTTP client that records the posted JSON request and answers with Wave's
// response (a success one unless status and body are set).
type jsonDoer struct {
	status  int
	body    string
	request *http.Request
	json    map[string]any
}

func (d *jsonDoer) Do(request *http.Request) (*http.Response, error) {
	decoder := json.NewDecoder(request.Body)
	decoder.UseNumber()
	d.request, d.json = request, nil
	if err := decoder.Decode(&d.json); err != nil {
		return nil, err
	}
	status, body := d.status, d.body
	if status == 0 {
		status, body = http.StatusOK, `{"message":"success","transaction_id":"T1"}`
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

// parityWavePayData builds a wavepay.PaymentData from a vector's data; a null or missing
// additional field is the empty string in Go.
func parityWavePayData(data map[string]any) *wavepay.PaymentData {
	text := func(key string) string {
		value, _ := data[key].(string)
		return value
	}
	payment := &wavepay.PaymentData{
		OrderID: text("order_id"), MerchantReferenceID: text("merchant_reference_id"),
		CallbackURL: text("callback_url"), ReturnURL: text("return_url"), Description: text("description"),
		AdditionalField3: text("additional_field_3"), AdditionalField4: text("additional_field_4"), AdditionalField5: text("additional_field_5"),
	}
	for _, entry := range data["items"].([]any) {
		pair := entry.([]any)
		payment.Items = append(payment.Items, wavepay.Item{Name: pair[0].(string), Amount: myanmarpayments.MustParseAmount(pair[1].(json.Number).String())})
	}
	return payment
}

func TestParityWavePayRequest(t *testing.T) {
	vector := testutil.Fixture(t, "parity/vectors.json")["wave_pay_request"].(map[string]any)
	config := vector["config"].(map[string]any)
	ttl, err := strconv.Atoi(config["time_to_live_in_seconds"].(json.Number).String())
	if err != nil {
		t.Fatal(err)
	}
	response := vector["response"].(map[string]any)
	status, err := strconv.Atoi(response["status"].(json.Number).String())
	if err != nil {
		t.Fatal(err)
	}
	responseBody, err := json.Marshal(response["body"])
	if err != nil {
		t.Fatal(err)
	}
	doer := &jsonDoer{status: status, body: string(responseBody)}
	gateway, err := wavepay.New(wavepay.Config{
		MerchantID: config["merchant_id"].(string), SecretKey: config["secret_key"].(string),
		MerchantName: config["merchant_name"].(string), TimeToLiveSeconds: ttl,
		BaseURL: config["base_url"].(string), BaseRedirectURL: config["base_redirect_url"].(string),
	}, doer)
	if err != nil {
		t.Fatal(err)
	}
	expected := vector["expected"].(map[string]any)

	for _, item := range vector["cases"].([]any) {
		c := item.(map[string]any)
		name := c["name"].(string)
		payment, err := gateway.Initiate(context.Background(), parityWavePayData(c["data"].(map[string]any)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if doer.request.Method != expected["method"] || doer.request.URL.String() != expected["url"] {
			t.Errorf("%s: request %s %s", name, doer.request.Method, doer.request.URL)
		}
		for header, want := range expected["headers"].(map[string]any) {
			if got := doer.request.Header.Get(header); got != want {
				t.Errorf("%s: header %s = %q, want %q", name, header, got, want)
			}
		}
		if got, want := fmt.Sprint(doer.json), fmt.Sprint(c["expected_body"]); got != want {
			t.Errorf("%s: body\n got %s\nwant %s", name, got, want)
		}
		if payment.URL != expected["redirect_url"] || payment.GatewayReference != expected["gateway_reference"] {
			t.Errorf("%s: payment %+v", name, payment)
		}
	}
}

func TestParityWavePayLimits(t *testing.T) {
	vector := testutil.Fixture(t, "parity/vectors.json")["wave_pay_limits"].(map[string]any)
	data := vector["data"].(map[string]any)
	set := map[string]func(*wavepay.PaymentData, string){
		"orderId":             func(d *wavepay.PaymentData, v string) { d.OrderID = v },
		"merchantReferenceId": func(d *wavepay.PaymentData, v string) { d.MerchantReferenceID = v },
		"callbackUrl":         func(d *wavepay.PaymentData, v string) { d.CallbackURL = v },
		"returnUrl":           func(d *wavepay.PaymentData, v string) { d.ReturnURL = v },
		"description":         func(d *wavepay.PaymentData, v string) { d.Description = v },
		"additionalField3":    func(d *wavepay.PaymentData, v string) { d.AdditionalField3 = v },
		"additionalField4":    func(d *wavepay.PaymentData, v string) { d.AdditionalField4 = v },
		"additionalField5":    func(d *wavepay.PaymentData, v string) { d.AdditionalField5 = v },
	}
	build := func(c map[string]any) *wavepay.PaymentData {
		setter, ok := set[c["field"].(string)]
		if !ok {
			t.Fatalf("unknown field %v", c["field"])
		}
		payment := parityWavePayData(data)
		setter(payment, c["value"].(string))
		return payment
	}

	for _, item := range vector["valid"].([]any) {
		c := item.(map[string]any)
		if err := build(c).Validate(); err != nil {
			t.Errorf("%s: unexpected error %v", c["field"], err)
		}
	}
	for _, item := range vector["invalid"].([]any) {
		c := item.(map[string]any)
		var invalid *myanmarpayments.InvalidPaymentDataError
		err := build(c).Validate()
		if !errors.As(err, &invalid) || len(invalid.Errors) != 1 || invalid.Errors[c["field"].(string)] != c["error"] {
			t.Errorf("%s: got %v, want %q", c["field"], err, c["error"])
		}
	}
}

func TestParityWavePayRequestOverrides(t *testing.T) {
	vector := testutil.Fixture(t, "parity/vectors.json")["wave_pay_request_overrides"].(map[string]any)
	config := vector["config"].(map[string]any)
	ttl, err := strconv.Atoi(config["time_to_live_in_seconds"].(json.Number).String())
	if err != nil {
		t.Fatal(err)
	}
	doer := &jsonDoer{}
	gateway, err := wavepay.New(wavepay.Config{
		MerchantID: config["merchant_id"].(string), SecretKey: config["secret_key"].(string),
		MerchantName: config["merchant_name"].(string), TimeToLiveSeconds: ttl,
		BaseURL: wavepay.ProductionURL, BaseRedirectURL: wavepay.ProductionURL,
	}, doer)
	if err != nil {
		t.Fatal(err)
	}

	// A null merchant_name means "not set", the empty string in Go; a null time_to_live_in_seconds
	// is a nil TimeToLiveSeconds, so an explicit 0 is still rejected like in the other SDKs.
	build := func(item map[string]any) *wavepay.PaymentData {
		data := vector["data"].(map[string]any)
		payment := &wavepay.PaymentData{
			OrderID: data["order_id"].(string), MerchantReferenceID: data["merchant_reference_id"].(string),
			CallbackURL: data["callback_url"].(string), ReturnURL: data["return_url"].(string), Description: data["description"].(string),
		}
		for _, entry := range data["items"].([]any) {
			pair := entry.([]any)
			payment.Items = append(payment.Items, wavepay.Item{Name: pair[0].(string), Amount: myanmarpayments.MustParseAmount(pair[1].(json.Number).String())})
		}
		if name, ok := item["merchant_name"].(string); ok {
			payment.MerchantName = name
		}
		if seconds, ok := item["time_to_live_in_seconds"].(json.Number); ok {
			value, err := strconv.Atoi(seconds.String())
			if err != nil {
				t.Fatal(err)
			}
			payment.TimeToLiveSeconds = &value
		}
		return payment
	}

	for _, item := range vector["cases"].([]any) {
		vector := item.(map[string]any)
		if _, err := gateway.Initiate(context.Background(), build(vector)); err != nil {
			t.Fatalf("%v: %v", vector, err)
		}
		for field, want := range vector["expected"].(map[string]any) {
			if got := doer.json[field]; got != want {
				t.Errorf("%v: %s = %v, want %v", vector, field, got, want)
			}
		}
	}

	fields := map[string]string{"merchant_name": "merchantName", "time_to_live_in_seconds": "timeToLiveSeconds"}
	for _, item := range vector["invalid"].([]any) {
		vector := item.(map[string]any)
		_, err := gateway.Initiate(context.Background(), build(vector))
		var invalid *myanmarpayments.InvalidPaymentDataError
		field := fields[vector["setting"].(string)]
		if !errors.As(err, &invalid) || invalid.Errors[field] == "" {
			t.Errorf("%v: got %v, want an error on %s", vector, err, field)
		}
	}
}

type kbzDoer struct{ biz map[string]any }

func (d *kbzDoer) Do(request *http.Request) (*http.Response, error) {
	var body struct {
		Request struct {
			BizContent map[string]any `json:"biz_content"`
		}
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return nil, err
	}
	d.biz = body.Request.BizContent
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"Response":{"result":"SUCCESS","code":"0","msg":"success","prepay_id":"PREPAY123","qrCode":"qr"}}`)),
	}, nil
}

// parityEnv returns the shared config env without variable, then with it set to value when value
// is a string (a JSON null leaves it unset).
func parityEnv(vectors map[string]any, variable string, value any) func(string) string {
	vars := map[string]string{}
	for key, v := range vectors["config"].(map[string]any)["env"].(map[string]any) {
		if key != variable {
			vars[key] = v.(string)
		}
	}
	if text, ok := value.(string); ok {
		vars[variable] = text
	}
	return func(key string) string { return vars[key] }
}

func TestParityKbzPayTimeToLive(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	vector := vectors["kbz_pay_time_to_live"].(map[string]any)
	variable := vector["variable"].(string)
	for _, item := range vector["cases"].([]any) {
		c := item.(map[string]any)
		doer := &kbzDoer{}
		gateway, err := kbzpay.New(kbzpay.ConfigFromEnv(parityEnv(vectors, variable, c["config"])), doer)
		if err != nil {
			t.Fatal(err)
		}
		data := kbzpay.PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb"}
		if minutes, ok := c["payment"].(json.Number); ok {
			ttl, err := strconv.Atoi(minutes.String())
			if err != nil {
				t.Fatal(err)
			}
			data.TimeToLiveMinutes = &ttl
		}
		payment, err := gateway.QR(context.Background(), data)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := doer.biz["timeout_express"].(string); got != c["timeout_express"] || payment.ExpiresAt.IsZero() {
			t.Errorf("case %v: timeout_express %q (expires %v)", c, got, payment.ExpiresAt)
		}
	}
}

// queueDoer is a fake HTTP client that answers with its responses in order (the last one again
// once they run out) and records every request body.
type queueDoer struct {
	responses []queuedResponse
	bodies    [][]byte
}

type queuedResponse struct {
	status int
	body   string
}

func (d *queueDoer) Do(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	d.bodies = append(d.bodies, body)
	response := d.responses[min(len(d.bodies), len(d.responses))-1]
	return &http.Response{
		StatusCode: response.status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(response.body)),
	}, nil
}

// last returns the last request body.
func (d *queueDoer) last() []byte { return d.bodies[len(d.bodies)-1] }

func parityInt(t *testing.T, value any) int {
	t.Helper()
	number, err := strconv.Atoi(value.(json.Number).String())
	if err != nil {
		t.Fatal(err)
	}
	return number
}

// parityAmount reads a vector amount: a string is parsed as a decimal, a number is whole kyat
// (negative ones included) and null is unset.
func parityAmount(t *testing.T, value any) myanmarpayments.Amount {
	t.Helper()
	switch amount := value.(type) {
	case string:
		return myanmarpayments.MustParseAmount(amount)
	case json.Number:
		n, err := amount.Int64()
		if err != nil {
			t.Fatal(err)
		}
		return myanmarpayments.Kyat(n)
	default:
		return myanmarpayments.Amount{}
	}
}

// parityErrors returns the field errors of err, or nil when err is nil.
func parityErrors(t *testing.T, err error) map[string]string {
	t.Helper()
	if err == nil {
		return nil
	}
	var invalid *myanmarpayments.InvalidPaymentDataError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected an InvalidPaymentDataError, got %v", err)
	}
	return invalid.Errors
}

func parityExpectErrors(t *testing.T, name string, err error, want any) {
	t.Helper()
	got := parityErrors(t, err)
	expected := map[string]string{}
	for field, message := range want.(map[string]any) {
		expected[field] = message.(string)
	}
	if fmt.Sprint(got) != fmt.Sprint(expected) {
		t.Errorf("%s: errors %v, want %v", name, got, expected)
	}
}

// parityAPIError checks an *APIError against a vector's message, gateway_code and
// gateway_message (null is the empty string in Go).
func parityAPIError(t *testing.T, name string, err error, want map[string]any) {
	t.Helper()
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("%s: expected an APIError, got %v", name, err)
	}
	code, _ := want["gateway_code"].(string)
	message, _ := want["gateway_message"].(string)
	if apiErr.Message != want["message"] || apiErr.GatewayCode != code || apiErr.GatewayMessage != message {
		t.Errorf("%s: got %q %q %q, want %v", name, apiErr.Message, apiErr.GatewayCode, apiErr.GatewayMessage, want)
	}
}

// parityVars returns the shared config env with overrides applied (a JSON null removes the
// variable).
func parityVars(vectors map[string]any, overrides map[string]any) map[string]string {
	vars := map[string]string{}
	for key, value := range vectors["config"].(map[string]any)["env"].(map[string]any) {
		vars[key] = value.(string)
	}
	for key, value := range overrides {
		if text, ok := value.(string); ok {
			vars[key] = text
		} else {
			delete(vars, key)
		}
	}
	return vars
}

func TestParityURLValidation(t *testing.T) {
	vector := testutil.Fixture(t, "parity/vectors.json")["url_validation"].(map[string]any)
	field := vector["field"].(string)
	for _, item := range vector["cases"].([]any) {
		c := item.(map[string]any)
		got := parityErrors(t, validate.New().URL(field, c["url"].(string)).Err())
		if c["valid"] == true && got != nil || c["valid"] != true && got[field] != vector["message"] {
			t.Errorf("%q: errors %v, want valid %v", c["url"], got, c["valid"])
		}
	}
}

func TestParityQueryEscape(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	getenv := parityEnv(vectors, "", nil)
	for _, item := range vectors["query_escape"].(map[string]any)["cases"].([]any) {
		c := item.(map[string]any)
		input, escaped := c["input"].(string), c["escaped"].(string)

		kbzDoer := &queueDoer{responses: []queuedResponse{{200, `{"Response":{"result":"SUCCESS","code":"0","prepay_id":"P1","qrCode":"qr"}}`}}}
		kbz, err := kbzpay.New(kbzpay.ConfigFromEnv(getenv), kbzDoer)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := kbz.QR(context.Background(), kbzpay.PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb", CallbackInfo: input}); err != nil {
			t.Fatal(err)
		}
		var sent struct {
			Request struct {
				BizContent map[string]string `json:"biz_content"`
			}
		}
		if err := json.Unmarshal(kbzDoer.last(), &sent); err != nil {
			t.Fatal(err)
		}
		if got := sent.Request.BizContent["callback_info"]; got != escaped {
			t.Errorf("callback_info %q, want %q", got, escaped)
		}

		response, _ := json.Marshal(map[string]string{"message": "success", "transaction_id": input})
		wave, err := wavepay.New(wavepay.ConfigFromEnv(getenv), &queueDoer{responses: []queuedResponse{{200, string(response)}}})
		if err != nil {
			t.Fatal(err)
		}
		payment, err := wave.Initiate(context.Background(), parityWavePayData(vectors["wave_pay_limits"].(map[string]any)["data"].(map[string]any)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(payment.URL, "/authenticate?transaction_id="+escaped) {
			t.Errorf("redirect %q, want transaction_id=%s", payment.URL, escaped)
		}
	}
}

func TestParityASCIITrim(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	vector := vectors["ascii_trim"].(map[string]any)
	for _, item := range vector["env"].([]any) {
		c := item.(map[string]any)
		gateway, err := kbzpay.New(kbzpay.ConfigFromEnv(parityEnv(vectors, "KBZ_PAY_APP_ID", c["value"])), nil)
		if want, ok := c["app_id"].(string); ok {
			if err != nil || gateway.Config().AppID != want {
				t.Errorf("%q: app id %v, want %q", c["value"], err, want)
			}
		} else if err == nil || err.Error() != "myanmarpayments: "+c["message"].(string) {
			t.Errorf("%q: got %v, want %v", c["value"], err, c["message"])
		}
	}
	for _, item := range vector["kbz_pay_order_id"].([]any) {
		c := item.(map[string]any)
		data := kbzpay.PaymentData{OrderID: c["order_id"].(string), Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb"}
		parityExpectErrors(t, fmt.Sprintf("%q", c["order_id"]), data.Validate(), c["errors"])
	}
}

// Go settings are typed, so only the text cases of config_setting_types apply: integers,
// floats and booleans cannot be given as a string setting.
func TestParityConfigSettingTypes(t *testing.T) {
	vector := testutil.Fixture(t, "parity/vectors.json")["config_setting_types"].(map[string]any)
	config := vector["config"].(map[string]any)
	for _, item := range vector["cases"].([]any) {
		c := item.(map[string]any)
		value, ok := c["value"].(string)
		if !ok {
			continue
		}
		gateway, err := cybersource.New(cybersource.Config{
			ProfileID: value, AccessKey: config["access_key"].(string), SecretKey: config["secret_key"].(string), BaseURL: config["base_url"].(string),
		})
		if want, ok := c["result"].(string); ok {
			if err != nil || gateway.Config().ProfileID != want {
				t.Errorf("%q: %v, want %q", value, err, want)
			}
		} else if err == nil || err.Error() != "myanmarpayments: "+vector["message"].(string) {
			t.Errorf("%q: got %v, want %v", value, err, vector["message"])
		}
	}
}

func parityConfigError(t *testing.T, name string, err error, gateway, key, message any) {
	t.Helper()
	var configErr *myanmarpayments.ConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("%s: got %v, want a ConfigurationError", name, err)
	}
	if configErr.Gateway != gateway || configErr.Key != key || err.Error() != "myanmarpayments: "+message.(string) {
		t.Errorf("%s: got %s %s %q, want %v %v %v", name, configErr.Gateway, configErr.Key, err, gateway, key, message)
	}
}

func TestParityConfigIntegerLimits(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	for _, item := range vectors["config_integer_limits"].(map[string]any)["cases"].([]any) {
		c := item.(map[string]any)
		variable := c["variable"].(string)
		_, times, err := parityConfigGateway(c["gateway"].(string), parityVars(vectors, map[string]any{variable: c["value"]}))
		name := fmt.Sprintf("%s %s=%s", c["gateway"], variable, c["value"])
		if seconds, ok := c["seconds"]; ok {
			key := "timeout_in_seconds"
			if variable == "WAVE_PAY_TIME_TO_LIVE_IN_SECONDS" {
				key = "time_to_live_in_seconds"
			}
			if err != nil || times[key] != parityInt(t, seconds) {
				t.Errorf("%s: %v %v, want %v", name, err, times, seconds)
			}
			continue
		}
		parityConfigError(t, name, err, c["gateway"], c["key"], c["message"])
	}
}

func TestParityConfigValidationOrder(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	for _, item := range vectors["config_validation_order"].(map[string]any)["cases"].([]any) {
		c := item.(map[string]any)
		_, _, err := parityConfigGateway(c["gateway"].(string), parityVars(vectors, c["env"].(map[string]any)))
		parityConfigError(t, fmt.Sprintf("%s %v", c["gateway"], c["env"]), err, c["gateway"], c["key"], c["message"])
	}
}

// parityWavePayCase builds Wave Pay payment data from snake_case vector data: items are
// [name, amount] pairs and amount, merchant_name and time_to_live_in_seconds are optional.
func parityWavePayCase(t *testing.T, data map[string]any) *wavepay.PaymentData {
	t.Helper()
	text := func(key string) string {
		value, _ := data[key].(string)
		return value
	}
	payment := &wavepay.PaymentData{
		OrderID: text("order_id"), MerchantReferenceID: text("merchant_reference_id"),
		CallbackURL: text("callback_url"), ReturnURL: text("return_url"), Description: text("description"),
		MerchantName: text("merchant_name"), Amount: parityAmount(t, data["amount"]),
		AdditionalField3: text("additional_field_3"), AdditionalField4: text("additional_field_4"), AdditionalField5: text("additional_field_5"),
	}
	for _, entry := range data["items"].([]any) {
		pair := entry.([]any)
		payment.Items = append(payment.Items, wavepay.Item{Name: pair[0].(string), Amount: parityAmount(t, pair[1])})
	}
	if seconds, ok := data["time_to_live_in_seconds"]; ok {
		value := parityInt(t, seconds)
		payment.TimeToLiveSeconds = &value
	}
	return payment
}

func TestParityNegativeAmounts(t *testing.T) {
	for _, item := range testutil.Fixture(t, "parity/vectors.json")["negative_amounts"].(map[string]any)["cases"].([]any) {
		c := item.(map[string]any)
		data := c["data"].(map[string]any)
		text := func(key string) string { return data[key].(string) }
		var err error
		switch c["gateway"] {
		case "kbz_pay":
			err = kbzpay.PaymentData{OrderID: text("order_id"), Amount: parityAmount(t, data["amount"]), CallbackURL: text("callback_url")}.Validate()
		case "wave_pay":
			err = parityWavePayCase(t, data).Validate()
		case "aya_pay":
			err = ayapay.PaymentData{OrderID: text("order_id"), Amount: parityAmount(t, data["amount"]), Channel: text("channel"), Method: ayapay.Method(text("method"))}.Validate()
		case "yoma_mmqr":
			err = yomammqr.PaymentData{OrderID: text("order_id"), Amount: parityAmount(t, data["amount"]), Description: text("description")}.Validate()
		case "cyber_source":
			err = cybersource.PaymentData{
				OrderID: text("order_id"), Amount: parityAmount(t, data["amount"]), CallbackURL: text("callback_url"), Currency: text("currency"),
				TransactionType: cybersource.TransactionType(text("transaction_type")), Locale: text("locale"),
			}.Validate()
		default:
			t.Fatalf("unknown gateway %v", c["gateway"])
		}
		parityExpectErrors(t, fmt.Sprint(c["gateway"], data), err, c["errors"])
	}
}

func TestParityWavePayValidation(t *testing.T) {
	vector := testutil.Fixture(t, "parity/vectors.json")["wave_pay_validation"].(map[string]any)
	for _, item := range vector["cases"].([]any) {
		c := item.(map[string]any)
		data := map[string]any{}
		for key, value := range vector["data"].(map[string]any) {
			data[key] = value
		}
		for key, value := range c["data"].(map[string]any) {
			data[key] = value
		}
		parityExpectErrors(t, c["name"].(string), parityWavePayCase(t, data).Validate(), c["errors"])
	}
}

func parityWavePayGateway(t *testing.T, config map[string]any, doer myanmarpayments.HTTPDoer) *wavepay.Gateway {
	t.Helper()
	gateway, err := wavepay.New(wavepay.Config{
		MerchantID: config["merchant_id"].(string), SecretKey: config["secret_key"].(string),
		MerchantName: config["merchant_name"].(string), TimeToLiveSeconds: parityInt(t, config["time_to_live_in_seconds"]),
		BaseURL: config["base_url"].(string), BaseRedirectURL: config["base_redirect_url"].(string),
	}, doer)
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

func TestParityWavePayExactAmount(t *testing.T) {
	vector := testutil.Fixture(t, "parity/vectors.json")["wave_pay_exact_amount"].(map[string]any)
	doer := &queueDoer{responses: []queuedResponse{{200, `{"message":"success","transaction_id":"T1"}`}}}
	gateway := parityWavePayGateway(t, vector["config"].(map[string]any), doer)
	if _, err := gateway.Initiate(context.Background(), parityWavePayCase(t, vector["data"].(map[string]any))); err != nil {
		t.Fatal(err)
	}

	expected := vector["expected"].(map[string]any)
	var sent map[string]any
	if err := json.Unmarshal(doer.last(), &sent); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doer.last()), `"amount":`+expected["amount_json"].(string)+`,`) {
		t.Errorf("body %s, want amount %s", doer.last(), expected["amount_json"])
	}
	if sent["items"] != expected["items"] || sent["hash"] != expected["hash"] {
		t.Errorf("items %v hash %v, want %v", sent["items"], sent["hash"], expected)
	}
}

func TestParityWavePayErrors(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	config := vectors["wave_pay_request"].(map[string]any)["config"].(map[string]any)
	data := vectors["wave_pay_limits"].(map[string]any)["data"].(map[string]any)
	for _, item := range vectors["wave_pay_errors"].(map[string]any)["cases"].([]any) {
		c := item.(map[string]any)
		status := parityInt(t, c["status"])
		gateway := parityWavePayGateway(t, config, &queueDoer{responses: []queuedResponse{{status, c["body"].(string)}}})
		_, err := gateway.Initiate(context.Background(), parityWavePayData(data))
		parityAPIError(t, c["body"].(string), err, map[string]any{
			"message":         fmt.Sprintf("Wave Pay payment request failed with HTTP %d: %s", status, c["gateway_message"]),
			"gateway_code":    c["gateway_code"],
			"gateway_message": c["gateway_message"],
		})
	}
}

func TestParityAyaPayServices(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	for _, item := range vectors["aya_pay_services"].(map[string]any)["cases"].([]any) {
		c := item.(map[string]any)
		status := http.StatusOK
		if value, ok := c["status"]; ok {
			status = parityInt(t, value)
		}
		gateway, err := ayapay.New(ayapay.ConfigFromEnv(parityEnv(vectors, "", nil)), &queueDoer{responses: []queuedResponse{{status, c["body"].(string)}}})
		if err != nil {
			t.Fatal(err)
		}
		name := c["name"].(string)
		services, err := gateway.Services(context.Background())
		if want, ok := c["error"].(map[string]any); ok {
			parityAPIError(t, name, err, want)
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := make([]map[string]any, len(services))
		for i, service := range services {
			methods := []any{}
			for _, method := range service.Methods {
				methods = append(methods, string(method))
			}
			unknown := []any{}
			for _, method := range service.UnknownMethods {
				unknown = append(unknown, method)
			}
			got[i] = map[string]any{"name": service.Name, "key": service.Key, "image_url": service.ImageURL, "methods": methods, "unknown_methods": unknown}
		}
		want := make([]map[string]any, 0)
		for _, entry := range c["services"].([]any) {
			service := entry.(map[string]any)
			if service["image_url"] == nil {
				service["image_url"] = "" // A missing image is the empty string in Go.
			}
			want = append(want, service)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: services\n got %v\nwant %v", name, got, want)
		}
	}
}

func TestParityYomaMmqrResponses(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	for _, item := range vectors["yoma_mmqr_responses"].(map[string]any)["cases"].([]any) {
		c := item.(map[string]any)
		doer := &queueDoer{}
		for _, entry := range c["responses"].([]any) {
			response := entry.(map[string]any)
			doer.responses = append(doer.responses, queuedResponse{parityInt(t, response["status"]), response["body"].(string)})
		}
		gateway, err := yomammqr.New(yomammqr.ConfigFromEnv(parityEnv(vectors, "", nil)), doer, nil)
		if err != nil {
			t.Fatal(err)
		}
		name, argument := c["name"].(string), c["argument"].(string)
		var got map[string]any
		if c["call"] == "status" {
			var result *myanmarpayments.PaymentStatusResult
			if result, err = gateway.Status(context.Background(), argument); err == nil {
				got = map[string]any{"status": string(result.Status), "gateway_status": result.GatewayStatus, "gateway_reference": result.GatewayReference}
			}
		} else {
			var payment *myanmarpayments.QrPayment
			if payment, err = gateway.RenewQR(context.Background(), argument); err == nil {
				got = map[string]any{"qr_image": payment.QRImage, "reference": payment.Reference}
			}
		}
		if want, ok := c["error"].(map[string]any); ok {
			parityAPIError(t, name, err, want)
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if fmt.Sprint(got) != fmt.Sprint(c["result"]) {
			t.Errorf("%s: got %v, want %v", name, got, c["result"])
		}
	}
}

func TestParityKbzPayResponses(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	for _, item := range vectors["kbz_pay_responses"].(map[string]any)["cases"].([]any) {
		c := item.(map[string]any)
		gateway, err := kbzpay.New(kbzpay.ConfigFromEnv(parityEnv(vectors, "", nil)), &queueDoer{responses: []queuedResponse{{200, c["body"].(string)}}})
		if err != nil {
			t.Fatal(err)
		}
		name := c["name"].(string)
		var got map[string]any
		if c["call"] == "status" {
			var result *myanmarpayments.PaymentStatusResult
			if result, err = gateway.Status(context.Background(), "ORDER_1"); err == nil {
				got = map[string]any{
					"order_id": result.OrderID, "status": string(result.Status), "gateway_status": result.GatewayStatus,
					"gateway_reference": result.GatewayReference, "amount": result.Amount,
				}
			}
		} else {
			var payment *myanmarpayments.QrPayment
			if payment, err = gateway.QR(context.Background(), kbzpay.PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb"}); err == nil {
				got = map[string]any{"qr_string": payment.QRString, "reference": payment.Reference}
			}
		}
		if want, ok := c["error"].(map[string]any); ok {
			parityAPIError(t, name, err, want)
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := map[string]any{}
		for key, value := range c["result"].(map[string]any) {
			if value == nil {
				value = "" // A missing value is the empty string in Go.
			}
			want[key] = value
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
}
