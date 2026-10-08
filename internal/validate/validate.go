// Package validate collects validation errors for payment data.
package validate

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
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

// Between fails when value is set (non-zero) and outside [min, max].
func (v *Validator) Between(field string, value, min, max int) *Validator {
	if value != 0 && (value < min || value > max) {
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

// URL fails when a non-empty value is not an absolute http(s) URL.
func (v *Validator) URL(field, value string) *Validator {
	if value == "" {
		return v
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		v.fail(field, fmt.Sprintf("The %s field must be a valid http or https URL.", field))
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
