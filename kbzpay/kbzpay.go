// Package kbzpay integrates KBZ Pay: PWA (redirect), QR and in-app payments, order status
// queries and payment notifications.
package kbzpay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/transport"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

var statuses = map[string]myanmarpayments.PaymentStatus{
	"PAY_SUCCESS":   myanmarpayments.StatusSuccessful,
	"WAIT_PAY":      myanmarpayments.StatusPending,
	"PAYING":        myanmarpayments.StatusPending,
	"PAY_FAILED":    myanmarpayments.StatusFailed,
	"ORDER_CLOSED":  myanmarpayments.StatusCanceled,
	"ORDER_EXPIRED": myanmarpayments.StatusExpired,
}

// Gateway talks to KBZ Pay.
type Gateway struct {
	config    Config
	transport *transport.Client
	signer    Signer
	now       func() time.Time
	nonce     func() string
}

// New returns a Gateway. A nil client uses myanmarpayments.DefaultHTTPClient.
func New(config Config, client myanmarpayments.HTTPDoer) (*Gateway, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	return &Gateway{
		config:    config,
		transport: transport.New(client),
		signer:    NewSigner(config.AppKey),
		now:       time.Now,
		nonce:     randomNonce,
	}, nil
}

// Config returns the configuration in use.
func (g *Gateway) Config() Config { return g.config }

// PWA creates an order and returns the KBZ Pay PWA checkout URL to redirect the customer to.
//
// The redirect only works on a phone with the KBZ Pay app installed, and its Referer must
// match the URL registered with KBZ.
func (g *Gateway) PWA(ctx context.Context, data PaymentData) (*myanmarpayments.RedirectPayment, error) {
	order, err := g.precreate(ctx, data, "PWAAPP")
	if err != nil {
		return nil, err
	}
	fields := g.orderInfoFields(order)

	return &myanmarpayments.RedirectPayment{
		OrderID:          data.OrderID,
		URL:              g.config.ResolvedPWAURL() + "?" + g.signer.SignString(fields) + "&sign=" + g.signer.Sign(fields),
		GatewayReference: order.prepayID,
		Raw:              order.response,
	}, nil
}

// QR creates an order and returns a QR payload for the customer to scan with the KBZ Pay app.
func (g *Gateway) QR(ctx context.Context, data PaymentData) (*myanmarpayments.QrPayment, error) {
	order, err := g.precreate(ctx, data, "PAY_BY_QRCODE")
	if err != nil {
		return nil, err
	}
	qrCode := values.Get(order.response, "qrCode")
	if qrCode == "" {
		return nil, &myanmarpayments.APIError{Message: "KBZ Pay did not return a QR code.", Raw: order.response}
	}

	payment := &myanmarpayments.QrPayment{OrderID: data.OrderID, QRString: qrCode, Reference: order.prepayID, Raw: order.response}
	if data.TimeoutMinutes > 0 {
		payment.ExpiresAt = g.now().Add(time.Duration(data.TimeoutMinutes) * time.Minute)
	}
	return payment, nil
}

// App creates an order and returns the signed values your mobile app passes to KBZPay.startPay().
// The SDK's own result only means the payment screen closed; rely on the callback or Status.
func (g *Gateway) App(ctx context.Context, data PaymentData) (*myanmarpayments.AppPayment, error) {
	order, err := g.precreate(ctx, data, "APP")
	if err != nil {
		return nil, err
	}
	fields := g.orderInfoFields(order)

	return &myanmarpayments.AppPayment{
		OrderID:   data.OrderID,
		OrderInfo: g.signer.SignString(fields),
		Sign:      g.signer.Sign(fields),
		SignType:  "SHA256",
		Raw:       order.response,
	}, nil
}

// Status asks KBZ Pay for the current state of an order (queryorder).
func (g *Gateway) Status(ctx context.Context, orderID string) (*myanmarpayments.PaymentStatusResult, error) {
	response, err := g.call(ctx, "queryorder", "kbz.payment.queryorder", "3.0", map[string]string{
		"appid":          g.config.AppID,
		"merch_code":     g.config.MerchantCode,
		"merch_order_id": orderID,
	}, nil, g.nonce(), g.timestamp())
	if err != nil {
		return nil, err
	}

	tradeStatus := values.Trimmed(response, "trade_status")
	result := &myanmarpayments.PaymentStatusResult{
		OrderID:          orderID,
		Status:           myanmarpayments.ResolveStatus(statuses, tradeStatus),
		GatewayStatus:    tradeStatus,
		GatewayReference: values.Get(response, "mm_order_id"),
		Amount:           values.Get(response, "total_amount"),
		Raw:              response,
	}
	if id := values.Get(response, "merch_order_id"); id != "" {
		result.OrderID = id
	}
	return result, nil
}

