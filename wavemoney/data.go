package wavemoney

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"

	"github.com/laranex/go-myanmar-payments/internal/validate"
)

// Item is a line item shown on Wave's payment page.
type Item struct {
	// Name is the item name.
	Name string `json:"name"`
	// Amount is the item amount in whole kyat.
	Amount int64 `json:"amount"`
}

// PaymentData is a Wave Money payment request. Wave only accepts whole kyat (MMK).
type PaymentData struct {
	// OrderID is your order id. One order can have several payment attempts.
	OrderID string
	// CallbackURL is the HTTPS URL on port 443 that Wave posts the result to (backend_result_url).
	CallbackURL string
	// ReturnURL is where Wave sends the customer back (frontend_result_url). Not proof of payment.
	ReturnURL string
	// Description is shown to the customer.
	Description string
	// Items are the line items shown on Wave's page.
	Items []Item
	// Amount is the total in whole kyat; 0 means the sum of the items.
	Amount int64
	// MerchantReferenceID is the unique id of this attempt. Wave rejects a reused one.
	// Initiate fills it with a random id when empty; store it, because Wave's callback may
	// omit OrderID but always carries this.
	MerchantReferenceID string
}

// ResolvedAmount returns Amount, or the sum of the items when Amount is 0.
func (d PaymentData) ResolvedAmount() int64 {
	if d.Amount != 0 {
		return d.Amount
	}
	var total int64
	for _, item := range d.Items {
		total += item.Amount
	}
	return total
}

// Validate checks the request against Wave's documented rules.
func (d PaymentData) Validate() error {
	v := validate.New().
		Required("orderId", d.OrderID).
		Required("callbackUrl", d.CallbackURL).
		URL("callbackUrl", d.CallbackURL).
		HTTPS("callbackUrl", d.CallbackURL)
	if parsed, err := url.Parse(d.CallbackURL); err == nil && parsed.Port() != "" && parsed.Port() != "443" {
		v.When(true, "callbackUrl", "The callbackUrl field must use the standard HTTPS port 443.")
	}
	v.Required("returnUrl", d.ReturnURL).
		URL("returnUrl", d.ReturnURL).
		Required("description", d.Description).
		When(len(d.Items) == 0, "items", "The items field must have at least one item.")
	for i, item := range d.Items {
		v.Required(fmt.Sprintf("items.%d.name", i), item.Name).Positive(fmt.Sprintf("items.%d.amount", i), item.Amount)
	}
	return v.Positive("amount", d.ResolvedAmount()).Err()
}

func randomReference() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
