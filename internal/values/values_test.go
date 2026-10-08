package values

import (
	"encoding/json"
	"testing"
)

func TestStringConvertsScalars(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
		ok    bool
	}{
		{"string", "ORDER_1", "ORDER_1", true},
		{"json number", json.Number("1000.50"), "1000.50", true},
		{"true", true, "true", true},
		{"false", false, "false", true},
		{"int", 42, "42", true},
		{"int64", int64(9007199254740993), "9007199254740993", true},
		{"float64", 1000.5, "1000.5", true},
		{"float64 whole", 1000.0, "1000", true},
		{"nil", nil, "", false},
		{"map", map[string]any{"a": 1}, "", false},
		{"slice", []any{"a"}, "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := String(tc.value)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("String(%v) = (%q, %v), want (%q, %v)", tc.value, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestGetTrimmedAndMapReadDecodedPayloads(t *testing.T) {
	payload := map[string]any{
		"order_id": "  ORDER_1  ",
		"amount":   json.Number("1000"),
		"nested":   map[string]any{"status": "SUCCESS"},
		"list":     []any{"a"},
	}

	if got := Get(payload, "order_id"); got != "  ORDER_1  " {
		t.Fatalf("Get keeps whitespace: got %q", got)
	}
	if got := Trimmed(payload, "order_id"); got != "ORDER_1" {
		t.Fatalf("Trimmed = %q, want ORDER_1", got)
	}
	if got := Get(payload, "amount"); got != "1000" {
		t.Fatalf("Get(amount) = %q, want 1000", got)
	}
	if got := Get(payload, "missing"); got != "" {
		t.Fatalf("Get(missing) = %q, want empty", got)
	}
	if got := Get(payload, "nested"); got != "" {
		t.Fatalf("Get on a map = %q, want empty", got)
	}

	nested := Map(payload, "nested")
	if nested == nil || nested["status"] != "SUCCESS" {
		t.Fatalf("Map(nested) = %v, want the nested object", nested)
	}
	if got := Map(payload, "list"); got != nil {
		t.Fatalf("Map on a slice = %v, want nil", got)
	}
	if got := Map(payload, "missing"); got != nil {
		t.Fatalf("Map(missing) = %v, want nil", got)
	}
}
