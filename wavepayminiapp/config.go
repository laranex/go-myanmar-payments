package wavepayminiapp

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// ProductionURL is production's API base URL from the Wave Money JS SDK documentation, the
// production value of BaseURL; it is not a default. UAT uses https://preprodnewapi.wavemoney.io.
const ProductionURL = "https://api.wavemoney.io:8100"

// Config holds Wave Pay Mini App (Wave Money JS SDK) credentials and endpoint. Every field is
// required except TimeoutSeconds.
type Config struct {
	// ClientID is the client id Wave issued, sent to the token endpoint.
	ClientID string
	// ClientSecret is the client secret Wave issued, sent to the token endpoint.
	ClientSecret string
	// MiniAppID is the mini app id Wave issued (miniAppID in makePayment).
	MiniAppID string
	// SecretKey is the hash secret key Wave issued for payment and callback hashes.
	SecretKey string
	// TimeoutSeconds is the timeout of the default HTTP client, a whole number greater than 0
	// (optional; 0 means myanmarpayments.DefaultTimeoutSeconds, 30). A client you pass to New
	// keeps its own timeout.
	TimeoutSeconds int
	// BaseURL is the API base URL, e.g. ProductionURL or https://preprodnewapi.wavemoney.io.
	// Access tokens are requested at its /auth/v1/external-utility/token.
	BaseURL string
}

// ConfigFromEnv reads WAVE_PAY_MINI_APP_CLIENT_ID, WAVE_PAY_MINI_APP_CLIENT_SECRET,
// WAVE_PAY_MINI_APP_MINI_APP_ID, WAVE_PAY_MINI_APP_SECRET_KEY, WAVE_PAY_MINI_APP_BASE_URL and
// MYANMAR_PAYMENTS_HTTP_TIMEOUT.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		ClientID:       env.First(getenv, "WAVE_PAY_MINI_APP_CLIENT_ID"),
		ClientSecret:   env.First(getenv, "WAVE_PAY_MINI_APP_CLIENT_SECRET"),
		MiniAppID:      env.First(getenv, "WAVE_PAY_MINI_APP_MINI_APP_ID"),
		SecretKey:      env.First(getenv, "WAVE_PAY_MINI_APP_SECRET_KEY"),
		TimeoutSeconds: env.Timeout(getenv),
		BaseURL:        env.First(getenv, "WAVE_PAY_MINI_APP_BASE_URL"),
	}
}

// validate checks client_id, client_secret, mini_app_id, secret_key, base_url, then the timeout,
// the same order as the other Laranex SDKs.
func (c Config) validate() error {
	if err := env.Require("wave_pay_mini_app", "client_id", c.ClientID, "client_secret", c.ClientSecret, "mini_app_id", c.MiniAppID, "secret_key", c.SecretKey, "base_url", c.BaseURL); err != nil {
		return err
	}
	return env.OptionalSeconds("wave_pay_mini_app", "timeout_in_seconds", c.TimeoutSeconds)
}

// ResolvedBaseURL returns BaseURL without its trailing slash.
func (c Config) ResolvedBaseURL() string {
	return strings.TrimRight(c.BaseURL, "/")
}
