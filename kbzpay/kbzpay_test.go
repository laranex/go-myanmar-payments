package kbzpay

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/testutil"
)

func newGateway(t *testing.T, server *testutil.Server) *Gateway {
	t.Helper()
	config := Config{AppID: "kp123", AppKey: "secret-key", MerchantCode: "100001"}
	if server != nil {
		config.APIURL = server.URL
	}
	gateway, err := New(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	gateway.now = func() time.Time { return time.Unix(1536637503, 0) }
	gateway.nonce = func() string { return "nonce123" }
	return gateway
}

func precreate(extra map[string]any) testutil.Reply {
	response := map[string]any{"result": "SUCCESS", "code": "0", "msg": "success", "prepay_id": "PREPAY123"}
	for key, value := range extra {
		response[key] = value
	}
	return testutil.Reply{Body: map[string]any{"Response": response}}
}

var data = PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/kbz/callback"}

func TestSignStringMatchesTheKBZDocsExample(t *testing.T) {
	vector := testutil.Fixture(t, "kbz_pay/sign_string.json")
	fields := vector["fields"].(map[string]any)

	if got, want := NewSigner("any").SignString(fields), vector["expected"].(string); got != want {
		t.Fatalf("sign string\n got %s\nwant %s", got, want)
	}
}

func TestSignStringDoesNotURLEncode(t *testing.T) {
	got := NewSigner("k").SignString(map[string]any{"callback_info": "title%3Diphonex", "notify_url": "https://a.test/x y"})
	if want := "callback_info=title%3Diphonex&notify_url=https://a.test/x y"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestPWASendsASignedPrecreateAndReturnsTheRedirectURL(t *testing.T) {
	server := testutil.NewServer(t, precreate(nil))
	gateway := newGateway(t, server)

	payment, err := gateway.PWA(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}

	recorded := server.Last(t)
	request := recorded.JSON(t)["Request"].(map[string]any)
	biz := request["biz_content"].(map[string]any)
	if recorded.Path != "/precreate" || request["method"] != "kbz.payment.precreate" || request["notify_url"] != data.CallbackURL {
		t.Fatalf("unexpected request %v", request)
	}
	if biz["merch_order_id"] != "ORDER_1" || biz["total_amount"] != "1000" || biz["trade_type"] != "PWAAPP" || biz["trans_currency"] != "MMK" {
		t.Fatalf("unexpected biz_content %v", biz)
	}
	signed := map[string]any{}
	for key, value := range request {
		if key != "biz_content" {
			signed[key] = value
		}
	}
	for key, value := range biz {
		signed[key] = value
	}
	if request["sign"] != gateway.signer.Sign(signed) {
		t.Fatal("request signature mismatch")
	}

	if !strings.HasPrefix(payment.URL, SandboxPWAURL+"?") || payment.GatewayReference != "PREPAY123" {
		t.Fatalf("unexpected payment %+v", payment)
	}
	query, _ := url.ParseQuery(payment.URL[strings.Index(payment.URL, "?")+1:])
	fields := map[string]any{}
	for key := range query {
		if key != "sign" {
			fields[key] = query.Get(key)
		}
	}
	if query.Get("prepay_id") != "PREPAY123" || query.Get("sign") != gateway.signer.Sign(fields) {
		t.Fatalf("unexpected PWA query %v", query)
	}
}

func TestQRReturnsThePayload(t *testing.T) {
	server := testutil.NewServer(t, precreate(map[string]any{"qrCode": "kbzpay://qr/abc"}))

	payment, err := newGateway(t, server).QR(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if payment.QRString != "kbzpay://qr/abc" || payment.QRImage != "" {
		t.Fatalf("unexpected payment %+v", payment)
	}
	if server.Last(t).JSON(t)["Request"].(map[string]any)["biz_content"].(map[string]any)["trade_type"] != "PAY_BY_QRCODE" {
		t.Fatal("wrong trade type")
	}
}

func TestAppReturnsSignedOrderInfo(t *testing.T) {
	gateway := newGateway(t, testutil.NewServer(t, precreate(nil)))

	payment, err := gateway.App(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	want := "appid=kp123&merch_code=100001&nonce_str=nonce123&prepay_id=PREPAY123&timestamp=1536637503"
	if payment.OrderInfo != want || payment.SignType != "SHA256" {
		t.Fatalf("unexpected payment %+v", payment)
	}
	fields := map[string]any{"appid": "kp123", "merch_code": "100001", "nonce_str": "nonce123", "prepay_id": "PREPAY123", "timestamp": "1536637503"}
	if payment.Sign != gateway.signer.Sign(fields) {
		t.Fatal("sign mismatch")
	}
}

func TestOptionalFieldsGoInBizContentAndDecimalsAreKept(t *testing.T) {
	server := testutil.NewServer(t, precreate(nil))

	_, err := newGateway(t, server).PWA(context.Background(), PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.MustParseAmount("1000.50"), CallbackURL: "https://shop.test/cb", Title: "Shoes", TimeoutMinutes: 30, CallbackInfo: "cart=9"})
	if err != nil {
		t.Fatal(err)
	}
	biz := server.Last(t).JSON(t)["Request"].(map[string]any)["biz_content"].(map[string]any)
	if biz["title"] != "Shoes" || biz["timeout_express"] != "30m" || biz["callback_info"] != "cart%3D9" || biz["total_amount"] != "1000.50" {
		t.Fatalf("unexpected biz_content %v", biz)
	}
}

func TestPrecreateFailureIsAnAPIError(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"Response": map[string]any{"result": "FAIL", "code": "ORDER_ID_USED", "msg": "Order id used"}}})

	_, err := newGateway(t, server).PWA(context.Background(), data)
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) || apiErr.GatewayCode != "ORDER_ID_USED" || apiErr.GatewayMessage != "Order id used" {
		t.Fatalf("expected APIError, got %v", err)
	}
}

