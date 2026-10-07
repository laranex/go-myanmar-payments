package ayapay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments"
	"github.com/laranex/go-myanmar-payments/internal/testutil"
)

func newGateway(t *testing.T, server *testutil.Server) *Gateway {
	t.Helper()
	config := Config{AppKey: "app-key", AppSecret: "test-secret"}
	if server != nil {
		config.BaseURL = server.URL
	}
	gateway, err := New(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	gateway.now = func() time.Time { return time.Unix(1733470212, 0) }
	return gateway
}

func hmacHex(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func signed(t *testing.T, payload map[string]any, checksumString string) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"payload":      base64.StdEncoding.EncodeToString(raw),
		"checkSum":     hmacHex(checksumString, "test-secret"),
		"merchOrderId": payload["merchOrderId"],
	}
}

func vector(t *testing.T) (map[string]any, string) {
	t.Helper()
	fixture := testutil.Fixture(t, "aya_pay/callback_payload.json")
	return fixture["payload"].(map[string]any), fixture["checksum_string"].(string)
}

func callbackRequest(t *testing.T, body map[string]any) *myanmarpayments.CallbackRequest {
	t.Helper()
	request, err := myanmarpayments.NewCallbackRequestFromJSON(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestInitiateSignsTheFormInTheDocumentedOrder(t *testing.T) {
	gateway := newGateway(t, nil)
	payment, err := gateway.Initiate(PaymentData{OrderID: "ORD123456", Amount: myanmarpayments.Kyat(1000), Channel: "kbz_pay", Method: MethodQR, ReturnURL: "https://shop.test/done", Description: "Order", UserRefs: []string{"cart-9"}})
	if err != nil {
		t.Fatal(err)
	}

	want := hmacHex(strings.Join([]string{"ORD123456", "1000", "app-key", "1733470212", "cart-9", "", "", "", "", "Order", "104", "kbz_pay", "QR", "https://shop.test/done"}, ":"), "test-secret")
	checkSum, _ := payment.Field("checkSum")
	if payment.Action != SandboxURL+"/v1/payment/request" || payment.Enctype != "multipart/form-data" || checkSum != want {
		t.Fatalf("unexpected payment %+v", payment)
	}
	if !strings.Contains(payment.HTML(), `name="checkSum" value="`+want+`"`) {
		t.Fatal("HTML does not carry the checksum")
	}
}

func TestServicesListsChannelsAndMethods(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"status": "00", "message": "success", "data": []any{
		map[string]any{"name": "AYA Pay", "key": "aya_pay", "image_url": "https://img.test/aya.png", "methods": []any{"QR", "NOTI"}},
		map[string]any{"name": "JCB", "key": "jcb", "image_url": nil, "methods": []any{"WEB", "TOKEN"}},
	}}})

	services, err := newGateway(t, server).Services(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	request := server.Last(t).JSON(t)
	if request["checkSum"] != hmacHex("app-key:test-secret:1733470212", "test-secret") {
		t.Fatal("services checksum mismatch")
	}
	if len(services) != 2 || services[0].Key != "aya_pay" || !services[0].Supports(MethodQR) {
		t.Fatalf("unexpected services %+v", services)
	}
	if len(services[1].Methods) != 1 || services[1].Methods[0] != MethodWeb || services[1].UnknownMethods[0] != "TOKEN" {
		t.Fatalf("unexpected methods %+v", services[1])
	}
}

func TestCallbackIsVerifiedAgainstTheFixedFieldOrder(t *testing.T) {
	payload, checksum := vector(t)

	callback, err := newGateway(t, nil).HandleCallback(callbackRequest(t, signed(t, payload, checksum)))
	if err != nil {
		t.Fatal(err)
	}
	if !callback.IsSuccessful() || callback.OrderID != "ORD123456" || callback.GatewayReference != "TRN0001" || callback.Amount != "1000" {
		t.Fatalf("unexpected callback %+v", callback)
	}
}

func TestEveryStatusCodeIsMapped(t *testing.T) {
	cases := map[string]myanmarpayments.PaymentStatus{
		"00": myanmarpayments.StatusSuccessful, "01": myanmarpayments.StatusPending, "02": myanmarpayments.StatusFailed,
		"03": myanmarpayments.StatusFailed, "04": myanmarpayments.StatusExpired, "99": myanmarpayments.StatusUnknown,
	}
	for code, want := range cases {
		t.Run(code, func(t *testing.T) {
			payload, checksum := vector(t)
			payload = maps.Clone(payload)
			payload["statusCode"] = code
			checksum = strings.Replace(checksum, ":00:", ":"+code+":", 1)

			callback, err := newGateway(t, nil).HandleCallback(callbackRequest(t, signed(t, payload, checksum)))
			if err != nil {
				t.Fatal(err)
			}
			if callback.Status != want {
				t.Fatalf("got %s want %s", callback.Status, want)
			}
		})
	}
}

