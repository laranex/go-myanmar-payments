package ayapay

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// Endpoints from the APG Integration Guide.
const (
	SandboxURL    = "https://uat-pgw.ayainnovation.com"
	ProductionURL = "https://pgw.ayainnovation.com"
)

// Config holds AYA Payment Gateway credentials. The zero value of Production selects UAT.
type Config struct {
	// AppKey is the public application key, sent with every request.
	AppKey string
	// AppSecret signs requests and verifies callbacks.
	AppSecret string
	// Production selects the production endpoints.
	Production bool
	// BaseURL overrides the gateway base URL.
	BaseURL string
}

// ConfigFromEnv reads AYA_PAY_APP_KEY, AYA_PAY_APP_SECRET, AYA_PAY_SANDBOX and AYA_PAY_BASE_URL,
// falling back to the AYA_PGW_* names.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		AppKey:     env.First(getenv, "AYA_PAY_APP_KEY", "AYA_PGW_APP_KEY"),
		AppSecret:  env.First(getenv, "AYA_PAY_APP_SECRET", "AYA_PGW_APP_SECRET"),
		Production: env.Production(getenv, "AYA_PAY_SANDBOX"),
		BaseURL:    env.First(getenv, "AYA_PAY_BASE_URL", "AYA_PGW_BASE_URL"),
	}
}

func (c Config) validate() error {
	return env.Require("aya_pay", "app_key", c.AppKey, "app_secret", c.AppSecret)
}

// ResolvedBaseURL returns the base URL in use.
func (c Config) ResolvedBaseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	if c.Production {
		return ProductionURL
	}
	return SandboxURL
}