func TestStatusMapsEveryTradeStatus(t *testing.T) {
	cases := map[string]myanmarpayments.PaymentStatus{
		"PAY_SUCCESS": myanmarpayments.StatusSuccessful, " PAY_SUCCESS": myanmarpayments.StatusSuccessful,
		"WAIT_PAY": myanmarpayments.StatusPending, "PAYING": myanmarpayments.StatusPending,
		"PAY_FAILED": myanmarpayments.StatusFailed, "ORDER_CLOSED": myanmarpayments.StatusCancelled,
		"ORDER_EXPIRED": myanmarpayments.StatusExpired, "SOMETHING_NEW": myanmarpayments.StatusUnknown,
	}
	for tradeStatus, want := range cases {
		t.Run(tradeStatus, func(t *testing.T) {
			server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"Response": map[string]any{
				"result": "SUCCESS", "code": "0", "merch_order_id": "ORDER_1", "trade_status": tradeStatus, "total_amount": "1000", "mm_order_id": "MM1",
			}}})

			result, err := newGateway(t, server).Status(context.Background(), "ORDER_1")
			if err != nil {
				t.Fatal(err)
			}
			request := server.Last(t).JSON(t)["Request"].(map[string]any)
			if request["method"] != "kbz.payment.queryorder" || request["version"] != "3.0" {
				t.Fatalf("unexpected request %v", request)
			}
			if result.Status != want || result.GatewayStatus != strings.TrimSpace(tradeStatus) || result.GatewayReference != "MM1" || result.Amount != "1000" {
				t.Fatalf("unexpected result %+v", result)
			}
		})
	}
}

