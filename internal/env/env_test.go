package env

import (
	"errors"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

func getter(values map[string]string) Getter {
	return func(key string) string { return values[key] }
}

func TestFirstReturnsTheFirstNonEmptyTrimmedValue(t *testing.T) {
	get := getter(map[string]string{"A": "  ", "B": " second ", "C": "third"})

	if got := First(get, "A", "B", "C"); got != "second" {
		t.Fatalf("First = %q, want %q", got, "second")
	}
	if got := First(get, "A", "MISSING"); got != "" {
		t.Fatalf("First with only blank values = %q, want empty", got)
	}
	if got := First(get); got != "" {
		t.Fatalf("First without keys = %q, want empty", got)
	}
}

func TestSecondsReadsAWholeNumberGreaterThanZero(t *testing.T) {
	cases := map[string]int{
		"":                        0,
		"  ":                      0,
		"30":                      30,
		" 30 ":                    30,
		"+30":                     30,
		"0":                       -1,
		"-5":                      -1,
		"five":                    -1,
		"1.5":                     -1,
		"99999999999999999999999": -1,
	}

	for value, want := range cases {
		if got := Seconds(getter(map[string]string{"TIMEOUT": value}), "TIMEOUT"); got != want {
			t.Errorf("Seconds(%q) = %d, want %d", value, got, want)
		}
	}
}

func TestRequireSecondsReportsMissingAndInvalidValues(t *testing.T) {
	if err := RequireSeconds("wave_money", "time_to_live_in_seconds", 300); err != nil {
		t.Fatalf("RequireSeconds(300) returned %v", err)
	}

	var configErr *myanmarpayments.ConfigurationError
	if !errors.As(RequireSeconds("wave_money", "time_to_live_in_seconds", 0), &configErr) || configErr.Invalid || configErr.Key != "time_to_live_in_seconds" {
		t.Fatalf("RequireSeconds(0) = %+v, want a missing time_to_live_in_seconds", configErr)
	}
	if !errors.As(RequireSeconds("wave_money", "time_to_live_in_seconds", -1), &configErr) || !configErr.Invalid {
		t.Fatalf("RequireSeconds(-1) = %+v, want an invalid time_to_live_in_seconds", configErr)
	}
}

func TestOptionalSecondsReportsOnlyInvalidValues(t *testing.T) {
	for _, seconds := range []int{0, 30} {
		if err := OptionalSeconds("kbz_pay", "timeout_in_seconds", seconds); err != nil {
			t.Fatalf("OptionalSeconds(%d) returned %v", seconds, err)
		}
	}

	var configErr *myanmarpayments.ConfigurationError
	if !errors.As(OptionalSeconds("kbz_pay", "timeout_in_seconds", -1), &configErr) || !configErr.Invalid || configErr.Key != "timeout_in_seconds" {
		t.Fatalf("OptionalSeconds(-1) = %+v, want an invalid timeout_in_seconds", configErr)
	}
}

func TestTimeoutDefaultsWhenUnsetOrBlank(t *testing.T) {
	cases := map[string]int{"": myanmarpayments.DefaultTimeoutSeconds, "   ": 30, "45": 45, "0": -1, "thirty": -1}
	for value, want := range cases {
		if got := Timeout(getter(map[string]string{TimeoutKey: value})); got != want {
			t.Errorf("Timeout(%q) = %d, want %d", value, got, want)
		}
	}
	if got := Timeout(getter(map[string]string{})); got != 30 {
		t.Errorf("Timeout(unset) = %d, want 30", got)
	}
	if got := TimeoutSeconds(15); got != 15 {
		t.Errorf("TimeoutSeconds(15) = %d, want 15", got)
	}
}

func TestRequireReportsTheFirstBlankKey(t *testing.T) {
	if err := Require("KBZ Pay", "app_id", "kp123", "app_key", "secret"); err != nil {
		t.Fatalf("Require with every value set returned %v", err)
	}
	if err := Require("KBZ Pay"); err != nil {
		t.Fatalf("Require without pairs returned %v", err)
	}

	err := Require("KBZ Pay", "app_id", "kp123", "app_key", "  ", "merchant_code", "")
	var configErr *myanmarpayments.ConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("Require returned %T, want *ConfigurationError", err)
	}
	if configErr.Gateway != "KBZ Pay" || configErr.Key != "app_key" {
		t.Fatalf("ConfigurationError = %+v, want gateway KBZ Pay and key app_key", *configErr)
	}
}

func TestRequireIgnoresADanglingKey(t *testing.T) {
	if err := Require("Wave Money", "merchant_id", "m1", "dangling"); err != nil {
		t.Fatalf("Require with an odd number of arguments returned %v", err)
	}
}
