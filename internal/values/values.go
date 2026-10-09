// Package values converts decoded JSON and form values to strings.
package values

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

// DecodeObject decodes data as one JSON object, keeping numbers as json.Number. It reports
// false for anything else: invalid JSON, trailing data, or a JSON value that is not an object.
func DecodeObject(data []byte) (map[string]any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil || decoded == nil {
		return nil, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	return decoded, true
}

// IsNested reports whether v is a JSON object or array, which gateways never sign.
func IsNested(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

// String returns v as a string. Maps, slices and nil report ok=false.
func String(v any) (string, bool) {
	switch value := v.(type) {
	case string:
		return value, true
	case json.Number:
		return value.String(), true
	case bool:
		if value {
			return "true", true
		}
		return "false", true
	case int:
		return strconv.Itoa(value), true
	case int64:
		return strconv.FormatInt(value, 10), true
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), true
	default:
		return "", false
	}
}

// Get returns m[key] as a trimmed-free string, or "" when missing or not scalar.
func Get(m map[string]any, key string) string {
	s, _ := String(m[key])
	return s
}

// Trimmed returns m[key] as a string with surrounding whitespace removed.
func Trimmed(m map[string]any, key string) string {
	return strings.TrimSpace(Get(m, key))
}

// Map returns m[key] as a map, or nil.
func Map(m map[string]any, key string) map[string]any {
	value, _ := m[key].(map[string]any)
	return value
}
