package wavemoney

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

func TestConfigFromEnvReadsTheWaveVariables(t *testing.T) {
	env := map[string]string{
		"WAVE_MONEY_MERCHANT_ID": "merchant", "WAVE_MONEY_SECRET_KEY": "secret", "APP_NAME": "My Shop",
		"WAVE_MONEY_TIME_TO_LIVE_IN_SECONDS": "600", "WAVE_MONEY_SANDBOX": "off",
		"WAVE_MONEY_BASE_URL": "https://wave.test/", "WAVE_MONEY_AUTHENTICATE_URL": "https://pay.wave.test/",
	}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	if config.MerchantID != "merchant" || config.SecretKey != "secret" || config.MerchantName != "My Shop" || config.TimeToLiveSeconds != 600 || !config.Production {
		t.Fatalf("unexpected config %+v", config)
	}
	if config.ResolvedBaseURL() != "https://wave.test" || config.ResolvedAuthenticateURL() != "https://pay.wave.test" || config.ResolvedTimeToLive() != 600 {
		t.Fatal("unexpected resolved values")
	}

	gateway, err := New(config, nil)
	if err != nil || gateway.Config() != config {
		t.Fatalf("unexpected gateway %v", err)
	}
}

func TestConfigDefaults(t *testing.T) {
	config := ConfigFromEnv(func(string) string { return "" })
	if config.Production || config.TimeToLiveSeconds != 300 || (Config{}).ResolvedTimeToLive() != 300 {
		t.Fatalf("unexpected defaults %+v", config)
	}
	production := Config{Production: true}
	if production.ResolvedBaseURL() != ProductionURL || production.ResolvedAuthenticateURL() != ProductionAuthenticateURL {
		t.Fatal("unexpected production URLs")
	}
}

func TestNewReportsTheMissingCredential(t *testing.T) {
	var configErr *myanmarpayments.ConfigurationError
	if _, err := New(Config{MerchantID: "m", SecretKey: "s"}, nil); !errors.As(err, &configErr) || configErr.Gateway != "wave_money" || configErr.Key != "merchant_name" {
		t.Fatalf("expected missing merchant_name, got %v", err)
	}
}
