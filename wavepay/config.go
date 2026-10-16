package wavepay

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// ProductionURL is production's API and payment page host from the Pay with Wave documentation, the
// production value of both BaseURL and BaseRedirectURL; it is not a default.
const ProductionURL = "https://payments.wavemoney.io"

// Config holds Wave Pay (Pay with Wave) credentials and endpoints. Every field is required except
// TimeoutSeconds.
type Config struct {
	// MerchantID is the merchant id Wave issued.
	MerchantID string
	// SecretKey is the hash secret key Wave issued.
	SecretKey string
	// MerchantName is your business name, shown on Wave's payment page.
	MerchantName string
	// TimeToLiveSeconds is how long the customer has to pay, a whole number greater than 0. A
	// payment's own TimeToLiveSeconds overrides it.
	TimeToLiveSeconds int
	// TimeoutSeconds is the timeout of the default HTTP client, a whole number greater than 0
	// (optional; 0 means myanmarpayments.DefaultTimeoutSeconds, 30). A client you pass to New
	// keeps its own timeout.
	TimeoutSeconds int
	// BaseURL is the API base URL, e.g. ProductionURL or https://preprodpayments.wavemoney.io:8107.
	// Payments are requested at its /payment.
	BaseURL string
	// BaseRedirectURL is the host the customer is sent to, e.g. ProductionURL or
	// https://preprodpayments.wavemoney.io:8107. The customer approves the payment at its /authenticate.
	BaseRedirectURL string
}

// ConfigFromEnv reads WAVE_PAY_MERCHANT_ID, WAVE_PAY_SECRET_KEY, WAVE_PAY_MERCHANT_NAME,
// WAVE_PAY_BASE_URL, WAVE_PAY_BASE_REDIRECT_URL, WAVE_PAY_TIME_TO_LIVE_IN_SECONDS and
// MYANMAR_PAYMENTS_HTTP_TIMEOUT.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		MerchantID:        env.First(getenv, "WAVE_PAY_MERCHANT_ID"),
		SecretKey:         env.First(getenv, "WAVE_PAY_SECRET_KEY"),
		MerchantName:      env.First(getenv, "WAVE_PAY_MERCHANT_NAME"),
		TimeToLiveSeconds: env.Seconds(getenv, "WAVE_PAY_TIME_TO_LIVE_IN_SECONDS"),
		TimeoutSeconds:    env.Timeout(getenv),
		BaseURL:           env.First(getenv, "WAVE_PAY_BASE_URL"),
		BaseRedirectURL:   env.First(getenv, "WAVE_PAY_BASE_REDIRECT_URL"),
	}
}

func (c Config) validate() error {
	if err := env.Require("wave_pay", "merchant_id", c.MerchantID, "secret_key", c.SecretKey, "merchant_name", c.MerchantName, "base_url", c.BaseURL, "base_redirect_url", c.BaseRedirectURL); err != nil {
		return err
	}
	if err := env.RequiredSeconds("wave_pay", "time_to_live_in_seconds", c.TimeToLiveSeconds); err != nil {
		return err
	}
	return env.OptionalSeconds("wave_pay", "timeout_in_seconds", c.TimeoutSeconds)
}

// ResolvedBaseURL returns BaseURL without its trailing slash.
func (c Config) ResolvedBaseURL() string {
	return strings.TrimRight(c.BaseURL, "/")
}

// ResolvedBaseRedirectURL returns BaseRedirectURL without its trailing slash.
func (c Config) ResolvedBaseRedirectURL() string {
	return strings.TrimRight(c.BaseRedirectURL, "/")
}
