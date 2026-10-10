package ayapay

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

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

func TestConfigFromEnvFallsBackToThePGWNames(t *testing.T) {
	env := map[string]string{"AYA_PGW_APP_KEY": "key", "AYA_PGW_APP_SECRET": "secret", "AYA_PGW_BASE_URL": "https://pgw.test"}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	if config.AppKey != "key" || config.AppSecret != "secret" || config.TimeoutSeconds != 0 || config.BaseURL != "https://pgw.test" {
		t.Fatalf("unexpected config %+v", config)
	}
	if (Config{}).ResolvedBaseURL() != ProductionURL {
		t.Fatal("unexpected default URLs")
	}
}

func TestNewReportsTheMissingCredential(t *testing.T) {
	var configErr *myanmarpayments.ConfigurationError
	if _, err := New(Config{AppKey: "key"}, nil); !errors.As(err, &configErr) || configErr.Gateway != "aya_pay" || configErr.Key != "app_secret" {
		t.Fatalf("expected missing app_secret, got %v", err)
	}
	if _, err := New(Config{AppKey: "key", AppSecret: "s"}, nil); !errors.As(err, &configErr) || configErr.Key != "timeout_in_seconds" {
		t.Fatalf("expected missing timeout_in_seconds, got %v", err)
	}
}

func TestServiceSupports(t *testing.T) {
	service := Service{Methods: []Method{MethodQR}}
	if !service.Supports(MethodQR) || service.Supports(MethodWeb) {
		t.Fatal("unexpected Supports")
	}
}
