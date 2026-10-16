// Package env reads gateway configuration from environment variables.
package env

import (
	"regexp"
	"strconv"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

// Getter looks up a variable, e.g. os.Getenv.
type Getter func(string) string

// TimeoutKey is the variable every gateway that calls an API reads its HTTP timeout from.
const TimeoutKey = "MYANMAR_PAYMENTS_HTTP_TIMEOUT"

// MaxInteger is the largest whole number a setting may hold, the same in every Laranex SDK
// (2^31 - 1).
const MaxInteger = 2147483647

// First returns the first non-empty value among keys, trimmed of ASCII whitespace and NUL.
func First(get Getter, keys ...string) string {
	for _, key := range keys {
		if value := values.TrimASCII(get(key)); value != "" {
			return value
		}
	}
	return ""
}

var wholeNumber = regexp.MustCompile(`^[+-]?[0-9]+$`)

// Seconds reads a whole number of seconds: 0 when unset or blank, the value when it is a whole
// number from 1 to MaxInteger, and -1 for anything else, so validation reports it as invalid.
func Seconds(get Getter, key string) int {
	value := values.TrimASCII(get(key))
	if value == "" {
		return 0
	}
	if !wholeNumber.MatchString(value) {
		return -1
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 || seconds > MaxInteger {
		return -1
	}
	return int(seconds)
}

// Timeout reads TimeoutKey like Seconds, but returns myanmarpayments.DefaultTimeoutSeconds when
// it is unset or blank.
func Timeout(get Getter) int {
	return TimeoutSeconds(Seconds(get, TimeoutKey))
}

// TimeoutSeconds returns seconds, or myanmarpayments.DefaultTimeoutSeconds when it is 0 (unset).
func TimeoutSeconds(seconds int) int {
	if seconds == 0 {
		return myanmarpayments.DefaultTimeoutSeconds
	}
	return seconds
}

// Require returns a ConfigurationError for the first blank value (empty or only ASCII whitespace
// and NUL), given key/value pairs.
func Require(gateway string, keyValues ...string) error {
	for i := 0; i+1 < len(keyValues); i += 2 {
		if values.IsBlank(keyValues[i+1]) {
			return &myanmarpayments.ConfigurationError{Gateway: gateway, Key: keyValues[i]}
		}
	}
	return nil
}

// OptionalSeconds returns a ConfigurationError when seconds is negative or above MaxInteger
// (invalid); 0 means the setting is unset and its default applies.
func OptionalSeconds(gateway, key string, seconds int) error {
	if seconds < 0 || seconds > MaxInteger {
		return &myanmarpayments.ConfigurationError{Gateway: gateway, Key: key, Invalid: true}
	}
	return nil
}

// RequiredSeconds returns a ConfigurationError when seconds is 0 (missing) or negative (invalid).
func RequiredSeconds(gateway, key string, seconds int) error {
	if seconds == 0 {
		return &myanmarpayments.ConfigurationError{Gateway: gateway, Key: key}
	}
	return OptionalSeconds(gateway, key, seconds)
}
