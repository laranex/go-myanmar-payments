package kbzpay

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

func TestConfigFromEnvReadsTheKBZVariables(t *testing.T) {
	env := map[string]string{
		"KBZ_PAY_APP_ID": "app", "KBZ_PAY_APP_KEY": " key ", "KBZ_PAY_MERCHANT_CODE": "100001", "KBZ_PAY_SANDBOX": "false",
		"KBZ_PAY_BASE_URL": "https://kbz.test/api/", "KBZ_PAY_PWA_BASE_REDIRECT_URL": "https://kbz.test/pwa/#",
	}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	if config.AppID != "app" || config.AppKey != "key" || config.MerchantCode != "100001" || !config.Production {
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

func TestConfigDefaultsToUATAndSwitchesToProduction(t *testing.T) {
	sandbox, production := Config{}, Config{Production: true}
	if sandbox.ResolvedAPIURL() != SandboxAPIURL || sandbox.ResolvedPWAURL() != SandboxPWAURL {
		t.Fatal("unexpected sandbox URLs")
	}
	if production.ResolvedAPIURL() != ProductionAPIURL || production.ResolvedPWAURL() != ProductionPWAURL {
		t.Fatal("unexpected production URLs")
	}
}

func TestNewReportsTheMissingCredential(t *testing.T) {
	var configErr *myanmarpayments.ConfigurationError
	if _, err := New(Config{AppID: "app", AppKey: "key"}, nil); !errors.As(err, &configErr) || configErr.Gateway != "kbz_pay" || configErr.Key != "merchant_code" {
		t.Fatalf("expected missing merchant_code, got %v", err)
	}
}
