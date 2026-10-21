package wavepayminiapp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/validate"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

// gatewayName is the name used in amount error messages.
const gatewayName = "Wave Pay Mini App"

// Item is a line item of a mini app payment (items in makePayment).
type Item struct {
	// Name is the product name.
	Name string
	// Amount is the product price in whole kyat, e.g. myanmarpayments.Kyat(1000). Wave takes an
	// integer.
	Amount myanmarpayments.Amount
}

// PaymentData is a Wave Pay Mini App payment: the values signed into Wave's makePayment
// payload. Wave only accepts whole kyat and gives no length or character limits.
type PaymentData struct {
	// OrderID is your order id (orderID).
	OrderID string
	// CallbackURL is the URL Wave posts the result to (callBackUrl). It is signed into the hash,
	// so verify the callback with this same URL.
	CallbackURL string
	// Items are the line items, at least one.
	Items []Item
	// Amount is the total in whole kyat. Leave it unset to charge the exact sum of the items.
	Amount myanmarpayments.Amount
	// MerchantReferenceID is your reference for this attempt (merchantRefID). Payment fills it
	// with a random 32-character hex id when empty; store it: the callback's merchantRefId names
	// the attempt that was paid.
	MerchantReferenceID string
}

// ResolvedAmount returns Amount, or the exact sum of the items when Amount is unset. When an item
// amount is missing, malformed or has decimals the sum is left unset and Validate reports the
// item.
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

// Validate checks the data in the same order as the other Laranex SDKs: orderId, callbackUrl,
// items, merchantReferenceId, then each item's name and amount, then amount. Amounts are whole
// kyat greater than 0. Without Amount, no items gives "The amount field is required.", and an
// item amount that is invalid or has decimals skips the total check (the item's own error is
// reported). An empty MerchantReferenceID is valid (Payment generates one); one made only of
// whitespace is not.
func (d PaymentData) Validate() error {
	rule := validate.AmountRule{Gateway: gatewayName}
	v := validate.New().
		Required("orderId", d.OrderID).
		Required("callbackUrl", d.CallbackURL).
		URL("callbackUrl", d.CallbackURL).
		When(len(d.Items) == 0, "items", "The items field must have at least one item.").
		When(d.MerchantReferenceID != "" && values.IsBlank(d.MerchantReferenceID), "merchantReferenceId", "The merchantReferenceId field is required.")
	for i, item := range d.Items {
		v.Required(fmt.Sprintf("items.%d.name", i), item.Name).
			Amount(fmt.Sprintf("items.%d.amount", i), item.Amount, rule)
	}
	if len(d.Items) > 0 && !d.Amount.IsSet() && !d.ResolvedAmount().IsSet() {
		return v.Err()
	}

	return v.Amount("amount", d.ResolvedAmount(), rule).Err()
}

func randomReference() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
