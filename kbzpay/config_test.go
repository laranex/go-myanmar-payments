package kbzpay

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

// testConfig returns a complete, valid configuration with the production URLs; tests change only
// what they check.
func testConfig() Config {
	return Config{
		AppID: "kp123", AppKey: "secret-key", MerchantCode: "100001", APIURL: ProductionAPIURL, PWAURL: ProductionPWAURL,
		TimeToLiveMinutes: 15, TimeoutSeconds: 30,
	}
}

func TestConfigFromEnvReadsTheKBZVariables(t *testing.T) {
	env := map[string]string{
		"KBZ_PAY_APP_ID": "app", "KBZ_PAY_APP_KEY": " key ", "KBZ_PAY_MERCHANT_CODE": "100001", "MYANMAR_PAYMENTS_HTTP_TIMEOUT": " 15 ",
		"KBZ_PAY_BASE_URL": "https://kbz.test/api/", "KBZ_PAY_PWA_BASE_REDIRECT_URL": "https://kbz.test/pwa/#", "KBZ_PAY_TIME_TO_LIVE_IN_MINUTES": "20",
	}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	if config.AppID != "app" || config.AppKey != "key" || config.MerchantCode != "100001" || config.TimeoutSeconds != 15 || config.TimeToLiveMinutes != 20 {
		t.Fatalf("unexpected config %+v", config)
	}
	if config.ResolvedAPIURL() != "https://kbz.test/api" || config.ResolvedPWAURL() != "https://kbz.test/pwa/#/" {
		t.Fatalf("unexpected URLs %s %s", config.ResolvedAPIURL(), config.ResolvedPWAURL())
	}

	gateway, err := New(config, nil)
	if err != nil || gateway.Config() != config {
		t.Fatalf("unexpected gateway %v", err)
	}
}

func TestResolvedURLsHaveNoProductionFallback(t *testing.T) {
	if (Config{}).ResolvedAPIURL() != "" {
		t.Fatal("expected no production fallback")
	}
	config := testConfig()
	if config.ResolvedAPIURL() != ProductionAPIURL || config.ResolvedPWAURL() != ProductionPWAURL {
		t.Fatal("unexpected production URLs")
	}
}

func TestNewRequiresEverySettingExceptTheTimeout(t *testing.T) {
	for key, clear := range map[string]func(*Config){
		"app_id":                  func(c *Config) { c.AppID = "" },
		"app_key":                 func(c *Config) { c.AppKey = " " },
		"merchant_code":           func(c *Config) { c.MerchantCode = "" },
		"api_url":                 func(c *Config) { c.APIURL = "   " },
		"pwa_url":                 func(c *Config) { c.PWAURL = "" },
		"time_to_live_in_minutes": func(c *Config) { c.TimeToLiveMinutes = 0 },
	} {
		config := testConfig()
		clear(&config)
		var configErr *myanmarpayments.ConfigurationError
		if _, err := New(config, nil); !errors.As(err, &configErr) || configErr.Key != key || configErr.Invalid || err.Error() != "myanmarpayments: The kbz_pay configuration is missing ["+key+"]." {
			t.Fatalf("expected missing %s, got %v", key, err)
		}
	}

	for _, value := range []string{"", "  "} {
		env := map[string]string{"KBZ_PAY_APP_ID": "app", "KBZ_PAY_APP_KEY": "key", "KBZ_PAY_MERCHANT_CODE": "m", "KBZ_PAY_BASE_URL": ProductionAPIURL, "KBZ_PAY_PWA_BASE_REDIRECT_URL": ProductionPWAURL, "KBZ_PAY_TIME_TO_LIVE_IN_MINUTES": value}
		var configErr *myanmarpayments.ConfigurationError
		if _, err := New(ConfigFromEnv(func(key string) string { return env[key] }), nil); !errors.As(err, &configErr) || configErr.Key != "time_to_live_in_minutes" || configErr.Invalid {
			t.Fatalf("%q: expected missing time_to_live_in_minutes, got %v", value, err)
		}
	}
}

func TestTimeToLiveOutOfRangeIsInvalid(t *testing.T) {
	for _, minutes := range []int{-1, 121} {
		config := testConfig()
		config.TimeToLiveMinutes = minutes
		var configErr *myanmarpayments.ConfigurationError
		if _, err := New(config, nil); !errors.As(err, &configErr) || configErr.Key != "time_to_live_in_minutes" || !configErr.Invalid ||
			err.Error() != "myanmarpayments: The kbz_pay configuration [time_to_live_in_minutes] must be a whole number from 1 to 120." {
			t.Fatalf("%d: expected invalid time_to_live_in_minutes, got %v", minutes, err)
		}
	}
}

func TestTimeoutDefaultsTo30Seconds(t *testing.T) {
	for _, value := range []string{"", "   "} {
		config := ConfigFromEnv(func(key string) string {
			return map[string]string{"MYANMAR_PAYMENTS_HTTP_TIMEOUT": value}[key]
		})
		if config.TimeoutSeconds != myanmarpayments.DefaultTimeoutSeconds {
			t.Fatalf("%q: got %d, want the default", value, config.TimeoutSeconds)
		}
	}

	config := testConfig()
	config.TimeoutSeconds = 0
	gateway, err := New(config, nil)
	if err != nil || gateway.Config().TimeoutSeconds != 30 {
		t.Fatalf("expected the default timeout, got %v", err)
	}
}

func TestNewReportsTheMissingCredential(t *testing.T) {
	var configErr *myanmarpayments.ConfigurationError
	missing := testConfig()
	missing.MerchantCode = ""
	if _, err := New(missing, nil); !errors.As(err, &configErr) || configErr.Gateway != "kbz_pay" || configErr.Key != "merchant_code" {
		t.Fatalf("expected missing merchant_code, got %v", err)
	}
	invalid := testConfig()
	invalid.TimeoutSeconds = -1
	if _, err := New(invalid, nil); !errors.As(err, &configErr) || configErr.Key != "timeout_in_seconds" || !configErr.Invalid {
		t.Fatalf("expected invalid timeout_in_seconds, got %v", err)
	}
}
