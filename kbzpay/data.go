package kbzpay

import (
	"net/url"
	"regexp"
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/validate"
)

var orderIDPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// PaymentData is a KBZ Pay order, used for the PWA, QR and in-app flows alike.
// KBZ Pay only accepts MMK.
type PaymentData struct {
	// OrderID is your unique order id (merch_order_id): letters, digits and "_" only, at most 40.
	OrderID string
	// Amount is the amount in kyat, greater than 0 with at most 2 decimal places, e.g.
	// myanmarpayments.Kyat(1000) or myanmarpayments.MustParseAmount("1000.50"). KBZ Pay only accepts MMK.
	Amount myanmarpayments.Amount
	// CallbackURL is the public URL KBZ posts the result to (notify_url): at most 512 characters, no query string.
	CallbackURL string
	// Title is the product name shown to the customer (optional).
	Title string
	// TimeToLiveSeconds is how long the customer has to pay before the order expires, 60 to 7200
	// seconds in whole minutes, a multiple of 60 (optional; nil uses Config.TimeToLiveSeconds, and KBZ's default
	// of 7200 when that is 0 too). KBZ counts minutes, so 1800 is sent as 30m.
	TimeToLiveSeconds *int
	// CallbackInfo is free text KBZ sends back unchanged in the callback (optional).
	CallbackInfo string
}

// Validate checks the order against KBZ's documented limits.
func (d PaymentData) Validate() error {
	return validate.New().
		Required("orderId", d.OrderID).
		Max("orderId", d.OrderID, 40).
		Pattern("orderId", d.OrderID, orderIDPattern, "letters, numbers and underscores").
		Amount("amount", d.Amount, validate.AmountRule{Gateway: "KBZ Pay", MaxDecimals: 2}).
		Required("callbackUrl", d.CallbackURL).
		URL("callbackUrl", d.CallbackURL).
		Max("callbackUrl", d.CallbackURL, 512).
		When(strings.Contains(d.CallbackURL, "?"), "callbackUrl", "The callbackUrl field must not contain a query string.").
		Between("timeToLiveSeconds", d.TimeToLiveSeconds, 60, 7200).
		When(d.TimeToLiveSeconds != nil && *d.TimeToLiveSeconds%60 != 0, "timeToLiveSeconds", "The timeToLiveSeconds field must be a multiple of 60.").
		Max("callbackInfo", url.QueryEscape(d.CallbackInfo), 512).
		Err()
}
