package myanmarpayments

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var amountPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)

// Amount is an exact, non-negative money amount in a gateway's currency, kept as decimal text so
// it is never rounded through a float. Build one with Kyat for whole amounts or ParseAmount for
// decimal ones; each gateway's Validate then checks it against that gateway's documented rules
// (for example, Wave Money, AYA and Yoma MMQR only accept whole kyat).
//
// The zero value means "not provided". Amount is immutable and safe to copy.
type Amount struct {
	value string
}

// Kyat returns a whole-unit amount, e.g. Kyat(1000) for 1000 MMK.
//
// It takes an int64 so plain integer variables need no conversion. A negative n does not panic:
// it yields an Amount that every gateway's Validate rejects, so bad input surfaces as an
// *InvalidPaymentDataError like any other field.
func Kyat(n int64) Amount {
	return Amount{value: strconv.FormatInt(n, 10)}
}

// ParseAmount parses a decimal amount such as "1000" or "1000.50". Only digits with an optional
// fractional part are accepted: no sign, exponent, spaces or thousands separators. Leading zeros
// of the whole part are removed; the fractional digits are kept exactly as given.
//
// On bad input it returns an *InvalidPaymentDataError for the "amount" field.
func ParseAmount(s string) (Amount, error) {
	if !amountPattern.MatchString(s) {
		return Amount{}, &InvalidPaymentDataError{Errors: map[string]string{
			"amount": fmt.Sprintf("The amount field must be a number such as 1000 or 1000.50, got %q.", s),
		}}
	}

	whole, fraction, hasFraction := strings.Cut(s, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	if hasFraction {
		return Amount{value: whole + "." + fraction}, nil
	}

	return Amount{value: whole}, nil
}

// MustParseAmount is like ParseAmount but panics on bad input. Use it for constants and tests.
func MustParseAmount(s string) Amount {
	amount, err := ParseAmount(s)
	if err != nil {
		panic(err)
	}

	return amount
}

// String returns the amount exactly as it is sent to the gateway, e.g. "1000.50".
// It is empty for the zero value.
func (a Amount) String() string { return a.value }

// IsSet reports whether the amount was provided (it is not the zero value).
func (a Amount) IsSet() bool { return a.value != "" }

// Valid reports whether the amount is set and is a well-formed, non-negative decimal.
func (a Amount) Valid() bool { return amountPattern.MatchString(a.value) }

// DecimalPlaces returns the number of fractional digits, e.g. 2 for "1000.50".
func (a Amount) DecimalPlaces() int {
	_, fraction, found := strings.Cut(a.value, ".")
	if !found {
		return 0
	}

	return len(fraction)
}

// WholePart returns the digits before the decimal point, e.g. "1000" for "1000.50".
// It is empty for the zero value.
func (a Amount) WholePart() string {
	whole, _, _ := strings.Cut(a.value, ".")
	return whole
}

// Equals reports whether text is the same amount, compared by value: leading zeros of the whole
// part and trailing zeros of the fraction are ignored, so "01000", "1000" and "1000.00" are equal.
// Text that is not plain digits with an optional fraction (e.g. "1,000" or " 1000") is never
// equal, and neither is an unset or invalid amount. Use it to compare a callback's Amount with
// your order.
func (a Amount) Equals(text string) bool {
	if !a.Valid() || !amountPattern.MatchString(text) {
		return false
	}
	return normalizeAmount(a.value) == normalizeAmount(text)
}

func normalizeAmount(value string) string {
	whole, fraction, _ := strings.Cut(value, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	fraction = strings.TrimRight(fraction, "0")
	if fraction == "" {
		return whole
	}
	return whole + "." + fraction
}

// IsZero reports whether the amount is a valid zero, e.g. "0" or "0.00".
func (a Amount) IsZero() bool {
	return a.Valid() && strings.Trim(strings.ReplaceAll(a.value, ".", ""), "0") == ""
}

// IsPositive reports whether the amount is valid and greater than zero.
func (a Amount) IsPositive() bool { return a.Valid() && !a.IsZero() }

// MarshalJSON encodes the amount as a JSON string, e.g. "1000.50", so no JSON consumer has to
// parse it as a float. The zero value encodes as null.
func (a Amount) MarshalJSON() ([]byte, error) {
	if !a.IsSet() {
		return []byte("null"), nil
	}

	return json.Marshal(a.value)
}

// UnmarshalJSON accepts a JSON string ("1000.50") or a JSON number (1000.50). A number is read
// from its literal text, never converted through a float, so 10.5 stays exactly "10.5"; numbers
// with a sign or exponent (-1, 1e5) are rejected. null leaves the amount unset.
func (a *Amount) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*a = Amount{}
		return nil
	}

	text := string(data)
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	}

	parsed, err := ParseAmount(text)
	if err != nil {
		return err
	}
	*a = parsed

	return nil
}
