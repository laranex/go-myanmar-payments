package myanmarpayments

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

// CallbackRequest is an incoming request from a gateway (a server callback or a browser
// return). Signatures are verified against what the gateway actually sent, so build it
// from the real request with NewCallbackRequestFromHTTP.
type CallbackRequest struct {
	// Body is the raw request body exactly as received.
	Body []byte
	// Header holds the request headers.
	Header http.Header
	// Query holds the query string parameters.
	Query url.Values
}

// NewCallbackRequest builds a request from its parts, e.g. to replay a stored callback.
func NewCallbackRequest(body []byte, header http.Header, query url.Values) *CallbackRequest {
	if header == nil {
		header = http.Header{}
	}
	if query == nil {
		query = url.Values{}
	}

	return &CallbackRequest{Body: body, Header: header, Query: query}
}

// MaxCallbackBodyBytes is the largest callback body NewCallbackRequestFromHTTP reads (64 KiB).
// Gateway callbacks are a few kilobytes at most.
const MaxCallbackBodyBytes = 64 << 10

// ErrCallbackBodyTooLarge is returned (wrapped) by NewCallbackRequestFromHTTP when the body is
// larger than MaxCallbackBodyBytes.
var ErrCallbackBodyTooLarge = errors.New("myanmarpayments: The callback body is too large")

// NewCallbackRequestFromHTTP reads the body of r once and keeps its headers and query. The query
// is decoded like a urlencoded body (see ParsedBody): names are kept exactly as sent, the first
// value of a repeated name wins and a bad "%" escape or a ";" never drops a pair. The body of r
// is replaced so it can still be read afterwards. A body larger than MaxCallbackBodyBytes is not
// read past the limit and returns an error wrapping ErrCallbackBodyTooLarge.
func NewCallbackRequestFromHTTP(r *http.Request) (*CallbackRequest, error) {
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(r.Body, MaxCallbackBodyBytes+1))
		if err != nil {
			return nil, fmt.Errorf("myanmarpayments: Could not read the callback body: %w", err)
		}
		if len(body) > MaxCallbackBodyBytes {
			return nil, fmt.Errorf("%w (more than %d bytes)", ErrCallbackBodyTooLarge, MaxCallbackBodyBytes)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	return NewCallbackRequest(body, r.Header.Clone(), values.DecodeForm(r.URL.RawQuery)), nil
}

// NewCallbackRequestFromJSON encodes payload as a JSON body, e.g. to replay a callback stored
// as decoded JSON.
func NewCallbackRequestFromJSON(payload any, header http.Header) (*CallbackRequest, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("myanmarpayments: Could not encode the callback payload: %w", err)
	}
	if header == nil {
		header = http.Header{}
	}
	header.Set("Content-Type", "application/json")

	return NewCallbackRequest(body, header, nil), nil
}

// ParsedBody decodes the body as JSON (numbers kept as json.Number) or as a urlencoded form,
// like the other Laranex SDKs. Only ASCII whitespace and NUL around the body are trimmed, so a
// body that starts with a byte order mark is read as a form. A form is split on "&" and at the
// first "=" of each pair; "+" is a space, a valid %XX escape is decoded and any other "%" is
// kept as it is; ";" is not a separator, names are kept exactly as sent and the first value of
// a repeated name wins.
func (r *CallbackRequest) ParsedBody() map[string]any {
	body := bytes.Trim(r.Body, values.ASCIISpace)
	if len(body) == 0 {
		return map[string]any{}
	}

	if decoded, ok := values.DecodeObject(body); ok {
		return decoded
	}

	return valuesToMap(values.DecodeForm(string(body)))
}

// Input returns the parsed body merged over the query string.
func (r *CallbackRequest) Input() map[string]any {
	input := valuesToMap(r.Query)
	for key, value := range r.ParsedBody() {
		input[key] = value
	}

	return input
}

// QueryInput returns the query string merged over the parsed body.
func (r *CallbackRequest) QueryInput() map[string]any {
	input := r.ParsedBody()
	for key, value := range valuesToMap(r.Query) {
		input[key] = value
	}

	return input
}

func valuesToMap(values url.Values) map[string]any {
	result := make(map[string]any, len(values))
	for key, list := range values {
		if len(list) > 0 {
			result[key] = list[0]
		}
	}

	return result
}

// Acknowledgement is the HTTP response a gateway expects after it delivers a callback.
// Gateways retry until they receive it.
type Acknowledgement struct {
	Status  int
	Body    string
	Headers map[string]string
}

// DefaultAcknowledgement is an empty 200 plain-text response.
func DefaultAcknowledgement() Acknowledgement {
	return Acknowledgement{Status: http.StatusOK, Headers: map[string]string{"Content-Type": "text/plain"}}
}

// Write sends the acknowledgement on w.
func (a Acknowledgement) Write(w http.ResponseWriter) error {
	for name, value := range a.Headers {
		w.Header().Set(name, value)
	}
	status := a.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, err := io.WriteString(w, a.Body)

	return err
}

// PaymentCallback is a gateway notification whose signature has been verified.
//
// Always compare Amount with your order before fulfilling it, and handle duplicates:
// gateways retry.
type PaymentCallback struct {
	// OrderID is your order id.
	OrderID string
	// Status is the status mapped onto this package's statuses.
	Status PaymentStatus
	// GatewayStatus is the gateway's own status value, unmapped.
	GatewayStatus string
	// GatewayReference is the gateway's id for the payment.
	GatewayReference string
	// Amount is the amount the gateway reports, as it sent it.
	Amount string
	// Raw is the verified payload.
	Raw map[string]any
	// Acknowledgement is the response to send so the gateway stops retrying.
	Acknowledgement Acknowledgement
}

// IsSuccessful reports whether the customer paid.
func (c PaymentCallback) IsSuccessful() bool { return c.Status == StatusSuccessful }

// PaymentStatusResult is the state of a payment as reported by a gateway's status API.
type PaymentStatusResult struct {
	// OrderID is your order id, when the gateway returns it.
	OrderID string
	// Status is the status mapped onto this package's statuses.
	Status PaymentStatus
	// GatewayStatus is the gateway's own status value, unmapped.
	GatewayStatus string
	// GatewayReference is the gateway's id for the payment.
	GatewayReference string
	// Amount is the amount the gateway reports, as it sent it.
	Amount string
	// Raw is the gateway's response, for logging.
	Raw map[string]any
}

// IsSuccessful reports whether the customer paid.
func (r PaymentStatusResult) IsSuccessful() bool { return r.Status == StatusSuccessful }

// HeaderValue returns the named header, case-insensitively. A repeated header's values are
// joined with ", ", like the other Laranex SDKs, so a header sent twice never matches a single
// expected value.
func (r *CallbackRequest) HeaderValue(name string) string {
	var joined []string
	for key, list := range r.Header {
		if strings.EqualFold(key, name) {
			joined = append(joined, list...)
		}
	}

	return strings.Join(joined, ", ")
}
