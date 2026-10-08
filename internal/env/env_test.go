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

func TestProductionReadsTheSandboxFlag(t *testing.T) {
	cases := map[string]bool{
		"":        false,
		"  ":      false,
		"true":    false,
		"TRUE":    false,
		"1":       false,
		"yes":     false,
		"garbage": false,
		"false":   true,
		"FALSE":   true,
		"0":       true,
		"no":      true,
		"NO":      true,
		"off":     true,
		" off ":   true,
	}

	for value, want := range cases {
		got := Production(getter(map[string]string{"GATEWAY_SANDBOX": value}), "GATEWAY_SANDBOX")
		if got != want {
			t.Errorf("Production(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestIntFallsBackToTheDefault(t *testing.T) {
	get := getter(map[string]string{"TIMEOUT": " 30 ", "BAD": "thirty", "NEG": "-5"})

	if got := Int(get, "TIMEOUT", 10); got != 30 {
		t.Fatalf("Int(TIMEOUT) = %d, want 30", got)
	}
	if got := Int(get, "NEG", 10); got != -5 {
		t.Fatalf("Int(NEG) = %d, want -5", got)
	}
	if got := Int(get, "BAD", 10); got != 10 {
		t.Fatalf("Int(BAD) = %d, want default 10", got)
	}
	if got := Int(get, "MISSING", 7); got != 7 {
		t.Fatalf("Int(MISSING) = %d, want default 7", got)
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
