package myanmarpayments

import "strings"

// PaymentStatus is a gateway-independent payment status. Every gateway's own status
// values are mapped onto these.
type PaymentStatus string

const (
	// StatusSuccessful means the customer paid. It is the only status that means money was collected.
	StatusSuccessful PaymentStatus = "successful"
	// StatusPending means the payment is still in progress or waiting on the customer.
	StatusPending PaymentStatus = "pending"
	// StatusFailed means the payment was attempted and failed or was rejected.
	StatusFailed PaymentStatus = "failed"
	// StatusCanceled means the payment or order was canceled or closed before completing.
	StatusCanceled PaymentStatus = "canceled"
	// StatusExpired means the payment window ran out before the customer paid.
	StatusExpired PaymentStatus = "expired"
	// StatusUnknown means the gateway sent a status this package does not recognize.
	StatusUnknown PaymentStatus = "unknown"
)

// IsFinal reports whether the status will not change any more.
func (s PaymentStatus) IsFinal() bool {
	return s != StatusPending && s != StatusUnknown
}

// ResolveStatus maps a gateway status onto a PaymentStatus, trimming whitespace.
// Statuses missing from the map resolve to StatusUnknown.
func ResolveStatus(statuses map[string]PaymentStatus, gatewayStatus string) PaymentStatus {
	if status, ok := statuses[strings.TrimSpace(gatewayStatus)]; ok {
		return status
	}

	return StatusUnknown
}

// PaymentFlow is how the customer completes a payment after it has been initiated.
type PaymentFlow string

const (
	// FlowRedirect sends the customer's browser to a gateway-hosted URL.
	FlowRedirect PaymentFlow = "redirect"
	// FlowForm posts a signed form from the customer's browser to the gateway.
	FlowForm PaymentFlow = "form"
	// FlowQR shows a QR code the customer scans with their wallet app.
	FlowQR PaymentFlow = "qr"
	// FlowApp hands a signed payload to your mobile app, which opens the wallet SDK.
	FlowApp PaymentFlow = "app"
)
