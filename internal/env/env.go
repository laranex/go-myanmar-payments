// Package env reads gateway configuration from environment variables.
package env

import (
	"regexp"
	"strconv"
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

// Getter looks up a variable, e.g. os.Getenv.
type Getter func(string) string

// TimeoutKey is the variable every gateway that calls an API reads its HTTP timeout from.
const TimeoutKey = "MYANMAR_PAYMENTS_HTTP_TIMEOUT"

// First returns the first non-empty value among keys.
func First(get Getter, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(get(key)); value != "" {
			return value
		}
	}
	return ""
}

var wholeNumber = regexp.MustCompile(`^[+-]?[0-9]+$`)

// Seconds reads a whole number of seconds: 0 when unset or blank, the value when it is a whole
// number greater than 0, and -1 for anything else, so validation reports it as invalid.
func Seconds(get Getter, key string) int {
	value := strings.TrimSpace(get(key))
	if value == "" {
		return 0
	}
	if !wholeNumber.MatchString(value) {
		return -1
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return -1
	}
	return seconds
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

// RequireSeconds returns a ConfigurationError when seconds is 0 (missing) or negative (invalid).
func RequireSeconds(gateway, key string, seconds int) error {
	switch {
	case seconds == 0:
		return &myanmarpayments.ConfigurationError{Gateway: gateway, Key: key}
	case seconds < 0:
		return &myanmarpayments.ConfigurationError{Gateway: gateway, Key: key, Invalid: true}
	}
	return nil
}
