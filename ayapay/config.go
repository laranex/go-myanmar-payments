package ayapay

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// ProductionURL is the AYA Payment Gateway's production host. To use UAT or a proxy, set BaseURL.
const ProductionURL = "https://pgw.ayainnovation.com"

// Config holds AYA Payment Gateway credentials. Every field except BaseURL is required.
type Config struct {
	// AppKey is the public application key, sent with every request.
	AppKey string
	// AppSecret signs requests and verifies callbacks.
	AppSecret string
	// TimeoutSeconds is the timeout of the default HTTP client, a whole number greater than 0.
	// A client you pass to New keeps its own timeout.
	TimeoutSeconds int
	// BaseURL overrides the gateway base URL.
	BaseURL string
}

// ConfigFromEnv reads AYA_PAY_APP_KEY, AYA_PAY_APP_SECRET, MYANMAR_PAYMENTS_HTTP_TIMEOUT and
// AYA_PAY_BASE_URL, falling back to the AYA_PGW_* names.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		AppKey:         env.First(getenv, "AYA_PAY_APP_KEY", "AYA_PGW_APP_KEY"),
		AppSecret:      env.First(getenv, "AYA_PAY_APP_SECRET", "AYA_PGW_APP_SECRET"),
		TimeoutSeconds: env.Seconds(getenv, env.TimeoutKey),
		BaseURL:        env.First(getenv, "AYA_PAY_BASE_URL", "AYA_PGW_BASE_URL"),
	}
}

func (c Config) validate() error {
	if err := env.Require("aya_pay", "app_key", c.AppKey, "app_secret", c.AppSecret); err != nil {
		return err
	}
	return env.RequireSeconds("aya_pay", "timeout_in_seconds", c.TimeoutSeconds)
}

// ResolvedBaseURL returns the base URL in use: the override, or the production URL.
func (c Config) ResolvedBaseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return ProductionURL
}
