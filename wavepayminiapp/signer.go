package wavepayminiapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// Signer implements Wave's mini app hash (Wave Money JS SDK spec v1.8): lowercase hex
// HMAC-SHA256, keyed with the secret key, over amount, orderID, miniAppID, merchantRefID and
// callBackUrl joined with no separator. The payment and its callback use the same hash.
type Signer struct {
	secretKey string
}

// NewSigner returns a Signer for secretKey.
func NewSigner(secretKey string) Signer { return Signer{secretKey: secretKey} }

// Hash returns the hash; amount is the amount's exact whole-number text, e.g. "5000".
func (s Signer) Hash(amount, orderID, miniAppID, merchantReferenceID, callbackURL string) string {
	mac := hmac.New(sha256.New, []byte(s.secretKey))
	mac.Write([]byte(amount + orderID + miniAppID + merchantReferenceID + callbackURL))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether hashValue is the expected hash, compared in constant time and ignoring
// the case of the hex digits.
func (s Signer) Verify(hashValue, amount, orderID, miniAppID, merchantReferenceID, callbackURL string) bool {
	expected := s.Hash(amount, orderID, miniAppID, merchantReferenceID, callbackURL)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(strings.ToLower(hashValue))) == 1
}
