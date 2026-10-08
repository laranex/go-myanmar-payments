// Package env reads gateway configuration from environment variables.
package env

import (
	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"strconv"
	"strings"
)

// Getter looks up a variable, e.g. os.Getenv.
type Getter func(string) string

// First returns the first non-empty value among keys.
func First(get Getter, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(get(key)); value != "" {
			return value
		}
	}
	return ""
}

// Production reads a *_SANDBOX variable and reports whether production was requested.
// Unset or unparsable values mean sandbox.
func Production(get Getter, sandboxKey string) bool {
	value := strings.TrimSpace(get(sandboxKey))
	if value == "" {
		return false
	}
	sandbox, err := strconv.ParseBool(strings.ToLower(value))
	if err != nil {
		switch strings.ToLower(value) {
		case "no", "off":
			return true
		default:
			return false
		}
	}
	return !sandbox
}

// Int reads an integer, returning def when unset or invalid.
func Int(get Getter, key string, def int) int {
	value, err := strconv.Atoi(strings.TrimSpace(get(key)))
	if err != nil {
		return def
	}
	return value
}

// Require returns a ConfigurationError for the first blank value, given key/value pairs.
func Require(gateway string, keyValues ...string) error {
	for i := 0; i+1 < len(keyValues); i += 2 {
		if strings.TrimSpace(keyValues[i+1]) == "" {
			return &myanmarpayments.ConfigurationError{Gateway: gateway, Key: keyValues[i]}
		}
	}
	return nil
}
