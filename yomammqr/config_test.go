package yomammqr

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

func TestConfigFromEnvReadsTheYomaVariables(t *testing.T) {
	env := map[string]string{
		"YOMA_MMQR_MERCHANT_ID": "M001", "YOMA_MMQR_CLIENT_ID": "client", "YOMA_MMQR_CLIENT_SECRET": "secret",
		"YOMA_MMQR_WEBHOOK_HASHKEY": "hash", "YOMA_MMQR_WEBHOOK_SECRET": "shared", "YOMA_MMQR_SANDBOX": "no",
		"YOMA_MMQR_BASE_URL": "https://yoma.test/", "YOMA_MMQR_API_VERSION": "v2",
	}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	want := Config{MerchantID: "M001", ClientID: "client", ClientSecret: "secret", WebhookHashKey: "hash", WebhookSecret: "shared", Production: true, BaseURL: "https://yoma.test/", APIVersion: "v2"}
	if config != want || config.ResolvedBaseURL() != "https://yoma.test" || config.ResolvedAPIVersion() != "v2" {
		t.Fatalf("unexpected config %+v", config)
	}

	gateway, err := New(config, nil, nil)
	if err != nil || gateway.Config() != config {
		t.Fatalf("unexpected gateway %v", err)
	}
}

func TestConfigDefaults(t *testing.T) {
	if (Config{}).ResolvedBaseURL() != SandboxURL || (Config{Production: true}).ResolvedBaseURL() != ProductionURL || (Config{}).ResolvedAPIVersion() != DefaultAPIVersion {
		t.Fatal("unexpected defaults")
	}
}

func TestNewReportsTheMissingCredential(t *testing.T) {
	var configErr *myanmarpayments.ConfigurationError
	if _, err := New(Config{MerchantID: "M", ClientID: "c", ClientSecret: "s"}, nil, nil); !errors.As(err, &configErr) || configErr.Gateway != "yoma_mmqr" || configErr.Key != "webhook_hashkey" {
		t.Fatalf("expected missing webhook_hashkey, got %v", err)
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
