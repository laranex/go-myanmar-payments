package cybersource

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// ProductionURL is the Secure Acceptance production host. To use the test host or a proxy, set
// BaseURL.
const ProductionURL = "https://secureacceptance.cybersource.com"

// Config holds a Secure Acceptance profile. Every field except BaseURL is required.
type Config struct {
	// ProfileID is the Secure Acceptance profile id.
	ProfileID string
	// AccessKey is the profile's access key.
	AccessKey string
	// SecretKey signs the fields.
	SecretKey string
	// BaseURL overrides the Secure Acceptance base URL.
	BaseURL string
}

// ConfigFromEnv reads CYBER_SOURCE_PROFILE_ID, CYBER_SOURCE_ACCESS_KEY, CYBER_SOURCE_SECRET_KEY
// and CYBER_SOURCE_BASE_URL.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		ProfileID: env.First(getenv, "CYBER_SOURCE_PROFILE_ID"),
		AccessKey: env.First(getenv, "CYBER_SOURCE_ACCESS_KEY"),
		SecretKey: env.First(getenv, "CYBER_SOURCE_SECRET_KEY"),
		BaseURL:   env.First(getenv, "CYBER_SOURCE_BASE_URL"),
	}
}

func (c Config) validate() error {
	return env.Require("cyber_source", "profile_id", c.ProfileID, "access_key", c.AccessKey, "secret_key", c.SecretKey)
}

// ResolvedBaseURL returns the base URL in use: the override, or the production URL.
func (c Config) ResolvedBaseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return ProductionURL
}
