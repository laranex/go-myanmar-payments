package myanmarpayments_test

import (
	"errors"
	"net/http"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/ayapay"
	"github.com/laranex/go-myanmar-payments/v4/cybersource"
	"github.com/laranex/go-myanmar-payments/v4/internal/testutil"
	"github.com/laranex/go-myanmar-payments/v4/kbzpay"
	"github.com/laranex/go-myanmar-payments/v4/wavemoney"
	"github.com/laranex/go-myanmar-payments/v4/yomammqr"
)

// The vectors in testdata/parity/vectors.json are shared byte for byte with php-, node- and
// python-myanmar-payments: every SDK must give the same answer for every case.

type callbackHandler interface {
	HandleCallback(*myanmarpayments.CallbackRequest) (*myanmarpayments.PaymentCallback, error)
}

func parityGateways(t *testing.T, secrets map[string]any) map[string]callbackHandler {
	t.Helper()
	secret := func(name string) string { return secrets[name].(string) }
	must := func(gateway callbackHandler, err error) callbackHandler {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return gateway
	}

	return map[string]callbackHandler{
		"kbz_pay": must(kbzpay.New(kbzpay.Config{AppID: "kp123", AppKey: secret("kbz_pay_app_key"), MerchantCode: "100001"}, nil)),
		"wave_money": must(wavemoney.New(wavemoney.Config{
			MerchantID: "merchant", SecretKey: secret("wave_money_secret_key"), MerchantName: "Shop",
		}, nil)),
		"aya_pay": must(ayapay.New(ayapay.Config{AppKey: "app-key", AppSecret: secret("aya_pay_app_secret")}, nil)),
		"yoma_mmqr": must(yomammqr.New(yomammqr.Config{
			MerchantID: "merchant", ClientID: "client", ClientSecret: "secret", WebhookHashKey: secret("yoma_mmqr_webhook_hashkey"),
		}, nil, nil)),
		"cyber_source": must(cybersource.New(cybersource.Config{
			ProfileID: "profile", AccessKey: "access", SecretKey: secret("cyber_source_secret_key"),
		})),
	}
}

func TestParityCallbacks(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")
	gateways := parityGateways(t, vectors["secrets"].(map[string]any))

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

func TestParitySandbox(t *testing.T) {
	vectors := testutil.Fixture(t, "parity/vectors.json")

	for _, item := range vectors["sandbox"].([]any) {
		vector := item.(map[string]any)
		value, sandbox := vector["value"].(string), vector["sandbox"].(bool)
		env := func(prefix string) func(string) string {
			return func(key string) string {
				if key == prefix+"_SANDBOX" {
					return value
				}
				return ""
			}
		}
		production := []bool{
			kbzpay.ConfigFromEnv(env("KBZ_PAY")).Production,
			wavemoney.ConfigFromEnv(env("WAVE_MONEY")).Production,
			ayapay.ConfigFromEnv(env("AYA_PAY")).Production,
			yomammqr.ConfigFromEnv(env("YOMA_MMQR")).Production,
			cybersource.ConfigFromEnv(env("CYBER_SOURCE")).Production,
		}
		for i, got := range production {
			if got == sandbox {
				t.Errorf("gateway %d: *_SANDBOX=%q gives production=%v, want sandbox=%v", i, value, got, sandbox)
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
		WebhookHashKey: "hash", BaseURL: vector["base_url"].(string),
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

	amountError := func(err error) string {
		var invalid *myanmarpayments.InvalidPaymentDataError
		if !errors.As(err, &invalid) {
			t.Fatalf("expected InvalidPaymentDataError, got %v", err)
		}
		return invalid.Errors["amount"]
	}
	decimal := myanmarpayments.MustParseAmount("1000.505")
	wave := wavemoney.PaymentData{
		OrderID: "ORDER_1", CallbackURL: "https://shop.test/cb", ReturnURL: "https://shop.test/done", Description: "Order",
		Items: []wavemoney.Item{{Name: "Shoes", Amount: myanmarpayments.Kyat(1000)}}, Amount: myanmarpayments.MustParseAmount("1000.5"),
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
