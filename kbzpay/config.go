package kbzpay

import (
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// Production endpoints of KBZ Pay. To use UAT or a proxy, set APIURL and PWAURL.
const (
	ProductionAPIURL = "https://api.kbzpay.com/payment/gateway"
	ProductionPWAURL = "https://wap.kbzpay.com/pgw/pwa/#/"
)

// TimeToLiveRequirement is what a valid Config.TimeToLiveSeconds is: KBZ counts whole minutes,
// up to 120.
const TimeToLiveRequirement = "a multiple of 60 from 60 to 7200"

// Config holds KBZ Pay credentials. UAT and production issue separate credentials. Every field
// except TimeoutSeconds, TimeToLiveSeconds and the URL overrides is required.
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
	// APIURL overrides the API base URL, e.g. the UAT one or a proxy.
	APIURL string
	// PWAURL overrides the PWA checkout URL. A trailing "#" or "#/" is normalized to "#/".
	PWAURL string
	// TimeToLiveSeconds is how long the customer has to pay each order, 60 to 7200 seconds in
	// whole minutes (optional; 0 leaves it to KBZ, 7200). A payment's own TimeToLiveSeconds
	// overrides it.
	TimeToLiveSeconds int
}

// ConfigFromEnv reads KBZ_PAY_APP_ID, KBZ_PAY_APP_KEY, KBZ_PAY_MERCHANT_CODE,
// MYANMAR_PAYMENTS_HTTP_TIMEOUT, KBZ_PAY_BASE_URL, KBZ_PAY_PWA_BASE_REDIRECT_URL and
// KBZ_PAY_TIME_TO_LIVE_IN_SECONDS.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		AppID:             env.First(getenv, "KBZ_PAY_APP_ID"),
		AppKey:            env.First(getenv, "KBZ_PAY_APP_KEY"),
		MerchantCode:      env.First(getenv, "KBZ_PAY_MERCHANT_CODE"),
		TimeoutSeconds:    env.Timeout(getenv),
		APIURL:            env.First(getenv, "KBZ_PAY_BASE_URL"),
		PWAURL:            env.First(getenv, "KBZ_PAY_PWA_BASE_REDIRECT_URL"),
		TimeToLiveSeconds: env.Seconds(getenv, "KBZ_PAY_TIME_TO_LIVE_IN_SECONDS"),
	}
}

func (c Config) validate() error {
	if err := env.Require("kbz_pay", "app_id", c.AppID, "app_key", c.AppKey, "merchant_code", c.MerchantCode); err != nil {
		return err
	}
	if err := env.OptionalSeconds("kbz_pay", "timeout_in_seconds", c.TimeoutSeconds); err != nil {
		return err
	}
	if ttl := c.TimeToLiveSeconds; ttl != 0 && (ttl < 60 || ttl > 7200 || ttl%60 != 0) {
		return &myanmarpayments.ConfigurationError{Gateway: "kbz_pay", Key: "time_to_live_in_seconds", Invalid: true, Requirement: TimeToLiveRequirement}
	}
	return nil
}

// ResolvedAPIURL returns the API base URL in use: the override, or the production URL.
func (c Config) ResolvedAPIURL() string {
	if strings.TrimSpace(c.APIURL) != "" {
		return strings.TrimRight(c.APIURL, "/")
	}
	return ProductionAPIURL
}

// ResolvedPWAURL returns the PWA checkout URL in use, always ending in "/".
func (c Config) ResolvedPWAURL() string {
	pwa := c.PWAURL
	if strings.TrimSpace(pwa) == "" {
		pwa = ProductionPWAURL
	}
	return strings.TrimRight(pwa, "/") + "/"
}
