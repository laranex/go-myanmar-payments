package myanmarpayments

import (
	"html/template"
	"strings"
	"time"
)

// PaymentResult is returned when a payment is initiated. Each flow has its own type.
type PaymentResult interface {
	Flow() PaymentFlow
}

// RedirectPayment: send the customer's browser to URL to complete the payment.
type RedirectPayment struct {
	// OrderID is your order id, as sent to the gateway.
	OrderID string
	// URL is the gateway page to redirect the customer to.
	URL string
	// GatewayReference is the gateway's id for this attempt (Wave transaction_id, KBZ prepay_id).
	GatewayReference string
	// Raw is the gateway's response, for logging.
	Raw map[string]any
}

// Flow returns FlowRedirect.
func (RedirectPayment) Flow() PaymentFlow { return FlowRedirect }

// FormField is one signed hidden field of a FormPayment.
type FormField struct {
	Name  string
	Value string
}

// FormPayment: POST Fields to Action from the customer's browser. HTML renders a page that
// does this automatically.
type FormPayment struct {
	// OrderID is your order id, as sent to the gateway.
	OrderID string
	// Action is the gateway URL the form posts to.
	Action string
	// Fields are the signed hidden fields, in signing order. Post them unchanged.
	Fields []FormField
	// Enctype is the encoding the gateway expects for the form.
	Enctype string
}

// Flow returns FlowForm.
func (FormPayment) Flow() PaymentFlow { return FlowForm }

// Field returns the value of the named field.
func (p FormPayment) Field(name string) (string, bool) {
	for _, field := range p.Fields {
		if field.Name == name {
			return field.Value, true
		}
	}

	return "", false
}

// Values returns the fields as a map.
func (p FormPayment) Values() map[string]string {
	values := make(map[string]string, len(p.Fields))
	for _, field := range p.Fields {
		values[field.Name] = field.Value
	}

	return values
}

// HTML returns a complete page that posts the form as soon as it loads. All values are escaped.
func (p FormPayment) HTML() string {
	escape := template.HTMLEscapeString
	enctype := p.Enctype
	if enctype == "" {
		enctype = "application/x-www-form-urlencoded"
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>Redirecting to payment</title></head><body>`)
	b.WriteString(`<form id="payment-form" method="POST" action="` + escape(p.Action) + `" enctype="` + escape(enctype) + `">`)
	for _, field := range p.Fields {
		b.WriteString(`<input type="hidden" name="` + escape(field.Name) + `" value="` + escape(field.Value) + `">`)
	}
	b.WriteString(`<noscript><button type="submit">Continue to payment</button></noscript></form>`)
	b.WriteString(`<script>document.getElementById("payment-form").submit();</script></body></html>`)

	return b.String()
}

// QrPayment: show a QR code for the customer to scan. Gateways return either a payload to
// encode (QRString) or a ready-made image (QRImage).
type QrPayment struct {
	// OrderID is your order id, as sent to the gateway.
	OrderID string
	// QRString is a QR payload to encode into an image yourself (KBZ Pay).
	QRString string
	// QRImage is a base64 encoded image to display as is (Yoma MMQR).
	QRImage string
	// ExpiresAt is when the QR stops being payable; zero when the gateway does not limit it.
	ExpiresAt time.Time
	// Reference is the gateway's id for this QR, used to check its status (Yoma refLabel).
	Reference string
	// Raw is the gateway's response, for logging.
	Raw map[string]any
}

// Flow returns FlowQR.
func (QrPayment) Flow() PaymentFlow { return FlowQR }

// QRImageDataURI returns the image as a data URI for an <img src>, or "" when there is no image.
func (p QrPayment) QRImageDataURI(mimeType string) string {
	if p.QRImage == "" {
		return ""
	}
	if mimeType == "" {
		mimeType = "image/png"
	}

	return "data:" + mimeType + ";base64," + p.QRImage
}

// AppPayment: pass these values to your mobile app, which hands them to the wallet's SDK.
type AppPayment struct {
	// OrderID is your order id, as sent to the gateway.
	OrderID string `json:"orderId"`
	// OrderInfo is the signed order string the SDK expects.
	OrderInfo string `json:"orderInfo"`
	// Sign is the signature of OrderInfo.
	Sign string `json:"sign"`
	// SignType is the signature algorithm, e.g. SHA256.
	SignType string `json:"signType"`
	// Raw is the gateway's response, for logging.
	Raw map[string]any `json:"-"`
}

// Flow returns FlowApp.
func (AppPayment) Flow() PaymentFlow { return FlowApp }
