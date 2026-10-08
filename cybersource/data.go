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
	// CallbackURL is the HTTPS URL CyberSource posts the result to (override_backoffice_post_url), at most 255 characters.
	CallbackURL string
	// ReturnURL is the HTTPS receipt page (override_custom_receipt_page), at most 255 characters (optional).
	ReturnURL string
	// CancelURL is the HTTPS page shown on cancel (override_custom_cancel_page), at most 255 characters (optional).
	CancelURL string
	// Currency is an ISO 4217 code; empty means MMK.
	Currency string
	// TransactionType is what to do with the card; empty means Sale.
	TransactionType TransactionType
	// Locale is the hosted page language, e.g. en-us; empty means en-us.
	Locale string
}

func (d PaymentData) currency() string {
	if d.Currency == "" {
		return "MMK"
	}
	return d.Currency
}

func (d PaymentData) transactionType() TransactionType {
	if d.TransactionType == "" {
		return Sale
	}
	return d.TransactionType
}

func (d PaymentData) locale() string {
	if d.Locale == "" {
		return "en-us"
	}
	return d.Locale
}

// Validate checks the payment against the Secure Acceptance field reference.
func (d PaymentData) Validate() error {
	t := d.transactionType()
	return validate.New().
		Required("orderId", d.OrderID).
		Max("orderId", d.OrderID, 50).
		Amount("amount", d.Amount, validate.AmountRule{Gateway: "CyberSource", MaxDecimals: -1, MaxLength: 15, AllowZero: true}).
		Required("callbackUrl", d.CallbackURL).
		URL("callbackUrl", d.CallbackURL).HTTPS("callbackUrl", d.CallbackURL).Max("callbackUrl", d.CallbackURL, 255).
		URL("returnUrl", d.ReturnURL).HTTPS("returnUrl", d.ReturnURL).Max("returnUrl", d.ReturnURL, 255).
		URL("cancelUrl", d.CancelURL).HTTPS("cancelUrl", d.CancelURL).Max("cancelUrl", d.CancelURL, 255).
		Pattern("currency", d.currency(), currencyPattern, "a three letter ISO 4217 code").
		Pattern("locale", d.locale(), localePattern, "a locale code such as en-us").
		When(t != Sale && t != Authorization && t != SaleAndCreateToken && t != AuthorizationAndCreateToken, "transactionType", "The transactionType field is not a supported transaction type.").
		Err()
}
