package cybersource

import (
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/env"
)

// Secure Acceptance hosts.
const (
	SandboxURL    = "https://testsecureacceptance.cybersource.com"
	ProductionURL = "https://secureacceptance.cybersource.com"
)

// Config holds a Secure Acceptance profile. The zero value of Production selects the test host.
type Config struct {
	// ProfileID is the Secure Acceptance profile id.
	ProfileID string
	// AccessKey is the profile's access key.
	AccessKey string
	// SecretKey signs the fields.
	SecretKey string
	// Production selects the production host.
	Production bool
	// BaseURL overrides the Secure Acceptance base URL.
	BaseURL string
}

// ConfigFromEnv reads the CYBER_SOURCE_* variables.
func ConfigFromEnv(getenv func(string) string) Config {
	return Config{
		ProfileID:  env.First(getenv, "CYBER_SOURCE_PROFILE_ID"),
		AccessKey:  env.First(getenv, "CYBER_SOURCE_ACCESS_KEY"),
		SecretKey:  env.First(getenv, "CYBER_SOURCE_SECRET_KEY"),
		Production: env.Production(getenv, "CYBER_SOURCE_SANDBOX"),
		BaseURL:    env.First(getenv, "CYBER_SOURCE_BASE_URL"),
	}
}

func (c Config) validate() error {
	return env.Require("cyber_source", "profile_id", c.ProfileID, "access_key", c.AccessKey, "secret_key", c.SecretKey)
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