// HandleCallback verifies KBZ Pay's payment notification. Reply with the callback's
// Acknowledgement (plain "success") or KBZ retries.
func (g *Gateway) HandleCallback(request *myanmarpayments.CallbackRequest) (*myanmarpayments.PaymentCallback, error) {
	input := request.ParsedBody()
	fields := values.Map(input, "Request")
	if fields == nil {
		fields = input
	}

	if !g.signer.Verify(fields) {
		return nil, &myanmarpayments.SignatureVerificationError{Message: "KBZ Pay callback signature verification failed.", Raw: input}
	}

	tradeStatus := values.Trimmed(fields, "trade_status")
	acknowledgement := myanmarpayments.DefaultAcknowledgement()
	acknowledgement.Body = "success"

	return &myanmarpayments.PaymentCallback{
		OrderID:          values.Get(fields, "merch_order_id"),
		Status:           myanmarpayments.ResolveStatus(statuses, tradeStatus),
		GatewayStatus:    tradeStatus,
		GatewayReference: values.Get(fields, "mm_order_id"),
		Amount:           values.Get(fields, "total_amount"),
		Raw:              fields,
		Acknowledgement:  acknowledgement,
	}, nil
}

type order struct {
	prepayID  string
	nonce     string
	timestamp string
	response  map[string]any
}

func (g *Gateway) precreate(ctx context.Context, data PaymentData, tradeType string) (order, error) {
	if err := data.Validate(); err != nil {
		return order{}, err
	}

	biz := map[string]string{
		"appid":          g.config.AppID,
		"merch_code":     g.config.MerchantCode,
		"merch_order_id": data.OrderID,
		"trade_type":     tradeType,
		"total_amount":   data.Amount.String(),
		"trans_currency": "MMK",
	}
	if data.Title != "" {
		biz["title"] = data.Title
	}
	if data.TimeoutMinutes > 0 {
		biz["timeout_express"] = strconv.Itoa(data.TimeoutMinutes) + "m"
	}
	if data.CallbackInfo != "" {
		biz["callback_info"] = url.QueryEscape(data.CallbackInfo)
	}

	nonce, timestamp := g.nonce(), g.timestamp()
	response, err := g.call(ctx, "precreate", "kbz.payment.precreate", "1.0", biz, map[string]string{"notify_url": data.CallbackURL}, nonce, timestamp)
	if err != nil {
		return order{}, err
	}

	prepayID := values.Get(response, "prepay_id")
	if prepayID == "" {
		return order{}, &myanmarpayments.APIError{Message: "KBZ Pay did not return a prepay_id.", Raw: response}
	}
	return order{prepayID: prepayID, nonce: nonce, timestamp: timestamp, response: response}, nil
}

// call sends a signed request and returns the Response object, failing when KBZ reports an error.
func (g *Gateway) call(ctx context.Context, endpoint, method, version string, biz, extra map[string]string, nonce, timestamp string) (map[string]any, error) {
	common := map[string]string{"timestamp": timestamp, "method": method, "nonce_str": nonce, "version": version}
	for key, value := range extra {
		common[key] = value
	}

	signed := map[string]any{}
	request := map[string]any{}
	for key, value := range common {
		signed[key] = value
		request[key] = value
	}
	for key, value := range biz {
		signed[key] = value
	}
	request["sign_type"] = "SHA256"
	request["sign"] = g.signer.Sign(signed)
	request["biz_content"] = biz

	response, err := g.transport.PostJSON(ctx, g.config.ResolvedAPIURL()+"/"+endpoint, map[string]any{"Request": request}, nil)
	if err != nil {
		return nil, err
	}

	body := response.JSON()
	result := values.Map(body, "Response")
	if result == nil {
		result = map[string]any{}
	}

	if !response.Successful() || values.Get(result, "result") != "SUCCESS" || values.Get(result, "code") != "0" {
		code, message := values.Get(result, "code"), values.Get(result, "msg")
		text := fmt.Sprintf("KBZ Pay %s failed with HTTP %d.", endpoint, response.Status)
		if code != "" {
			text = fmt.Sprintf("KBZ Pay %s failed: [%s] %s", endpoint, code, message)
		}
		return nil, &myanmarpayments.APIError{Message: text, GatewayCode: code, GatewayMessage: message, HTTPStatus: response.Status, Raw: body}
	}

	return result, nil
}

func (g *Gateway) orderInfoFields(o order) map[string]any {
	return stringFields(map[string]string{
		"appid":      g.config.AppID,
		"merch_code": g.config.MerchantCode,
		"nonce_str":  o.nonce,
		"prepay_id":  o.prepayID,
		"timestamp":  o.timestamp,
	})
}

func (g *Gateway) timestamp() string { return strconv.FormatInt(g.now().Unix(), 10) }

func randomNonce() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
