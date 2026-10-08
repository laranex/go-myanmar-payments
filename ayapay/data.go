package ayapay

import (
	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/validate"
)

// Method is how the customer pays through the chosen channel. Services lists the methods
// each channel supports.
type Method string

const (
	// MethodWeb pays on a hosted web page (cards, web checkout).
	MethodWeb Method = "WEB"
	// MethodQR scans a QR with the wallet app.
	MethodQR Method = "QR"
	// MethodNoti approves a push notification in the wallet app.
	MethodNoti Method = "NOTI"
)

func (m Method) valid() bool { return m == MethodWeb || m == MethodQR || m == MethodNoti }

// PaymentData is an AYA Payment Gateway order. AYA only accepts MMK (currency code 104) and
// documents no decimals, so amounts are whole kyat.
type PaymentData struct {
	// OrderID is your unique order id (merchOrderId), 6 to 40 characters.
	OrderID string
	// Amount is the amount in whole kyat, e.g. myanmarpayments.Kyat(1000). AYA documents no decimals.
	Amount myanmarpayments.Amount
	// Channel is the channel key from Services, e.g. aya_pay, kbz_pay, visa.
	Channel string
	// Method is how the customer pays through that channel.
	Method Method
	// ReturnURL is where AYA sends the customer afterwards (optional; defaults to the URL registered with AYA).
	ReturnURL string
	// Description is shown to the customer (optional).
	Description string
	// UserRefs are up to five of your own reference values, echoed back in the callback.
	UserRefs []string
}

// Validate checks the order against AYA's documented rules.
func (d PaymentData) Validate() error {
	return validate.New().
		Required("orderId", d.OrderID).
		Length("orderId", d.OrderID, 6, 40).
		Amount("amount", d.Amount, validate.AmountRule{Gateway: "AYA Payment Gateway"}).
		Required("channel", d.Channel).
		When(!d.Method.valid(), "method", "The method field must be one of WEB, QR or NOTI.").
		URL("returnUrl", d.ReturnURL).
		When(len(d.UserRefs) > 5, "userRefs", "The userRefs field must not have more than 5 items.").
		Err()
}

// Service is a payment channel enabled for your merchant account.
type Service struct {
	// Name is the display name, e.g. "AYA Pay".
	Name string
	// Key is the Channel value to send when initiating, e.g. aya_pay or visa.
	Key string
	// ImageURL is the channel's logo.
	ImageURL string
	// Methods are the methods this channel supports.
	Methods []Method
	// UnknownMethods are methods the gateway listed that this package does not know yet.
	UnknownMethods []string
}

// Supports reports whether the channel supports method.
func (s Service) Supports(method Method) bool {
	for _, m := range s.Methods {
		if m == method {
			return true
		}
	}
	return false
}
