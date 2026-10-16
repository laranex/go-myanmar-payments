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