func signedCallback(t *testing.T, signer Signer, fields map[string]any) *myanmarpayments.CallbackRequest {
	t.Helper()
	fields["sign_type"] = "SHA256"
	fields["sign"] = signer.Sign(fields)
	request, err := myanmarpayments.NewCallbackRequestFromJSON(map[string]any{"Request": fields}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestCallbackIsVerifiedAndAcknowledgedWithSuccess(t *testing.T) {
	gateway := newGateway(t, nil)
	callback, err := gateway.HandleCallback(signedCallback(t, gateway.signer, map[string]any{
		"appid": "kp123", "notify_time": 1536637503, "merch_code": "100001", "merch_order_id": "ORDER_1", "mm_order_id": "0112345",
		"total_amount": "1000", "trans_currency": "MMK", "trade_status": "PAY_SUCCESS", "callback_info": "title%3Diphonex", "nonce_str": "abc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !callback.IsSuccessful() || callback.OrderID != "ORDER_1" || callback.GatewayReference != "0112345" || callback.Amount != "1000" {
		t.Fatalf("unexpected callback %+v", callback)
	}
	if callback.Acknowledgement.Body != "success" || callback.Acknowledgement.Status != 200 {
		t.Fatalf("unexpected acknowledgement %+v", callback.Acknowledgement)
	}
}

func TestTamperedCallbackIsRejected(t *testing.T) {
	gateway := newGateway(t, nil)
	fields := map[string]any{"merch_order_id": "ORDER_1", "total_amount": "1000", "trade_status": "PAY_SUCCESS", "sign_type": "SHA256"}
	fields["sign"] = gateway.signer.Sign(fields)
	fields["total_amount"] = "1"
	request, _ := myanmarpayments.NewCallbackRequestFromJSON(map[string]any{"Request": fields}, nil)

	_, err := gateway.HandleCallback(request)
	var sigErr *myanmarpayments.SignatureVerificationError
	if !errors.As(err, &sigErr) {
		t.Fatalf("expected SignatureVerificationError, got %v", err)
	}
}

func TestValidationFollowsKBZLimits(t *testing.T) {
	cases := map[string]struct {
		data  PaymentData
		field string
	}{
		"order id with dashes":    {PaymentData{OrderID: "ORDER-1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb"}, "orderId"},
		"order id too long":       {PaymentData{OrderID: strings.Repeat("a", 41), Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb"}, "orderId"},
		"zero amount":             {PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(0), CallbackURL: "https://shop.test/cb"}, "amount"},
		"three decimals":          {PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.MustParseAmount("1000.505"), CallbackURL: "https://shop.test/cb"}, "amount"},
		"negative":                {PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(-5), CallbackURL: "https://shop.test/cb"}, "amount"},
		"callback url with query": {PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb?x=1"}, "callbackUrl"},
		"timeout above 120":       {PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/cb", TimeoutMinutes: 121}, "timeoutMinutes"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var invalid *myanmarpayments.InvalidPaymentDataError
			if err := tc.data.Validate(); !errors.As(err, &invalid) || invalid.Errors[tc.field] == "" {
				t.Fatalf("expected an error on %s, got %v", tc.field, err)
			}
		})
	}
}

func TestConfigNamesAMissingKeyAndNormalisesThePWAURL(t *testing.T) {
	_, err := New(Config{AppID: "a", MerchantCode: "c"}, nil)
	var configErr *myanmarpayments.ConfigurationError
	if !errors.As(err, &configErr) || configErr.Key != "app_key" {
		t.Fatalf("expected missing app_key, got %v", err)
	}
	for _, pwa := range []string{"https://static.kbzpay.com/pgw/uat/pwa/#", "https://static.kbzpay.com/pgw/uat/pwa/#/"} {
		if got := (Config{PWAURL: pwa}).ResolvedPWAURL(); got != SandboxPWAURL {
			t.Fatalf("pwa %s normalized to %s", pwa, got)
		}
	}
	if (Config{Production: true}).ResolvedAPIURL() != ProductionAPIURL {
		t.Fatal("production URL not selected")
	}
}

func TestKBZAmountErrorsNameTheGateway(t *testing.T) {
	var invalid *myanmarpayments.InvalidPaymentDataError
	err := PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.MustParseAmount("1000.505"), CallbackURL: "https://shop.test/cb"}.Validate()
	if !errors.As(err, &invalid) || !strings.Contains(invalid.Errors["amount"], "KBZ Pay accepts at most 2 decimal places") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestQRExpiresWithTheTimeoutAndNeedsAQRCode(t *testing.T) {
	withTimeout := data
	withTimeout.TimeoutMinutes = 15
	payment, err := newGateway(t, testutil.NewServer(t, precreate(map[string]any{"qrCode": "kbzpay://qr/abc"}))).QR(context.Background(), withTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if !payment.ExpiresAt.Equal(time.Unix(1536637503, 0).Add(15*time.Minute)) || payment.Reference != "PREPAY123" {
		t.Fatalf("unexpected payment %+v", payment)
	}

	var apiErr *myanmarpayments.APIError
	if _, err := newGateway(t, testutil.NewServer(t, precreate(nil))).QR(context.Background(), data); !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError without a qrCode, got %v", err)
	}
}

func TestMissingPrepayIDAndInvalidDataAreErrors(t *testing.T) {
	var apiErr *myanmarpayments.APIError
	if _, err := newGateway(t, testutil.NewServer(t, precreate(map[string]any{"prepay_id": ""}))).App(context.Background(), data); !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError without a prepay_id, got %v", err)
	}
	var invalid *myanmarpayments.InvalidPaymentDataError
	if _, err := newGateway(t, nil).QR(context.Background(), PaymentData{}); !errors.As(err, &invalid) {
		t.Fatalf("expected InvalidPaymentDataError, got %v", err)
	}
}
