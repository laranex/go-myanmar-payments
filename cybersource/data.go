package cybersource

import (
	"regexp"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/validate"
)

// TransactionType is what CyberSource does with the card.
type TransactionType string

const (
	// Sale authorizes and captures in one step.
	Sale TransactionType = "sale"
	// Authorization authorizes only; capture later.
	Authorization TransactionType = "authorization"
	// SaleAndCreateToken is a sale that also saves the card as a payment token.
	SaleAndCreateToken TransactionType = "sale,create_payment_token"
	// AuthorizationAndCreateToken is an authorization that also saves the card as a payment token.
	AuthorizationAndCreateToken TransactionType = "authorization,create_payment_token"
)

var (
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
	localePattern   = regexp.MustCompile(`^[a-z]{2}-[a-z]{2}$`)
)

// PaymentData is a Secure Acceptance card payment. CyberSource is multi-currency.
type PaymentData struct {
	// OrderID is your order id (reference_number), at most 50 characters. Echoed back as req_reference_number.
	OrderID string
	// Amount is the order total in Currency, 0 or more and at most 15 characters including the
	// decimal point, e.g. myanmarpayments.MustParseAmount("10.50").
	Amount myanmarpayments.Amount
	// CallbackURL is the URL CyberSource posts the result to (override_backoffice_post_url), at most 255 characters.
	CallbackURL string
	// ReturnURL is the receipt page (override_custom_receipt_page), at most 255 characters (optional).
	ReturnURL string
	// CancelURL is the page shown on cancel (override_custom_cancel_page), at most 255 characters (optional).
	CancelURL string
	// Currency is an ISO 4217 code, e.g. MMK (required).
	Currency string
	// TransactionType is what to do with the card, e.g. Sale (required).
	TransactionType TransactionType
	// Locale is the hosted page language, e.g. en-us (required).
	Locale string
}

// Validate checks the payment against the Secure Acceptance field reference.
func (d PaymentData) Validate() error {
	t := d.TransactionType
	return validate.New().
		Required("orderId", d.OrderID).
		Max("orderId", d.OrderID, 50).
		Amount("amount", d.Amount, validate.AmountRule{Gateway: "CyberSource", MaxDecimals: -1, MaxLength: 15, AllowZero: true}).
		Required("callbackUrl", d.CallbackURL).
		URL("callbackUrl", d.CallbackURL).Max("callbackUrl", d.CallbackURL, 255).
		URL("returnUrl", d.ReturnURL).Max("returnUrl", d.ReturnURL, 255).
		URL("cancelUrl", d.CancelURL).Max("cancelUrl", d.CancelURL, 255).
		Required("currency", d.Currency).
		Pattern("currency", d.Currency, currencyPattern, "a three letter ISO 4217 code").
		Required("transactionType", string(t)).
		When(t != "" && t != Sale && t != Authorization && t != SaleAndCreateToken && t != AuthorizationAndCreateToken, "transactionType", "The transactionType field is not a supported transaction type.").
		Required("locale", d.Locale).
		Pattern("locale", d.Locale, localePattern, "a locale code such as en-us").
		Err()
}
