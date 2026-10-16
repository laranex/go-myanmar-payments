package wavepay

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

// testConfig returns a complete, valid configuration with the production URLs; tests change only
// what they check.
func testConfig() Config {
	return Config{
		MerchantID: "testmerchantID", SecretKey: "test-secret", MerchantName: "Shop", BaseURL: ProductionURL, BaseRedirectURL: ProductionURL,
		TimeToLiveSeconds: 300, TimeoutSeconds: 30,
	}
}

func TestConfigFromEnvReadsTheWaveVariables(t *testing.T) {
	env := map[string]string{
		"WAVE_PAY_MERCHANT_ID": "merchant", "WAVE_PAY_SECRET_KEY": "secret", "WAVE_PAY_MERCHANT_NAME": "My Shop", "APP_NAME": "Ignored",
		"WAVE_PAY_TIME_TO_LIVE_IN_SECONDS": "600", "MYANMAR_PAYMENTS_HTTP_TIMEOUT": "20",
		"WAVE_PAY_BASE_URL": "https://wave.test:8107/", "WAVE_PAY_BASE_REDIRECT_URL": "https://wave.test/",
	}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	if config.MerchantID != "merchant" || config.SecretKey != "secret" || config.MerchantName != "My Shop" || config.TimeToLiveSeconds != 600 || config.TimeoutSeconds != 20 {
		t.Fatalf("unexpected config %+v", config)
	}
	if config.ResolvedBaseURL() != "https://wave.test:8107" || config.ResolvedBaseRedirectURL() != "https://wave.test" {
		t.Fatal("unexpected resolved values")
	}

	gateway, err := New(config, nil)
	if err != nil || gateway.Config() != config {
		t.Fatalf("unexpected gateway %v", err)
	}
}

func TestConfigDefaults(t *testing.T) {
	config := ConfigFromEnv(func(key string) string { return map[string]string{"APP_NAME": "Shop"}[key] })
	if config != (Config{TimeoutSeconds: myanmarpayments.DefaultTimeoutSeconds}) {
		t.Fatalf("unexpected defaults %+v", config)
	}
}

func TestNewRequiresEverySettingExceptTheTimeout(t *testing.T) {
	for key, clear := range map[string]func(*Config){
		"merchant_id":             func(c *Config) { c.MerchantID = "" },
		"secret_key":              func(c *Config) { c.SecretKey = " " },
		"merchant_name":           func(c *Config) { c.MerchantName = "" },
		"base_url":                func(c *Config) { c.BaseURL = "   " },
		"base_redirect_url":       func(c *Config) { c.BaseRedirectURL = "" },
		"time_to_live_in_seconds": func(c *Config) { c.TimeToLiveSeconds = 0 },
	} {
		config := testConfig()
		clear(&config)
		var configErr *myanmarpayments.ConfigurationError
		if _, err := New(config, nil); !errors.As(err, &configErr) || configErr.Gateway != "wave_pay" || configErr.Key != key || configErr.Invalid ||
			err.Error() != "myanmarpayments: The wave_pay configuration is missing ["+key+"]." {
			t.Fatalf("expected missing %s, got %v", key, err)
		}
	}
}

func TestNewReportsAnInvalidTimeSetting(t *testing.T) {
	var configErr *myanmarpayments.ConfigurationError
	base := testConfig()
	for _, tc := range []struct {
		ttl, timeout int
		key          string
		invalid      bool
	}{
		{-1, 30, "time_to_live_in_seconds", true},
		{300, -1, "timeout_in_seconds", true},
	} {
		config := base
		config.TimeToLiveSeconds, config.TimeoutSeconds = tc.ttl, tc.timeout
		if _, err := New(config, nil); !errors.As(err, &configErr) || configErr.Key != tc.key || configErr.Invalid != tc.invalid {
			t.Fatalf("%+v: got %v", tc, err)
		}
	}

	config := base
	config.TimeoutSeconds = 0
	gateway, err := New(config, nil)
	if err != nil || gateway.Config().TimeoutSeconds != myanmarpayments.DefaultTimeoutSeconds {
		t.Fatalf("expected the default timeout, got %v", err)
	}
}
