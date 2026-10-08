package yomammqr

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// Endpoints of Yoma Bank's payment hub.
const (
	SandboxURL        = "https://devapi.yomabank.net"
	ProductionURL     = "https://paymenthubapi.yomabank.com"
	DefaultAPIVersion = "v1rc"
)

// Config holds Yoma MMQR credentials. The zero value of Production selects UAT.
type Config struct {
	// MerchantID is the merchant id Yoma issued.
	MerchantID string
	// ClientID is the OAuth client id.
	ClientID string
	// ClientSecret is the OAuth client secret.
	ClientSecret string
	// WebhookHashKey is the hash key Yoma issued for verifying callbacks.
	WebhookHashKey string
	// WebhookSecret is the secret you shared with Yoma; when set, callbacks must carry it in X-Webhook-Secret.
	WebhookSecret string
	// Production selects the production payment hub.
	Production bool
	// BaseURL overrides the API base URL.
	BaseURL string
	// APIVersion is the {version} segment of the API paths; empty means v1rc.
	APIVersion string
}

// ConfigFromEnv reads the YOMA_MMQR_* variables.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		MerchantID:     env.First(getenv, "YOMA_MMQR_MERCHANT_ID"),
		ClientID:       env.First(getenv, "YOMA_MMQR_CLIENT_ID"),
		ClientSecret:   env.First(getenv, "YOMA_MMQR_CLIENT_SECRET"),
		WebhookHashKey: env.First(getenv, "YOMA_MMQR_WEBHOOK_HASHKEY"),
		WebhookSecret:  env.First(getenv, "YOMA_MMQR_WEBHOOK_SECRET"),
		Production:     env.Production(getenv, "YOMA_MMQR_SANDBOX"),
		BaseURL:        env.First(getenv, "YOMA_MMQR_BASE_URL"),
		APIVersion:     env.First(getenv, "YOMA_MMQR_API_VERSION"),
	}
}

func (c Config) validate() error {
	return env.Require("yoma_mmqr", "merchant_id", c.MerchantID, "client_id", c.ClientID, "client_secret", c.ClientSecret, "webhook_hashkey", c.WebhookHashKey)
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

// ResolvedAPIVersion returns the API version in use.
func (c Config) ResolvedAPIVersion() string {
	if c.APIVersion == "" {
		return DefaultAPIVersion
	}
	return c.APIVersion
}
