package kbzpay

import (
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// Production endpoints of KBZ Pay. They name production's values for APIURL and PWAURL; they are
// not defaults.
const (
	ProductionAPIURL = "https://api.kbzpay.com/payment/gateway"
	ProductionPWAURL = "https://wap.kbzpay.com/pgw/pwa/#/"
)

// TimeToLiveRequirement is what a valid Config.TimeToLiveMinutes is: KBZ counts whole minutes,
// up to 120.
const TimeToLiveRequirement = "a whole number from 1 to 120"

// Config holds KBZ Pay credentials and endpoints. Every field is required except TimeoutSeconds;
// UAT and production issue separate credentials and use separate URLs (ProductionAPIURL and
// ProductionPWAURL name production's).
type Config struct {
	// AppID is the appid KBZ issued for your merchant app.
	AppID string
	// AppKey is the secret key used to sign requests.
	AppKey string
	// MerchantCode is the merch_code KBZ issued.
	MerchantCode string
	// TimeoutSeconds is the timeout of the default HTTP client, a whole number greater than 0
	// (optional; 0 means myanmarpayments.DefaultTimeoutSeconds, 30). A client you pass to New
	// keeps its own timeout.
	TimeoutSeconds int
	// APIURL is the API base URL, e.g. ProductionAPIURL or
	// http://api-uat.kbzpay.com/payment/gateway/uat.
	APIURL string
	// PWAURL is the PWA checkout URL, e.g. ProductionPWAURL or
	// https://static.kbzpay.com/pgw/uat/pwa/#/. A trailing "#" or "#/" is normalized to "#/".
	PWAURL string
	// TimeToLiveMinutes is how long the customer has to pay each order, 1 to 120 minutes. A
	// payment's own TimeToLiveMinutes overrides it.
	TimeToLiveMinutes int
}

// ConfigFromEnv reads KBZ_PAY_APP_ID, KBZ_PAY_APP_KEY, KBZ_PAY_MERCHANT_CODE, KBZ_PAY_BASE_URL,
// KBZ_PAY_PWA_BASE_REDIRECT_URL, KBZ_PAY_TIME_TO_LIVE_IN_MINUTES and
// MYANMAR_PAYMENTS_HTTP_TIMEOUT.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		AppID:             env.First(getenv, "KBZ_PAY_APP_ID"),
		AppKey:            env.First(getenv, "KBZ_PAY_APP_KEY"),
		MerchantCode:      env.First(getenv, "KBZ_PAY_MERCHANT_CODE"),
		TimeoutSeconds:    env.Timeout(getenv),
		APIURL:            env.First(getenv, "KBZ_PAY_BASE_URL"),
		PWAURL:            env.First(getenv, "KBZ_PAY_PWA_BASE_REDIRECT_URL"),
		TimeToLiveMinutes: env.Seconds(getenv, "KBZ_PAY_TIME_TO_LIVE_IN_MINUTES"),
	}
}

func (c Config) validate() error {
	if err := env.Require("kbz_pay", "app_id", c.AppID, "app_key", c.AppKey, "merchant_code", c.MerchantCode, "api_url", c.APIURL, "pwa_url", c.PWAURL); err != nil {
		return err
	}
	if c.TimeToLiveMinutes == 0 {
		return &myanmarpayments.ConfigurationError{Gateway: "kbz_pay", Key: "time_to_live_in_minutes"}
	}
	if ttl := c.TimeToLiveMinutes; ttl < 1 || ttl > 120 {
		return &myanmarpayments.ConfigurationError{Gateway: "kbz_pay", Key: "time_to_live_in_minutes", Invalid: true, Requirement: TimeToLiveRequirement}
	}
	return env.OptionalSeconds("kbz_pay", "timeout_in_seconds", c.TimeoutSeconds)
}

// ResolvedAPIURL returns APIURL without its trailing slash.
func (c Config) ResolvedAPIURL() string {
	return strings.TrimRight(c.APIURL, "/")
}

// ResolvedPWAURL returns PWAURL, always ending in "/".
func (c Config) ResolvedPWAURL() string {
	return strings.TrimRight(c.PWAURL, "/") + "/"
}
