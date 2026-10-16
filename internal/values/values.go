// Package values converts decoded JSON and form values to strings.
package values

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
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

// Trimmed returns m[key] as a string with surrounding ASCII whitespace and NUL removed.
func Trimmed(m map[string]any, key string) string {
	return TrimASCII(Get(m, key))
}

// ASCIISpace is the set PHP's trim() removes: space, tab, line feed, carriage return, NUL and
// vertical tab. Every Laranex SDK trims with it, so a no-break space or a byte order mark is a
// character, not whitespace.
const ASCIISpace = " \t\n\r\x00\x0b"

// TrimASCII removes ASCIISpace from both ends of s.
func TrimASCII(s string) string { return strings.Trim(s, ASCIISpace) }

// IsBlank reports whether s holds nothing but ASCIISpace.
func IsBlank(s string) bool { return TrimASCII(s) == "" }

// DecodeForm decodes a urlencoded body or query string like PHP's Form::decode: pairs are split
// on "&" (empty ones skipped) and at their first "=", "+" is a space, a valid %XX escape is
// decoded and any other "%" is kept as it is. ";" is not a separator. Names are kept exactly as
// sent and values are listed in order, so the first value of a repeated name comes first.
func DecodeForm(text string) url.Values {
	form := url.Values{}
	for _, pair := range strings.Split(text, "&") {
		if pair == "" {
			continue
		}
		name, value, _ := strings.Cut(pair, "=")
		name = unescape(name)
		form[name] = append(form[name], unescape(value))
	}
	return form
}

// unescape decodes like PHP's urldecode: "+" is a space, %XX is the byte it names, and a "%"
// without two hex digits after it stays as it is.
func unescape(text string) string {
	if !strings.ContainsAny(text, "+%") {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); i++ {
		switch char := text[i]; {
		case char == '+':
			out.WriteByte(' ')
		case char == '%' && i+2 < len(text) && isHex(text[i+1]) && isHex(text[i+2]):
			out.WriteByte(fromHex(text[i+1])<<4 | fromHex(text[i+2]))
			i += 2
		default:
			out.WriteByte(char)
		}
	}
	return out.String()
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func fromHex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	default:
		return c - '0'
	}
}

// Map returns m[key] as a map, or nil.
func Map(m map[string]any, key string) map[string]any {
	value, _ := m[key].(map[string]any)
	return value
}
