package wavemoney

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/validate"
)

// Item is a line item shown on Wave's payment page.
type Item struct {
	// Name is the item name.
	Name string `json:"name"`
	// Amount is the item amount in whole kyat, e.g. myanmarpayments.Kyat(1000).
	Amount myanmarpayments.Amount `json:"amount"`
}

// PaymentData is a Wave Money payment request. Wave only accepts whole kyat (MMK).
type PaymentData struct {
	// OrderID is your order id: letters, numbers, underscores and dashes. One order can have
	// several payment attempts.
	OrderID string
	// CallbackURL is the URL that Wave posts the result to (backend_result_url).
	CallbackURL string
	// ReturnURL is where Wave sends the customer back (frontend_result_url). Not proof of payment.
	ReturnURL string
	// Description is shown to the customer.
	Description string
	// Items are the line items shown on Wave's page.
	Items []Item
	// Amount is the total in whole kyat. Leave it unset to charge the sum of the items.
	Amount myanmarpayments.Amount
	// MerchantReferenceID is the unique id of this attempt: letters, numbers, underscores and
	// dashes. Wave rejects a reused one.
	// Initiate fills it with a random id when empty; store it, because Wave's callback may
	// omit OrderID but always carries this.
	MerchantReferenceID string
	// MerchantName is the business name shown on Wave's page for this payment. Leave it empty to
	// use Config.MerchantName; a name made only of whitespace is rejected.
	MerchantName string
	// TimeToLiveSeconds is how long the customer has to pay this payment, a whole number greater
	// than 0 (optional; nil means Config.TimeToLiveSeconds, and one of the two is required). 0 and
	// negative values are rejected.
	TimeToLiveSeconds *int
}

// ResolvedAmount returns Amount, or the sum of the items when Amount is unset. Items are summed
// with exact integer arithmetic; when an item amount is missing, malformed or has decimals the
// sum is left unset and Validate reports the item.
func (d PaymentData) ResolvedAmount() myanmarpayments.Amount {
	if d.Amount.IsSet() {
		return d.Amount
	}
	if len(d.Items) == 0 {
		return myanmarpayments.Amount{}
	}

	total := new(big.Int)
	for _, item := range d.Items {
		if !item.Amount.Valid() || item.Amount.DecimalPlaces() > 0 {
			return myanmarpayments.Amount{}
		}
		value, ok := new(big.Int).SetString(item.Amount.String(), 10)
		if !ok {
			return myanmarpayments.Amount{}
		}
		total.Add(total, value)
	}

	return myanmarpayments.MustParseAmount(total.String())
}

// Validate checks the request against Wave's documented rules.
func (d PaymentData) Validate() error {
	v := validate.New().
		Required("orderId", d.OrderID).
		Pattern("orderId", d.OrderID, idPattern, idDescription).
		Pattern("merchantReferenceId", d.MerchantReferenceID, idPattern, idDescription).
		Required("callbackUrl", d.CallbackURL).
		URL("callbackUrl", d.CallbackURL)
	v.Required("returnUrl", d.ReturnURL).
		URL("returnUrl", d.ReturnURL).
		Required("description", d.Description).
		When(len(d.Items) == 0, "items", "The items field must have at least one item.").
		When(d.MerchantName != "" && strings.TrimSpace(d.MerchantName) == "", "merchantName", "The merchantName field is required.").
		When(d.TimeToLiveSeconds != nil && *d.TimeToLiveSeconds < 1, "timeToLiveSeconds", "The timeToLiveSeconds field must be greater than 0.")
	for i, item := range d.Items {
		v.Required(fmt.Sprintf("items.%d.name", i), item.Name).
			Amount(fmt.Sprintf("items.%d.amount", i), item.Amount, validate.AmountRule{Gateway: "Wave Money"})
	}
	if len(d.Items) > 0 && !d.Amount.IsSet() && !d.ResolvedAmount().IsSet() {
		return v.Err()
	}

	return v.Amount("amount", d.ResolvedAmount(), validate.AmountRule{Gateway: "Wave Money"}).Err()
}

func randomReference() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
