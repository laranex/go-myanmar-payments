package wavemoney

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// ProductionURL is the production endpoint from the WPPG documentation. To use UAT or a proxy, set
// BaseURL: payments are requested at its /payment and the customer is sent to its /authenticate.
const ProductionURL = "https://payments.wavemoney.io"

// Config holds Wave Money credentials. Every field except TimeToLiveSeconds, TimeoutSeconds and the
// URL overrides is required.
type Config struct {
	// MerchantID is the merchant id Wave issued.
	MerchantID string
	// SecretKey is the hash secret key Wave issued.
	SecretKey string
	// MerchantName is your business name, shown on Wave's payment page.
	MerchantName string
	// TimeToLiveSeconds is how long the customer has to pay, a whole number greater than 0
	// (optional; 0 means unset). A payment's own TimeToLiveSeconds overrides it; with neither set,
	// Initiate returns *myanmarpayments.InvalidPaymentDataError.
	TimeToLiveSeconds int
	// TimeoutSeconds is the timeout of the default HTTP client, a whole number greater than 0
	// (optional; 0 means myanmarpayments.DefaultTimeoutSeconds, 30). A client you pass to New
	// keeps its own timeout.
	TimeoutSeconds int
	// BaseURL overrides the API base URL; the customer is sent to its /authenticate page.
	BaseURL string
}

// ConfigFromEnv reads WAVE_MONEY_MERCHANT_ID, WAVE_MONEY_SECRET_KEY, WAVE_MONEY_MERCHANT_NAME,
// WAVE_MONEY_TIME_TO_LIVE_IN_SECONDS, MYANMAR_PAYMENTS_HTTP_TIMEOUT and WAVE_MONEY_BASE_URL.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		MerchantID:        env.First(getenv, "WAVE_MONEY_MERCHANT_ID"),
		SecretKey:         env.First(getenv, "WAVE_MONEY_SECRET_KEY"),
		MerchantName:      env.First(getenv, "WAVE_MONEY_MERCHANT_NAME"),
		TimeToLiveSeconds: env.Seconds(getenv, "WAVE_MONEY_TIME_TO_LIVE_IN_SECONDS"),
		TimeoutSeconds:    env.Timeout(getenv),
		BaseURL:           env.First(getenv, "WAVE_MONEY_BASE_URL"),
	}
}

func (c Config) validate() error {
	if err := env.Require("wave_money", "merchant_id", c.MerchantID, "secret_key", c.SecretKey, "merchant_name", c.MerchantName); err != nil {
		return err
	}
	if err := env.OptionalSeconds("wave_money", "time_to_live_in_seconds", c.TimeToLiveSeconds); err != nil {
		return err
	}
	return env.OptionalSeconds("wave_money", "timeout_in_seconds", c.TimeoutSeconds)
}

// ResolvedBaseURL returns the API base URL in use: the override, or the production URL.
func (c Config) ResolvedBaseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return ProductionURL
}
