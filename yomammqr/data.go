package yomammqr

import (
	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/validate"
)

// PaymentData is a Yoma MMQR order. Yoma accepts each order number once; renew an expired
// QR with RenewQR. Yoma documents no decimals, so amounts are whole kyat.
type PaymentData struct {
	// OrderID is your unique order number, at most 20 characters.
	OrderID string
	// Amount is the amount in whole kyat, e.g. myanmarpayments.Kyat(1000). Yoma documents no decimals.
	Amount myanmarpayments.Amount
	// Description is shown on the payment slip, at most 50 characters.
	Description string
}

// Validate checks the order against Yoma's documented limits.
func (d PaymentData) Validate() error {
	return validate.New().
		Required("orderId", d.OrderID).
		Max("orderId", d.OrderID, 20).
		Amount("amount", d.Amount, validate.AmountRule{Gateway: "Yoma MMQR"}).
		Required("description", d.Description).
		Max("description", d.Description, 50).
		Err()
}
