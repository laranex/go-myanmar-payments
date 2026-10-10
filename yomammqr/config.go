package yomammqr

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// ProductionURL is Yoma Bank's production payment hub. To use UAT or a proxy, set BaseURL.
const ProductionURL = "https://paymenthubapi.yomabank.com"

// Config holds Yoma MMQR credentials. Every field except WebhookSecret and BaseURL is required.
type Config struct {
	// MerchantID is the merchant id Yoma issued.
	MerchantID string
	// ClientID is the OAuth client id.
	ClientID string
	// ClientSecret is the OAuth client secret.
	ClientSecret string
	// WebhookHashKey is the hash key Yoma issued for verifying callbacks.
	WebhookHashKey string
	// APIVersion is the {version} segment of the API paths, e.g. v1rc.
	APIVersion string
	// TimeoutSeconds is the timeout of the default HTTP client, a whole number greater than 0.
	// A client you pass to New keeps its own timeout.
	TimeoutSeconds int
	// WebhookSecret is the secret you shared with Yoma; when set, callbacks must carry it in X-Webhook-Secret.
	WebhookSecret string
	// BaseURL overrides the API base URL.
	BaseURL string
}

// ConfigFromEnv reads YOMA_MMQR_MERCHANT_ID, YOMA_MMQR_CLIENT_ID, YOMA_MMQR_CLIENT_SECRET,
// YOMA_MMQR_WEBHOOK_HASHKEY, YOMA_MMQR_API_VERSION, MYANMAR_PAYMENTS_HTTP_TIMEOUT,
// YOMA_MMQR_WEBHOOK_SECRET and YOMA_MMQR_BASE_URL.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		MerchantID:     env.First(getenv, "YOMA_MMQR_MERCHANT_ID"),
		ClientID:       env.First(getenv, "YOMA_MMQR_CLIENT_ID"),
		ClientSecret:   env.First(getenv, "YOMA_MMQR_CLIENT_SECRET"),
		WebhookHashKey: env.First(getenv, "YOMA_MMQR_WEBHOOK_HASHKEY"),
		APIVersion:     env.First(getenv, "YOMA_MMQR_API_VERSION"),
		TimeoutSeconds: env.Seconds(getenv, env.TimeoutKey),
		WebhookSecret:  env.First(getenv, "YOMA_MMQR_WEBHOOK_SECRET"),
		BaseURL:        env.First(getenv, "YOMA_MMQR_BASE_URL"),
	}
}

func (c Config) validate() error {
	if err := env.Require("yoma_mmqr", "merchant_id", c.MerchantID, "client_id", c.ClientID, "client_secret", c.ClientSecret, "webhook_hashkey", c.WebhookHashKey, "api_version", c.APIVersion); err != nil {
		return err
	}
	return env.RequireSeconds("yoma_mmqr", "timeout_in_seconds", c.TimeoutSeconds)
}

// ResolvedBaseURL returns the base URL in use: the override, or the production URL.
func (c Config) ResolvedBaseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return ProductionURL
}
