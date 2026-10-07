// Package validate collects validation errors for payment data.
package validate

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments"
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

// Required fails when value is blank.
func (v *Validator) Required(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.fail(field, fmt.Sprintf("The %s field is required.", field))
	}
	return v
}

// Length fails when a non-empty value is shorter than min or longer than max bytes.
func (v *Validator) Length(field, value string, min, max int) *Validator {
	if value != "" && (len(value) < min || len(value) > max) {
		v.fail(field, fmt.Sprintf("The %s field must be between %d and %d characters.", field, min, max))
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

// Positive fails when value is not greater than zero.
func (v *Validator) Positive(field string, value int64) *Validator {
	if value <= 0 {
		v.fail(field, fmt.Sprintf("The %s field must be greater than 0.", field))
	}
	return v
}

// Between fails when value is set (non-zero) and outside [min, max].
func (v *Validator) Between(field string, value, min, max int) *Validator {
	if value != 0 && (value < min || value > max) {
		v.fail(field, fmt.Sprintf("The %s field must be between %d and %d.", field, min, max))
	}
	return v
}

// Decimal validates a non-negative decimal string with at most maxDecimals places.
// maxLength of 0 means unlimited.
func (v *Validator) Decimal(field, value string, maxDecimals, maxLength int, allowZero bool) *Validator {
	pattern := `^\d+$`
	if maxDecimals > 0 {
		pattern = fmt.Sprintf(`^\d+(\.\d{1,%d})?$`, maxDecimals)
	}
	if !regexp.MustCompile(pattern).MatchString(value) {
		if maxDecimals > 0 {
			v.fail(field, fmt.Sprintf("The %s field must be a number with at most %d decimal places.", field, maxDecimals))
		} else {
			v.fail(field, fmt.Sprintf("The %s field must be a whole number.", field))
		}
		return v
	}
	if !allowZero && strings.Trim(strings.ReplaceAll(value, ".", ""), "0") == "" {
		v.fail(field, fmt.Sprintf("The %s field must be greater than 0.", field))
	}
	if maxLength > 0 && len(value) > maxLength {
		v.fail(field, fmt.Sprintf("The %s field must not be greater than %d characters.", field, maxLength))
	}
	return v
}

// URL fails when a non-empty value is not an absolute http(s) URL.
func (v *Validator) URL(field, value string) *Validator {
	if value == "" {
		return v
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		v.fail(field, fmt.Sprintf("The %s field must be a valid URL.", field))
	}
	return v
}

// HTTPS fails when a non-empty value is not an https URL.
func (v *Validator) HTTPS(field, value string) *Validator {
	if value != "" && !strings.HasPrefix(strings.ToLower(value), "https://") {
		v.fail(field, fmt.Sprintf("The %s field must be an HTTPS URL.", field))
	}
	return v
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
