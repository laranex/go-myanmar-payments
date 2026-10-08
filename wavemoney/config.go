package wavemoney

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// Endpoints from the WPPG documentation. Wave serves the authenticate page without the API port.
// The documented test host no longer resolves in DNS (October 2026); set BaseURL if Wave gives you another.
const (
	SandboxURL                = "https://testpayments.wavemoney.io:8107"
	ProductionURL             = "https://payments.wavemoney.io"
	SandboxAuthenticateURL    = "https://testpayments.wavemoney.io"
	ProductionAuthenticateURL = "https://payments.wavemoney.io"
	defaultTimeToLiveSeconds  = 300
)

// Config holds Wave Money credentials. The zero value of Production selects the test endpoints.
type Config struct {
	// MerchantID is the merchant id Wave issued.
	MerchantID string
	// SecretKey is the hash secret key Wave issued.
	SecretKey string
	// MerchantName is your business name, shown on Wave's payment page.
	MerchantName string
	// TimeToLiveSeconds is how long the customer has to pay; 0 means 300.
	TimeToLiveSeconds int
	// Production selects the production endpoints.
	Production bool
	// BaseURL overrides the API base URL.
	BaseURL string
	// AuthenticateURL overrides the host the customer is redirected to.
	AuthenticateURL string
}

// ConfigFromEnv reads WAVE_MONEY_MERCHANT_ID, WAVE_MONEY_SECRET_KEY, WAVE_MONEY_MERCHANT_NAME,
// WAVE_MONEY_TIME_TO_LIVE_IN_SECONDS, WAVE_MONEY_SANDBOX, WAVE_MONEY_BASE_URL and WAVE_MONEY_AUTHENTICATE_URL.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		MerchantID:        env.First(getenv, "WAVE_MONEY_MERCHANT_ID"),
		SecretKey:         env.First(getenv, "WAVE_MONEY_SECRET_KEY"),
		MerchantName:      env.First(getenv, "WAVE_MONEY_MERCHANT_NAME", "APP_NAME"),
		TimeToLiveSeconds: env.Int(getenv, "WAVE_MONEY_TIME_TO_LIVE_IN_SECONDS", defaultTimeToLiveSeconds),
		Production:        env.Production(getenv, "WAVE_MONEY_SANDBOX"),
		BaseURL:           env.First(getenv, "WAVE_MONEY_BASE_URL"),
		AuthenticateURL:   env.First(getenv, "WAVE_MONEY_AUTHENTICATE_URL"),
	}
}

func (c Config) validate() error {
	return env.Require("wave_money", "merchant_id", c.MerchantID, "secret_key", c.SecretKey, "merchant_name", c.MerchantName)
}

// ResolvedBaseURL returns the API base URL in use.
func (c Config) ResolvedBaseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	if c.Production {
		return ProductionURL
	}
	return SandboxURL
}

// ResolvedAuthenticateURL returns the authenticate host in use.
func (c Config) ResolvedAuthenticateURL() string {
	if c.AuthenticateURL != "" {
		return strings.TrimRight(c.AuthenticateURL, "/")
	}
	if c.Production {
		return ProductionAuthenticateURL
	}
	return SandboxAuthenticateURL
}

// ResolvedTimeToLive returns the time to live in seconds.
func (c Config) ResolvedTimeToLive() int {
	if c.TimeToLiveSeconds <= 0 {
		return defaultTimeToLiveSeconds
	}
	return c.TimeToLiveSeconds
}
