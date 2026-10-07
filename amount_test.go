package myanmarpayments

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseAmountAcceptsPlainDecimals(t *testing.T) {
	cases := map[string]struct {
		want   string
		places int
		zero   bool
	}{
		"1000":    {"1000", 0, false},
		"1000.50": {"1000.50", 2, false},
		"0.5":     {"0.5", 1, false},
		"0":       {"0", 0, true},
		"0.00":    {"0.00", 2, true},
		"007":     {"7", 0, false},
		"00.10":   {"0.10", 2, false},
		"10.505":  {"10.505", 3, false},
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			amount, err := ParseAmount(input)
			if err != nil {
				t.Fatalf("ParseAmount(%q) error = %v", input, err)
			}
			if amount.String() != want.want || amount.DecimalPlaces() != want.places || amount.IsZero() != want.zero || amount.IsPositive() == want.zero {
				t.Fatalf("ParseAmount(%q) = %q (places %d, zero %v), want %q (places %d, zero %v)",
					input, amount, amount.DecimalPlaces(), amount.IsZero(), want.want, want.places, want.zero)
			}
			if !amount.IsSet() || !amount.Valid() {
				t.Fatalf("ParseAmount(%q) should be set and valid", input)
			}
		})
	}
}

func TestParseAmountRejectsAnythingButPlainDecimals(t *testing.T) {
	for _, input := range []string{"", "1e5", "-1", "+1", "1,000", " 10", "10 ", "10.", ".5", "1.2.3", "NaN", "Inf", "0x10", "١٠٠"} {
		t.Run(input, func(t *testing.T) {
			_, err := ParseAmount(input)
			var invalid *InvalidPaymentDataError
			if !errors.As(err, &invalid) || invalid.Errors["amount"] == "" {
				t.Fatalf("ParseAmount(%q) error = %v, want *InvalidPaymentDataError for amount", input, err)
			}
		})
	}
}

func TestMustParseAmountPanicsOnBadInput(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustParseAmount did not panic")
		}
	}()
	MustParseAmount("1,000")
}

func TestKyat(t *testing.T) {
	if got := Kyat(1000); got.String() != "1000" || !got.IsPositive() || got.DecimalPlaces() != 0 {
		t.Fatalf("Kyat(1000) = %q", got)
	}
	if got := Kyat(0); !got.IsZero() || got.IsPositive() {
		t.Fatalf("Kyat(0) = %q, want a valid zero", got)
	}
	if got := Kyat(-5); !got.IsSet() || got.Valid() || got.IsPositive() || got.IsZero() {
		t.Fatalf("Kyat(-5) should be set but invalid, got valid=%v", got.Valid())
	}
}

func TestZeroValueAmountIsUnset(t *testing.T) {
	var amount Amount
	if amount.IsSet() || amount.Valid() || amount.IsZero() || amount.IsPositive() || amount.String() != "" {
		t.Fatal("the zero Amount must be unset and invalid")
	}
}

func TestAmountJSON(t *testing.T) {
	encoded, err := json.Marshal(struct {
		Total Amount  `json:"total"`
		Empty Amount  `json:"empty"`
		Ptr   *Amount `json:"ptr,omitempty"`
	}{Total: MustParseAmount("1000.50")})
	if err != nil || string(encoded) != `{"total":"1000.50","empty":null}` {
		t.Fatalf("Marshal = %s, %v", encoded, err)
	}

	cases := map[string]struct {
		input string
		want  string
		ok    bool
	}{
		"string":          {`"1000.50"`, "1000.50", true},
		"integer number":  {`1000`, "1000", true},
		"decimal number":  {`10.5`, "10.5", true},
		"precise number":  {`0.30000000000000004`, "0.30000000000000004", true},
		"null":            {`null`, "", true},
		"negative number": {`-1`, "", false},
		"exponent number": {`1e5`, "", false},
		"bad string":      {`"1,000"`, "", false},
		"boolean":         {`true`, "", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var amount Amount
			err := json.Unmarshal([]byte(c.input), &amount)
			if (err == nil) != c.ok {
				t.Fatalf("Unmarshal(%s) error = %v, want ok=%v", c.input, err, c.ok)
			}
			if c.ok && amount.String() != c.want {
				t.Fatalf("Unmarshal(%s) = %q, want %q", c.input, amount, c.want)
			}
		})
	}

	var roundTrip Amount
	if err := json.Unmarshal(encoded[len(`{"total":`):len(`{"total":"1000.50"`)], &roundTrip); err != nil || roundTrip != MustParseAmount("1000.50") {
		t.Fatalf("round trip = %q, %v", roundTrip, err)
	}
}
