package wavemoney

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

func TestConfigFromEnvReadsTheWaveVariables(t *testing.T) {
	env := map[string]string{
		"WAVE_MONEY_MERCHANT_ID": "merchant", "WAVE_MONEY_SECRET_KEY": "secret", "WAVE_MONEY_MERCHANT_NAME": "My Shop", "APP_NAME": "Ignored",
		"WAVE_MONEY_TIME_TO_LIVE_IN_SECONDS": "600", "MYANMAR_PAYMENTS_HTTP_TIMEOUT": "20",
		"WAVE_MONEY_BASE_URL": "https://wave.test/", "WAVE_MONEY_AUTHENTICATE_URL": "https://pay.wave.test/",
	}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	if config.MerchantID != "merchant" || config.SecretKey != "secret" || config.MerchantName != "My Shop" || config.TimeToLiveSeconds != 600 || config.TimeoutSeconds != 20 {
		t.Fatalf("unexpected config %+v", config)
	}
	if config.ResolvedBaseURL() != "https://wave.test" || config.ResolvedAuthenticateURL() != "https://pay.wave.test" {
		t.Fatal("unexpected resolved values")
	}

	gateway, err := New(config, nil)
	if err != nil || gateway.Config() != config {
		t.Fatalf("unexpected gateway %v", err)
	}
}

func TestConfigDefaults(t *testing.T) {
	config := ConfigFromEnv(func(key string) string { return map[string]string{"APP_NAME": "Shop"}[key] })
	if config != (Config{}) {
		t.Fatalf("unexpected defaults %+v", config)
	}
	production := Config{}
	if production.ResolvedBaseURL() != ProductionURL || production.ResolvedAuthenticateURL() != ProductionAuthenticateURL {
		t.Fatal("unexpected production URLs")
	}
}

func TestNewReportsTheMissingCredential(t *testing.T) {
	var configErr *myanmarpayments.ConfigurationError
	if _, err := New(Config{MerchantID: "m", SecretKey: "s"}, nil); !errors.As(err, &configErr) || configErr.Gateway != "wave_money" || configErr.Key != "merchant_name" {
		t.Fatalf("expected missing merchant_name, got %v", err)
	}
	base := Config{MerchantID: "m", SecretKey: "s", MerchantName: "Shop"}
	for _, tc := range []struct {
		ttl, timeout int
		key          string
		invalid      bool
	}{
		{0, 30, "time_to_live_in_seconds", false},
		{-1, 30, "time_to_live_in_seconds", true},
		{300, 0, "timeout_in_seconds", false},
		{300, -1, "timeout_in_seconds", true},
	} {
		config := base
		config.TimeToLiveSeconds, config.TimeoutSeconds = tc.ttl, tc.timeout
		if _, err := New(config, nil); !errors.As(err, &configErr) || configErr.Key != tc.key || configErr.Invalid != tc.invalid {
			t.Fatalf("%+v: got %v", tc, err)
		}
	}
}
