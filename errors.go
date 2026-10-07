package myanmarpayments

import (
	"fmt"
	"sort"
	"strings"
)

// InvalidPaymentDataError is returned when payment data has values the gateway would reject.
type InvalidPaymentDataError struct {
	// Errors maps a field name to its error message.
	Errors map[string]string
}

func (e *InvalidPaymentDataError) Error() string {
	fields := make([]string, 0, len(e.Errors))
	for field := range e.Errors {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	messages := make([]string, 0, len(fields))
	for _, field := range fields {
		messages = append(messages, e.Errors[field])
	}

	return "myanmarpayments: invalid payment data: " + strings.Join(messages, " ")
}

// APIError is returned when a gateway rejects a request or answers with an error,
// including errors sent with HTTP 200.
type APIError struct {
	// Message describes what failed.
	Message string
	// GatewayCode is the gateway's own error code, e.g. ORDER_ID_USED or 09.
	GatewayCode string
	// GatewayMessage is the gateway's own error message.
	GatewayMessage string
	// HTTPStatus is the HTTP status of the response, or 0 when no response was received.
	HTTPStatus int
	// Raw is the decoded response body.
	Raw map[string]any
	// Err is the underlying transport error, if any.
	Err error
}

func (e *APIError) Error() string { return "myanmarpayments: " + e.Message }

// Unwrap returns the underlying transport error.
func (e *APIError) Unwrap() error { return e.Err }

// SignatureVerificationError is returned when a callback or gateway response fails
// signature verification. Never trust its payload.
type SignatureVerificationError struct {
	Message string
	// Raw is the unverified payload, for logging only.
	Raw map[string]any
}

func (e *SignatureVerificationError) Error() string { return "myanmarpayments: " + e.Message }

// ConfigurationError is returned when a gateway is missing a credential or setting it needs.
type ConfigurationError struct {
	Gateway string
	Key     string
}

func (e *ConfigurationError) Error() string {
	return fmt.Sprintf("myanmarpayments: the %s configuration is missing [%s]", e.Gateway, e.Key)
}
