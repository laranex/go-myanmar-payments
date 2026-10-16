package ayapay

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

// testConfig returns a complete, valid configuration with the production URL; tests change only
// what they check.
func testConfig() Config {
	return Config{AppKey: "app-key", AppSecret: "test-secret", BaseURL: ProductionURL, TimeoutSeconds: 30}
}

func TestConfigFromEnvReadsTheAYAVariables(t *testing.T) {
	env := map[string]string{"AYA_PAY_APP_KEY": "key", "AYA_PAY_APP_SECRET": "secret", "MYANMAR_PAYMENTS_HTTP_TIMEOUT": "30", "AYA_PAY_BASE_URL": "https://aya.test/"}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	if config.AppKey != "key" || config.AppSecret != "secret" || config.TimeoutSeconds != 30 || config.ResolvedBaseURL() != "https://aya.test" {
		t.Fatalf("unexpected config %+v", config)
	}

	gateway, err := New(config, nil)
	if err != nil || gateway.Config() != config {
		t.Fatalf("unexpected gateway %v", err)
	}
}

func TestConfigFromEnvReadsOnlyTheAYAPayNames(t *testing.T) {
	env := map[string]string{"AYA_PAY_APP_KEY": "key", "AYA_PAY_APP_SECRET": "secret", "AYA_PAY_BASE_URL": "https://aya.test"}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	if config.AppKey != "key" || config.AppSecret != "secret" || config.TimeoutSeconds != myanmarpayments.DefaultTimeoutSeconds || config.BaseURL != "https://aya.test" {
		t.Fatalf("unexpected config %+v", config)
	}
	legacy := map[string]string{"AYA_PGW_APP_KEY": "key", "AYA_PGW_APP_SECRET": "secret", "AYA_PGW_BASE_URL": "https://aya.test"}
	if config := ConfigFromEnv(func(key string) string { return legacy[key] }); config.AppKey != "" || config.AppSecret != "" || config.BaseURL != "" {
		t.Fatalf("expected the AYA_PGW_* names to be ignored, got %+v", config)
	}
	if (Config{}).ResolvedBaseURL() != "" {
		t.Fatal("expected no production fallback")
	}
}

func TestNewRequiresEverySettingExceptTheTimeout(t *testing.T) {
	for key, clear := range map[string]func(*Config){
		"app_key":    func(c *Config) { c.AppKey = "" },
		"app_secret": func(c *Config) { c.AppSecret = " " },
		"base_url":   func(c *Config) { c.BaseURL = "   " },
	} {
		config := testConfig()
		clear(&config)
		var configErr *myanmarpayments.ConfigurationError
		if _, err := New(config, nil); !errors.As(err, &configErr) || configErr.Gateway != "aya_pay" || configErr.Key != key || configErr.Invalid || err.Error() != "myanmarpayments: The aya_pay configuration is missing ["+key+"]." {
			t.Fatalf("expected missing %s, got %v", key, err)
		}
	}

	invalid := testConfig()
	invalid.TimeoutSeconds = -1
	var configErr *myanmarpayments.ConfigurationError
	if _, err := New(invalid, nil); !errors.As(err, &configErr) || configErr.Key != "timeout_in_seconds" || !configErr.Invalid {
		t.Fatalf("expected invalid timeout_in_seconds, got %v", err)
	}
}

func TestNewDefaultsTheTimeout(t *testing.T) {
	config := testConfig()
	config.TimeoutSeconds = 0
	gateway, err := New(config, nil)
	if err != nil || gateway.Config().TimeoutSeconds != myanmarpayments.DefaultTimeoutSeconds {
		t.Fatalf("expected the default timeout, got %v", err)
	}
}

func TestServiceSupports(t *testing.T) {
	service := Service{Methods: []Method{MethodQR}}
	if !service.Supports(MethodQR) || service.Supports(MethodWeb) {
		t.Fatal("unexpected Supports")
	}
}
