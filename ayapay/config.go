package ayapay

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// ProductionURL is the AYA Payment Gateway's production host, production's value for BaseURL; it
// is not a default.
const ProductionURL = "https://pgw.ayainnovation.com"

// Config holds AYA Payment Gateway credentials and endpoints. Every field is required except
// TimeoutSeconds.
type Config struct {
	// AppKey is the public application key, sent with every request.
	AppKey string
	// AppSecret signs requests and verifies callbacks.
	AppSecret string
	// TimeoutSeconds is the timeout of the default HTTP client, a whole number greater than 0
	// (optional; 0 means myanmarpayments.DefaultTimeoutSeconds, 30). A client you pass to New
	// keeps its own timeout.
	TimeoutSeconds int
	// BaseURL is the gateway base URL, e.g. ProductionURL or https://uat-pgw.ayainnovation.com.
	BaseURL string
}

// ConfigFromEnv reads AYA_PAY_APP_KEY, AYA_PAY_APP_SECRET, AYA_PAY_BASE_URL and
// MYANMAR_PAYMENTS_HTTP_TIMEOUT, falling back to the AYA_PGW_* names.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		AppKey:         env.First(getenv, "AYA_PAY_APP_KEY", "AYA_PGW_APP_KEY"),
		AppSecret:      env.First(getenv, "AYA_PAY_APP_SECRET", "AYA_PGW_APP_SECRET"),
		TimeoutSeconds: env.Timeout(getenv),
		BaseURL:        env.First(getenv, "AYA_PAY_BASE_URL", "AYA_PGW_BASE_URL"),
	}
}

func (c Config) validate() error {
	if err := env.Require("aya_pay", "app_key", c.AppKey, "app_secret", c.AppSecret, "base_url", c.BaseURL); err != nil {
		return err
	}
	return env.OptionalSeconds("aya_pay", "timeout_in_seconds", c.TimeoutSeconds)
}

// ResolvedBaseURL returns BaseURL without its trailing slash.
func (c Config) ResolvedBaseURL() string {
	return strings.TrimRight(c.BaseURL, "/")
}
