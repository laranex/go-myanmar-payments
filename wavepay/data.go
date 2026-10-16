package wavepay

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

// PaymentData is a Wave Pay payment request. Wave only accepts whole kyat (MMK).
type PaymentData struct {
	// OrderID is your order id, at most 100 characters: letters, numbers, underscores and dashes.
	// One order can have several payment attempts.
	OrderID string
	// CallbackURL is the URL that Wave posts the result to (backend_result_url), at most 255
	// characters.
	CallbackURL string
	// ReturnURL is where Wave sends the customer back (frontend_result_url), at most 255
	// characters. Not proof of payment.
	ReturnURL string
	// Description is shown to the customer (payment_description), at most 255 characters.
	Description string
	// Items are the line items shown on Wave's page.
	Items []Item
	// Amount is the total in whole kyat. Leave it unset to charge the sum of the items.
	Amount myanmarpayments.Amount
	// MerchantReferenceID is the unique id of this attempt, at most 100 characters: letters,
	// numbers, underscores and dashes. Wave rejects a reused one.
	// Initiate fills it with a random id when empty; store it: every callback carries it as
	// merchantReferenceId.
	MerchantReferenceID string
	// MerchantName is the business name shown on Wave's page for this payment. Leave it empty to
	// use Config.MerchantName; a name made only of whitespace is rejected.
	MerchantName string
	// TimeToLiveSeconds is how long the customer has to pay this payment, a whole number greater
	// than 0 (optional; nil means Config.TimeToLiveSeconds). 0 and
	// negative values are rejected.
	TimeToLiveSeconds *int
	// AdditionalField3 is extra information sent as additional_field_3, at most 50 characters;
	// Wave echoes it as the callback's additionalField3. Not sent when empty.
	AdditionalField3 string
	// AdditionalField4 is extra information sent as additional_field_4, at most 50 characters;
	// echoed as additionalField4. Not sent when empty.
	AdditionalField4 string
	// AdditionalField5 is extra information sent as additional_field_5, at most 50 characters;
	// echoed as additionalField5. Not sent when empty.
	AdditionalField5 string
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

// Validate checks the request against Wave's documented rules (Pay with Wave API spec v1.3).
// Lengths are counted in bytes of UTF-8.
func (d PaymentData) Validate() error {
	v := validate.New().
		Required("orderId", d.OrderID).
		Max("orderId", d.OrderID, 100).
		Pattern("orderId", d.OrderID, idPattern, idDescription).
		Max("merchantReferenceId", d.MerchantReferenceID, 100).
		Pattern("merchantReferenceId", d.MerchantReferenceID, idPattern, idDescription).
		Required("callbackUrl", d.CallbackURL).
		URL("callbackUrl", d.CallbackURL).
		Max("callbackUrl", d.CallbackURL, 255)
	v.Required("returnUrl", d.ReturnURL).
		URL("returnUrl", d.ReturnURL).
		Max("returnUrl", d.ReturnURL, 255).
		Required("description", d.Description).
		Max("description", d.Description, 255).
		When(len(d.Items) == 0, "items", "The items field must have at least one item.").
		When(d.MerchantName != "" && strings.TrimSpace(d.MerchantName) == "", "merchantName", "The merchantName field is required.").
		When(d.TimeToLiveSeconds != nil && *d.TimeToLiveSeconds < 1, "timeToLiveSeconds", "The timeToLiveSeconds field must be greater than 0.").
		Max("additionalField3", d.AdditionalField3, 50).
		Max("additionalField4", d.AdditionalField4, 50).
		Max("additionalField5", d.AdditionalField5, 50)
	for i, item := range d.Items {
		v.Required(fmt.Sprintf("items.%d.name", i), item.Name).
			Amount(fmt.Sprintf("items.%d.amount", i), item.Amount, validate.AmountRule{Gateway: "Wave Pay"})
	}
	if len(d.Items) > 0 && !d.Amount.IsSet() && !d.ResolvedAmount().IsSet() {
		return v.Err()
	}

	return v.Amount("amount", d.ResolvedAmount(), validate.AmountRule{Gateway: "Wave Pay"}).Err()
}

func randomReference() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
