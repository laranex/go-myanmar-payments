package kbzpay

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// Endpoints from the KBZ Pay UAT documentation.
const (
	SandboxAPIURL    = "http://api-uat.kbzpay.com/payment/gateway/uat"
	ProductionAPIURL = "https://api.kbzpay.com/payment/gateway"
	SandboxPWAURL    = "https://static.kbzpay.com/pgw/uat/pwa/#/"
	ProductionPWAURL = "https://wap.kbzpay.com/pgw/pwa/#/"
)

// Config holds KBZ Pay credentials. UAT and production issue separate credentials.
//
// The zero value of Production selects the UAT (sandbox) endpoints.
type Config struct {
	// AppID is the appid KBZ issued for your merchant app.
	AppID string
	// AppKey is the secret key used to sign requests.
	AppKey string
	// MerchantCode is the merch_code KBZ issued.
	MerchantCode string
	// Production selects the production endpoints instead of UAT.
	Production bool
	// APIURL overrides the API base URL, e.g. to go through a proxy.
	APIURL string
	// PWAURL overrides the PWA checkout URL. A trailing "#" or "#/" is normalized to "#/".
	PWAURL string
}

// ConfigFromEnv reads KBZ_PAY_APP_ID, KBZ_PAY_APP_KEY, KBZ_PAY_MERCHANT_CODE, KBZ_PAY_SANDBOX,
// KBZ_PAY_BASE_URL and KBZ_PAY_PWA_BASE_REDIRECT_URL.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		AppID:        env.First(getenv, "KBZ_PAY_APP_ID"),
		AppKey:       env.First(getenv, "KBZ_PAY_APP_KEY"),
		MerchantCode: env.First(getenv, "KBZ_PAY_MERCHANT_CODE"),
		Production:   env.Production(getenv, "KBZ_PAY_SANDBOX"),
		APIURL:       env.First(getenv, "KBZ_PAY_BASE_URL"),
		PWAURL:       env.First(getenv, "KBZ_PAY_PWA_BASE_REDIRECT_URL"),
	}
}

func (c Config) validate() error {
	return env.Require("kbz_pay", "app_id", c.AppID, "app_key", c.AppKey, "merchant_code", c.MerchantCode)
}

// ResolvedAPIURL returns the API base URL in use.
func (c Config) ResolvedAPIURL() string {
	if c.APIURL != "" {
		return strings.TrimRight(c.APIURL, "/")
	}
	if c.Production {
		return ProductionAPIURL
	}
	return SandboxAPIURL
}

// ResolvedPWAURL returns the PWA checkout URL in use, always ending in "/".
func (c Config) ResolvedPWAURL() string {
	pwa := c.PWAURL
	if pwa == "" {
		pwa = SandboxPWAURL
		if c.Production {
			pwa = ProductionPWAURL
		}
	}
	return strings.TrimRight(pwa, "/") + "/"
}
