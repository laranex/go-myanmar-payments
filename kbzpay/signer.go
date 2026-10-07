package kbzpay

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/laranex/go-myanmar-payments/internal/values"
)

// Signer implements KBZ Pay's signature: every non-empty scalar field except sign and
// sign_type, sorted by key, joined as raw key=value pairs (not URL-encoded), with
// "&key=<app key>" appended, hashed with SHA256 and uppercased.
type Signer struct {
	appKey string
}

// NewSigner returns a Signer for appKey.
func NewSigner(appKey string) Signer { return Signer{appKey: appKey} }

// SignString returns the sorted key=value string, without the app key.
func (s Signer) SignString(fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	pairs := map[string]string{}
	for key, value := range fields {
		if key == "sign" || key == "sign_type" {
			continue
		}
		str, ok := values.String(value)
		if !ok || str == "" {
			continue
		}
		keys = append(keys, key)
		pairs[key] = str
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+pairs[key])
	}
	return strings.Join(parts, "&")
}

// Sign returns the uppercase SHA256 signature of fields.
func (s Signer) Sign(fields map[string]any) string {
	sum := sha256.Sum256([]byte(s.SignString(fields) + "&key=" + s.appKey))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// Verify checks fields["sign"] in constant time.
func (s Signer) Verify(fields map[string]any) bool {
	sign, ok := fields["sign"].(string)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(s.Sign(fields)), []byte(strings.ToUpper(sign))) == 1
}

func stringFields(fields map[string]string) map[string]any {
	result := make(map[string]any, len(fields))
	for key, value := range fields {
		result[key] = value
	}
	return result
}
