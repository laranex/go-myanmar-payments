package wavepayminiapp

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

// testConfig returns a complete, valid configuration with the production URL; tests change only
// what they check.
func testConfig() Config {
	return Config{ClientID: "client", ClientSecret: "client-secret", MiniAppID: "MINI456", SecretKey: "test-secret", BaseURL: ProductionURL, TimeoutSeconds: 30}
}

func TestConfigFromEnvReadsTheMiniAppVariables(t *testing.T) {
	env := map[string]string{
		"WAVE_PAY_MINI_APP_CLIENT_ID": "c", "WAVE_PAY_MINI_APP_CLIENT_SECRET": "s", "WAVE_PAY_MINI_APP_MINI_APP_ID": "m",
		"WAVE_PAY_MINI_APP_SECRET_KEY": "k", "WAVE_PAY_MINI_APP_BASE_URL": " https://preprodnewapi.wavemoney.io/ ", "MYANMAR_PAYMENTS_HTTP_TIMEOUT": "20",
	}
	config := ConfigFromEnv(func(key string) string { return env[key] })
	want := Config{ClientID: "c", ClientSecret: "s", MiniAppID: "m", SecretKey: "k", BaseURL: "https://preprodnewapi.wavemoney.io/", TimeoutSeconds: 20}
	if config != want || config.ResolvedBaseURL() != "https://preprodnewapi.wavemoney.io" {
		t.Fatalf("unexpected config %+v", config)
	}

	gateway, err := New(config, nil, nil)
	if err != nil || gateway.Config() != config {
		t.Fatalf("unexpected gateway %v", err)
	}
	if ProductionURL != "https://api.wavemoney.io:8100" {
		t.Fatalf("unexpected production URL %s", ProductionURL)
	}
}

func TestNewDefaultsTheTimeout(t *testing.T) {
	config := testConfig()
	config.TimeoutSeconds = 0
	gateway, err := New(config, nil, nil)
	if err != nil || gateway.Config().TimeoutSeconds != myanmarpayments.DefaultTimeoutSeconds {
		t.Fatalf("unexpected timeout %v", err)
	}
}

func TestNewRequiresEverySettingExceptTheTimeout(t *testing.T) {
	for key, clear := range map[string]func(*Config){
		"client_id":     func(c *Config) { c.ClientID = "" },
		"client_secret": func(c *Config) { c.ClientSecret = " " },
		"mini_app_id":   func(c *Config) { c.MiniAppID = "" },
		"secret_key":    func(c *Config) { c.SecretKey = "\t" },
		"base_url":      func(c *Config) { c.BaseURL = "" },
	} {
		config := testConfig()
		clear(&config)
		var configErr *myanmarpayments.ConfigurationError
		if _, err := New(config, nil, nil); !errors.As(err, &configErr) || configErr.Key != key || configErr.Invalid || err.Error() != "myanmarpayments: The wave_pay_mini_app configuration is missing ["+key+"]." {
			t.Fatalf("expected missing %s, got %v", key, err)
		}
	}
}

func TestNewRejectsANegativeTimeout(t *testing.T) {
	config := testConfig()
	config.TimeoutSeconds = -1
	var configErr *myanmarpayments.ConfigurationError
	if _, err := New(config, nil, nil); !errors.As(err, &configErr) || !configErr.Invalid || configErr.Key != "timeout_in_seconds" {
		t.Fatalf("expected an invalid timeout, got %v", err)
	}
}