func TestWalletPayloadWithAbsentFieldsVerifies(t *testing.T) {
	payload := map[string]any{
		"merchOrderId": "LXAYA1007184542", "tranId": "C17913987426393655", "amount": "1000", "currenyCode": "104", "statusCode": "01",
		"paymentCardNumber": nil, "paymentMobileNumber": nil, "userRef1": nil, "userRef2": nil, "userRef3": nil,
		"userRef4": nil, "userRef5": nil, "description": nil, "dateTime": "2026-10-08 01:15:43",
	}
	checksum := "LXAYA1007184542:C17913987426393655:1000:104:01:::::::::2026-10-08 01:15:43"

	callback, err := newGateway(t, nil).HandleCallback(callbackRequest(t, signed(t, payload, checksum)))
	if err != nil {
		t.Fatal(err)
	}
	if callback.Status != myanmarpayments.StatusPending || callback.GatewayReference != "C17913987426393655" {
		t.Fatalf("unexpected callback %+v", callback)
	}
}

func TestVerifyRedirectReadsTheSignedQueryString(t *testing.T) {
	payload, checksum := vector(t)
	query := url.Values{}
	for key, value := range signed(t, payload, checksum) {
		query.Set(key, value.(string))
	}

	callback, err := newGateway(t, nil).VerifyRedirect(myanmarpayments.NewCallbackRequest(nil, nil, query))
	if err != nil || callback.OrderID != "ORD123456" {
		t.Fatalf("got %+v %v", callback, err)
	}
}

func TestTamperedCallbackIsRejected(t *testing.T) {
	payload, checksum := vector(t)
	body := signed(t, payload, checksum)
	tampered := maps.Clone(payload)
	tampered["amount"] = "1"
	raw, _ := json.Marshal(tampered)
	body["payload"] = base64.StdEncoding.EncodeToString(raw)

	_, err := newGateway(t, nil).HandleCallback(callbackRequest(t, body))
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureVerificationError, got %v", err)
	}
}

func TestStatusVerifiesTheEnquiryPayload(t *testing.T) {
	payload, checksum := vector(t)
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"status": "00", "message": "success", "data": signed(t, payload, checksum)}})

	result, err := newGateway(t, server).Status(context.Background(), "ORD123456")
	if err != nil {
		t.Fatal(err)
	}
	request := server.Last(t).JSON(t)
	timestamp := strconv.FormatFloat(request["timestamp"].(float64), 'f', -1, 64)
	if request["checkSum"] != hmacHex("ORD123456:"+timestamp+":app-key", "test-secret") {
		t.Fatal("enquiry checksum mismatch")
	}
	if !result.IsSuccessful() || result.GatewayReference != "TRN0001" {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestAPIErrorsCarryTheGatewayCode(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"status": "20", "message": "Transaction not found"}})

	_, err := newGateway(t, server).Status(context.Background(), "ORD123456")
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) || apiErr.GatewayCode != "20" || apiErr.GatewayMessage != "Transaction not found" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestValidationFollowsAYARules(t *testing.T) {
	cases := map[string]struct {
		data  PaymentData
		field string
	}{
		"short order id":  {PaymentData{OrderID: "A1", Amount: myanmarpayments.Kyat(1000), Channel: "aya_pay", Method: MethodQR}, "orderId"},
		"six user refs":   {PaymentData{OrderID: "ORD123456", Amount: myanmarpayments.Kyat(1000), Channel: "aya_pay", Method: MethodQR, UserRefs: []string{"1", "2", "3", "4", "5", "6"}}, "userRefs"},
		"unknown method":  {PaymentData{OrderID: "ORD123456", Amount: myanmarpayments.Kyat(1000), Channel: "aya_pay", Method: "SMS"}, "method"},
		"missing channel": {PaymentData{OrderID: "ORD123456", Amount: myanmarpayments.Kyat(1000), Method: MethodQR}, "channel"},
		"zero amount":     {PaymentData{OrderID: "ORD123456", Channel: "aya_pay", Method: MethodQR}, "amount"},
		"decimal amount":  {PaymentData{OrderID: "ORD123456", Amount: myanmarpayments.MustParseAmount("1000.50"), Channel: "aya_pay", Method: MethodQR}, "amount"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var invalid *myanmarpayments.InvalidPaymentDataError
			if err := tc.data.Validate(); !errors.As(err, &invalid) || invalid.Errors[tc.field] == "" {
				t.Fatalf("expected error on %s, got %v", tc.field, err)
			}
		})
	}
}
