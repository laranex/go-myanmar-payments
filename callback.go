package myanmarpayments

import (
	"bytes"
	"encoding/json"
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

// NewCallbackRequestFromHTTP reads the body of r once and keeps its headers and query.
// The body of r is replaced so it can still be read afterwards.
func NewCallbackRequestFromHTTP(r *http.Request) (*CallbackRequest, error) {
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("myanmarpayments: Could not read the callback body: %w", err)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	return NewCallbackRequest(body, r.Header.Clone(), r.URL.Query()), nil
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

// ParsedBody decodes the body as JSON (numbers kept as json.Number) or as a urlencoded form.
func (r *CallbackRequest) ParsedBody() map[string]any {
	body := bytes.TrimSpace(r.Body)
	if len(body) == 0 {
		return map[string]any{}
	}

	if decoded, ok := values.DecodeObject(body); ok {
		return decoded
	}

	// A malformed pair (e.g. a bad percent escape) is skipped; the other pairs are kept.
	form, _ := url.ParseQuery(string(body))

	return valuesToMap(form)
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

// HeaderValue returns the first value of the named header, case-insensitively.
func (r *CallbackRequest) HeaderValue(name string) string {
	for key, values := range r.Header {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}

	return ""
}
