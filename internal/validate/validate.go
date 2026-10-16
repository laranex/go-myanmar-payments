// Package validate collects validation errors for payment data.
package validate

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

// Validator collects one error per field.
type Validator struct {
	errors map[string]string
}

// New returns an empty Validator.
func New() *Validator { return &Validator{errors: map[string]string{}} }

func (v *Validator) fail(field, message string) {
	if _, ok := v.errors[field]; !ok {
		v.errors[field] = message
	}
}

// Required fails when value is blank: empty or only ASCII whitespace and NUL.
func (v *Validator) Required(field, value string) *Validator {
	if values.IsBlank(value) {
		v.fail(field, fmt.Sprintf("The %s field is required.", field))
	}
	return v
}

// Length fails when a non-empty value is shorter than min or longer than max bytes.
func (v *Validator) Length(field, value string, min, max int) *Validator {
	if value != "" && (len(value) < min || len(value) > max) {
		if min == max {
			v.fail(field, fmt.Sprintf("The %s field must be %d characters.", field, min))
		} else {
			v.fail(field, fmt.Sprintf("The %s field must be between %d and %d characters.", field, min, max))
		}
	}
	return v
}

// Max fails when value is longer than max bytes.
func (v *Validator) Max(field, value string, max int) *Validator {
	if len(value) > max {
		v.fail(field, fmt.Sprintf("The %s field must not be greater than %d characters.", field, max))
	}
	return v
}

// Pattern fails when a non-empty value does not match pattern.
func (v *Validator) Pattern(field, value string, pattern *regexp.Regexp, description string) *Validator {
	if value != "" && !pattern.MatchString(value) {
		v.fail(field, fmt.Sprintf("The %s field may only contain %s.", field, description))
	}
	return v
}

// Between fails when value is set (non-nil) and outside [min, max].
func (v *Validator) Between(field string, value *int, min, max int) *Validator {
	if value != nil && (*value < min || *value > max) {
		v.fail(field, fmt.Sprintf("The %s field must be between %d and %d.", field, min, max))
	}
	return v
}

// AmountRule describes what a gateway's documentation allows for an amount.
type AmountRule struct {
	// Gateway is the gateway's display name, used in error messages.
	Gateway string
	// MaxDecimals is the most fractional digits allowed; 0 means whole amounts only and -1 means no limit.
	MaxDecimals int
	// MaxLength limits the amount's text length; 0 means no limit.
	MaxLength int
	// AllowZero accepts an amount of 0.
	AllowZero bool
}

// Amount checks an amount against a gateway's documented rules.
func (v *Validator) Amount(field string, amount myanmarpayments.Amount, rule AmountRule) *Validator {
	switch {
	case !amount.IsSet():
		v.fail(field, fmt.Sprintf("The %s field is required.", field))
	case negativeAmount.MatchString(amount.String()):
		v.fail(field, fmt.Sprintf("The %s field must not be negative.", field))
	case !amount.Valid():
		v.fail(field, fmt.Sprintf("The %s field must be a non-negative number such as 1000 or 1000.50.", field))
	case rule.MaxDecimals == 0 && amount.DecimalPlaces() > 0:
		v.fail(field, fmt.Sprintf("%s does not accept decimal amounts; the %s field must be a whole number.", rule.Gateway, field))
	case rule.MaxDecimals > 0 && amount.DecimalPlaces() > rule.MaxDecimals:
		v.fail(field, fmt.Sprintf("%s accepts at most %d decimal places; the %s field has %d.", rule.Gateway, rule.MaxDecimals, field, amount.DecimalPlaces()))
	case !rule.AllowZero && amount.IsZero():
		v.fail(field, fmt.Sprintf("The %s field must be greater than 0.", field))
	case rule.MaxLength > 0 && len(amount.String()) > rule.MaxLength:
		v.fail(field, fmt.Sprintf("The %s field must not be greater than %d characters.", field, rule.MaxLength))
	}

	return v
}

// URL fails when a non-empty value is not an absolute http(s) URL (see IsHTTPURL).
func (v *Validator) URL(field, value string) *Validator {
	if value != "" && !IsHTTPURL(value) {
		v.fail(field, fmt.Sprintf("The %s field must be a valid http or https URL.", field))
	}
	return v
}

var (
	negativeAmount = regexp.MustCompile(`^-[0-9]+$`)
	hostLabel      = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$`)
	portPattern    = regexp.MustCompile(`^[0-9]{1,5}$`)
)

// IsHTTPURL reports whether value is a URL the way PHP's FILTER_VALIDATE_URL with an http or
// https scheme accepts it, the rule every Laranex SDK shares: printable ASCII only (no
// whitespace or control characters); "http://" or "https://" in any case followed by a host; the
// host is a bracketed IP literal or dot-separated labels of letters, digits and inner "-" (1 to
// 63 characters each, at most 253 in all, one trailing dot allowed); a port is 0 to 65535. User
// info, path, query and fragment may hold any printable ASCII.
func IsHTTPURL(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < 0x21 || value[i] > 0x7e {
			return false
		}
	}
	lower := strings.ToLower(value)
	var rest string
	switch {
	case strings.HasPrefix(lower, "http://"):
		rest = value[len("http://"):]
	case strings.HasPrefix(lower, "https://"):
		rest = value[len("https://"):]
	default:
		return false
	}

	authority := rest
	if end := strings.IndexAny(rest, "/?#"); end >= 0 {
		authority = rest[:end]
	}
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		authority = authority[at+1:]
	}

	host, port, hasPort := authority, "", false
	if strings.HasPrefix(authority, "[") {
		end := strings.Index(authority, "]")
		if end < 0 {
			return false
		}
		host, port = authority[:end+1], authority[end+1:]
		if port != "" && !strings.HasPrefix(port, ":") {
			return false
		}
		hasPort = port != ""
		port = strings.TrimPrefix(port, ":")
		if ip := net.ParseIP(host[1 : len(host)-1]); ip == nil || !strings.Contains(host, ":") {
			return false
		}
	} else {
		if colon := strings.LastIndex(authority, ":"); colon >= 0 {
			host, port, hasPort = authority[:colon], authority[colon+1:], true
		}
		if !validHost(host) {
			return false
		}
	}

	if hasPort && !portPattern.MatchString(port) {
		return false
	}
	if hasPort {
		number, err := strconv.Atoi(port)
		if err != nil || number > 65535 {
			return false
		}
	}
	return true
}

// validHost reports whether host is dot-separated LDH labels of 1 to 63 characters, at most 253
// characters in all, with one optional trailing dot.
func validHost(host string) bool {
	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) > 63 || !hostLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// When fails with message when condition is true.
func (v *Validator) When(condition bool, field, message string) *Validator {
	if condition {
		v.fail(field, message)
	}
	return v
}

// Err returns an *InvalidPaymentDataError when any check failed.
func (v *Validator) Err() error {
	if len(v.errors) == 0 {
		return nil
	}
	return &myanmarpayments.InvalidPaymentDataError{Errors: v.errors}
}
