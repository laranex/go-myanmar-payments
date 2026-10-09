package validate

import (
	"errors"
	"regexp"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

func failures(t *testing.T, err error) map[string]string {
	t.Helper()
	var invalid *myanmarpayments.InvalidPaymentDataError
	if !errors.As(err, &invalid) {
		t.Fatalf("Err returned %T (%v), want *InvalidPaymentDataError", err, err)
	}
	return invalid.Errors
}

func TestErrIsNilWhenEveryCheckPasses(t *testing.T) {
	err := New().
		Required("order_id", "ORDER_1").
		Length("order_id", "ORDER_1", 1, 20).
		Max("note", "short", 10).
		Pattern("order_id", "ORDER_1", regexp.MustCompile(`^[A-Z_0-9]+$`), "letters, numbers and underscores").
		Between("expires", nil, 1, 60).
		Between("expires", intPtr(30), 1, 60).
		Amount("amount", myanmarpayments.Kyat(1000), AmountRule{Gateway: "KBZ Pay", MaxDecimals: 2}).
		URL("callback_url", "https://shop.test/callback").
		When(false, "items", "never").
		Err()

	if err != nil {
		t.Fatalf("Err = %v, want nil", err)
	}
}

func TestStringRules(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9]+$`)
	errs := failures(t, New().
		Required("order_id", "   ").
		Length("short", "ab", 3, 10).
		Length("long", "abcdefghijkl", 3, 10).
		Length("empty_skipped", "", 3, 10).
		Max("note", "0123456789x", 10).
		Pattern("digits", "12a", pattern, "digits").
		Pattern("digits_skipped", "", pattern, "digits").
		Between("expires", intPtr(61), 1, 60).
		Between("zero", intPtr(0), 1, 60).
		When(true, "items", "The items field needs at least one item.").
		Err())

	want := map[string]string{
		"order_id": "The order_id field is required.",
		"short":    "The short field must be between 3 and 10 characters.",
		"long":     "The long field must be between 3 and 10 characters.",
		"note":     "The note field must not be greater than 10 characters.",
		"digits":   "The digits field may only contain digits.",
		"expires":  "The expires field must be between 1 and 60.",
		"zero":     "The zero field must be between 1 and 60.",
		"items":    "The items field needs at least one item.",
	}
	if len(errs) != len(want) {
		t.Fatalf("got %d errors %v, want %d", len(errs), errs, len(want))
	}
	for field, message := range want {
		if errs[field] != message {
			t.Errorf("%s: got %q, want %q", field, errs[field], message)
		}
	}
}

func TestOnlyTheFirstFailurePerFieldIsKept(t *testing.T) {
	errs := failures(t, New().Required("order_id", "").Max("order_id", "x", 0).Err())

	if errs["order_id"] != "The order_id field is required." {
		t.Fatalf("order_id = %q, want the required message", errs["order_id"])
	}
}

func TestURLRules(t *testing.T) {
	errs := failures(t, New().
		URL("relative", "/callback").
		URL("ftp", "ftp://shop.test/callback").
		URL("broken", "http://[::1").
		URL("http_ok", "http://shop.test/callback").
		URL("upper_ok", "HTTPS://shop.test/callback").
		URL("skipped", "").
		Err())

	for _, field := range []string{"relative", "ftp", "broken"} {
		if errs[field] != "The "+field+" field must be a valid http or https URL." {
			t.Errorf("%s = %q, want the valid URL message", field, errs[field])
		}
	}
	for _, field := range []string{"http_ok", "upper_ok", "skipped"} {
		if _, failed := errs[field]; failed {
			t.Errorf("%s should pass, got %q", field, errs[field])
		}
	}
}

func TestAmountFollowsTheGatewayRule(t *testing.T) {
	whole := AmountRule{Gateway: "Wave Money"}
	twoPlaces := AmountRule{Gateway: "KBZ Pay", MaxDecimals: 2}
	cybersource := AmountRule{Gateway: "CyberSource", MaxDecimals: -1, MaxLength: 15, AllowZero: true}

	cases := []struct {
		name   string
		amount myanmarpayments.Amount
		rule   AmountRule
		want   string
	}{
		{"unset", myanmarpayments.Amount{}, whole, "The amount field is required."},
		{"negative", myanmarpayments.Kyat(-1), whole, "The amount field must be a non-negative number such as 1000 or 1000.50."},
		{"decimal where whole required", myanmarpayments.MustParseAmount("10.5"), whole, "Wave Money does not accept decimal amounts; the amount field must be a whole number."},
		{"too many decimals", myanmarpayments.MustParseAmount("10.555"), twoPlaces, "KBZ Pay accepts at most 2 decimal places; the amount field has 3."},
		{"zero", myanmarpayments.Kyat(0), twoPlaces, "The amount field must be greater than 0."},
		{"too long", myanmarpayments.MustParseAmount("1234567890.123456"), cybersource, "The amount field must not be greater than 15 characters."},
		{"whole ok", myanmarpayments.Kyat(1000), whole, ""},
		{"two decimals ok", myanmarpayments.MustParseAmount("1000.50"), twoPlaces, ""},
		{"zero allowed", myanmarpayments.Kyat(0), cybersource, ""},
		{"unlimited decimals", myanmarpayments.MustParseAmount("1.123456"), cybersource, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := New().Amount("amount", tc.amount, tc.rule).Err()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Err = %v, want nil", err)
				}
				return
			}
			if got := failures(t, err)["amount"]; got != tc.want {
				t.Fatalf("amount = %q, want %q", got, tc.want)
			}
		})
	}
}

func intPtr(n int) *int { return &n }
