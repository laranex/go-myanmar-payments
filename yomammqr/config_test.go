package yomammqr

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

// testConfig returns a complete, valid configuration with the production URL; tests change only
// what they check.
func testConfig() Config {
	return Config{
		MerchantID: "M001", ClientID: "client", ClientSecret: "secret", WebhookHashKey: "hash-key", APIVersion: "v1rc",
		WebhookSecret: "webhook-secret", BaseURL: ProductionURL, TimeoutSeconds: 30,
	}
}

func TestConfigFromEnvReadsTheYomaVariables(t *testing.T) {
	env := map[string]string{
		"YOMA_MMQR_MERCHANT_ID": "M001", "YOMA_MMQR_CLIENT_ID": "client", "YOMA_MMQR_CLIENT_SECRET": "secret",
		"YOMA_MMQR_WEBHOOK_HASHKEY": "hash", "YOMA_MMQR_WEBHOOK_SECRET": "shared", "MYANMAR_PAYMENTS_HTTP_TIMEOUT": "30",
		"YOMA_MMQR_BASE_URL": "https://yoma.test/", "YOMA_MMQR_API_VERSION": "v2",
	}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	want := Config{MerchantID: "M001", ClientID: "client", ClientSecret: "secret", WebhookHashKey: "hash", WebhookSecret: "shared", TimeoutSeconds: 30, BaseURL: "https://yoma.test/", APIVersion: "v2"}
	if config != want || config.ResolvedBaseURL() != "https://yoma.test" {
		t.Fatalf("unexpected config %+v", config)
	}

	gateway, err := New(config, nil, nil)
	if err != nil || gateway.Config() != config {
		t.Fatalf("unexpected gateway %v", err)
	}
}

func TestResolvedBaseURLHasNoProductionFallback(t *testing.T) {
	if (Config{}).ResolvedBaseURL() != "" || (Config{BaseURL: ProductionURL + "/"}).ResolvedBaseURL() != ProductionURL {
		t.Fatal("unexpected base URL")
	}
}

func TestNewRequiresEverySettingExceptTheTimeout(t *testing.T) {
	for key, clear := range map[string]func(*Config){
		"merchant_id":     func(c *Config) { c.MerchantID = "" },
		"client_id":       func(c *Config) { c.ClientID = " " },
		"client_secret":   func(c *Config) { c.ClientSecret = "" },
		"webhook_hashkey": func(c *Config) { c.WebhookHashKey = "" },
		"api_version":     func(c *Config) { c.APIVersion = "" },
		"webhook_secret":  func(c *Config) { c.WebhookSecret = "   " },
		"base_url":        func(c *Config) { c.BaseURL = "" },
	} {
		config := testConfig()
		clear(&config)
		var configErr *myanmarpayments.ConfigurationError
		if _, err := New(config, nil, nil); !errors.As(err, &configErr) || configErr.Key != key || configErr.Invalid || err.Error() != "myanmarpayments: The yoma_mmqr configuration is missing ["+key+"]." {
			t.Fatalf("expected missing %s, got %v", key, err)
		}
	}
}

func TestNewReportsTheMissingCredential(t *testing.T) {
	var configErr *myanmarpayments.ConfigurationError
	if _, err := New(Config{MerchantID: "M", ClientID: "c", ClientSecret: "s"}, nil, nil); !errors.As(err, &configErr) || configErr.Gateway != "yoma_mmqr" || configErr.Key != "webhook_hashkey" {
		t.Fatalf("expected missing webhook_hashkey, got %v", err)
	}
	if _, err := New(Config{MerchantID: "M", ClientID: "c", ClientSecret: "s", WebhookHashKey: "h"}, nil, nil); !errors.As(err, &configErr) || configErr.Key != "api_version" {
		t.Fatalf("expected missing api_version, got %v", err)
	}
	invalid := testConfig()
	invalid.TimeoutSeconds = -1
	if _, err := New(invalid, nil, nil); !errors.As(err, &configErr) || configErr.Key != "timeout_in_seconds" || !configErr.Invalid {
		t.Fatalf("expected invalid timeout_in_seconds, got %v", err)
	}
}

func TestNewDefaultsTheTimeout(t *testing.T) {
	config := testConfig()
	config.TimeoutSeconds = 0
	gateway, err := New(config, nil, nil)
	if err != nil || gateway.Config().TimeoutSeconds != myanmarpayments.DefaultTimeoutSeconds {
		t.Fatalf("expected the default timeout, got %v", err)
	}
}

func TestForgetTokenDropsTheCachedToken(t *testing.T) {
	cache := myanmarpayments.NewMemoryTokenCache()
	gateway := newGateway(t, nil, cache)
	cache.Set(gateway.tokenCacheKey(), "cached", 0)

	gateway.ForgetToken()
	if _, ok := cache.Get(gateway.tokenCacheKey()); ok {
		t.Fatal("expected the token to be forgotten")
	}
}
