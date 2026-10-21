package myanmarpayments

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestEveryResultReportsItsFlow(t *testing.T) {
	cases := map[PaymentFlow]PaymentResult{
		FlowRedirect: &RedirectPayment{},
		FlowForm:     &FormPayment{},
		FlowQR:       &QrPayment{},
		FlowApp:      &AppPayment{},
		FlowMiniApp:  &MiniAppPayment{},
	}
	for want, result := range cases {
		if result.Flow() != want {
			t.Fatalf("%T: got %s, want %s", result, result.Flow(), want)
		}
	}
}

func TestMiniAppPaymentWritesTheMakePaymentObject(t *testing.T) {
	payment := MiniAppPayment{
		OrderID: "O<1>", Amount: MustParseAmount("1000"), MiniAppID: "M", MerchantReferenceID: "R/ü", CallbackURL: "https://shop.test/cb?a=1&b=2",
		HashValue: "h", Items: []MiniAppItem{{Name: "A \"B\"\n", Amount: Kyat(400)}, {Name: "C", Amount: MustParseAmount("600")}},
	}
	want := `{"amount":1000,"miniAppID":"M","merchantRefID":"R/ü","orderID":"O<1>","callBackUrl":"https://shop.test/cb?a=1&b=2","hashValue":"h","items":[{"name":"A \"B\"\n","amount":400},{"name":"C","amount":600}]}`
	if payment.JSON() != want {
		t.Fatalf("json\n got %s\nwant %s", payment.JSON(), want)
	}
	marshaled, err := json.Marshal(payment)
	if err != nil || !json.Valid(marshaled) || !strings.Contains(string(marshaled), `"orderID":"O\u003c1\u003e"`) {
		t.Fatalf("marshaled %s, %v", marshaled, err)
	}
	if got := (MiniAppPayment{}).JSON(); got != `{"amount":0,"miniAppID":"","merchantRefID":"","orderID":"","callBackUrl":"","hashValue":"","items":[]}` {
		t.Fatalf("empty payment %s", got)
	}
	// Invalid UTF-8 becomes U+FFFD; Go versions differ in writing it raw or as \ufffd, so compare decoded.
	got := (MiniAppPayment{Items: []MiniAppItem{{Name: "\xff"}}}).JSON()
	var decoded struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil || len(decoded.Items) != 1 || decoded.Items[0].Name != "\ufffd" {
		t.Fatalf("invalid UTF-8 %s (%v)", got, err)
	}
}

func TestFormPaymentFieldAndValues(t *testing.T) {
	payment := FormPayment{Fields: []FormField{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}}}
	if value, ok := payment.Field("b"); !ok || value != "2" {
		t.Fatal("expected field b")
	}
	if _, ok := payment.Field("missing"); ok {
		t.Fatal("unexpected field")
	}
	if values := payment.Values(); len(values) != 2 || values["a"] != "1" {
		t.Fatalf("unexpected values %v", values)
	}
	if html := payment.HTML(); !strings.Contains(html, `enctype="application/x-www-form-urlencoded"`) {
		t.Fatalf("expected the default enctype in %s", html)
	}
}

func TestAppPaymentJSONLeavesRawOut(t *testing.T) {
	encoded, err := json.Marshal(AppPayment{OrderID: "O", OrderInfo: "info", Sign: "S", SignType: "SHA256", Raw: map[string]any{"secret": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"orderId":"O","orderInfo":"info","sign":"S","signType":"SHA256"}` {
		t.Fatalf("unexpected JSON %s", encoded)
	}
	if (QrPayment{}).QRImageDataURI("image/jpeg") != "" || (QrPayment{QRImage: "x"}).QRImageDataURI("image/jpeg") != "data:image/jpeg;base64,x" {
		t.Fatal("unexpected data URI")
	}
}

func TestIsSuccessful(t *testing.T) {
	if !(PaymentCallback{Status: StatusSuccessful}).IsSuccessful() || (PaymentCallback{Status: StatusPending}).IsSuccessful() {
		t.Fatal("unexpected callback IsSuccessful")
	}
	if !(PaymentStatusResult{Status: StatusSuccessful}).IsSuccessful() || (PaymentStatusResult{Status: StatusUnknown}).IsSuccessful() {
		t.Fatal("unexpected status IsSuccessful")
	}
}

func TestCallbackRequestFromJSON(t *testing.T) {
	request, err := NewCallbackRequestFromJSON(map[string]any{"orderNumber": "ORD-1", "amount": json.Number("1000")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if request.HeaderValue("content-type") != "application/json" || request.ParsedBody()["amount"] != json.Number("1000") || request.Query == nil {
		t.Fatalf("unexpected request %+v", request)
	}
	if _, err := NewCallbackRequestFromJSON(map[string]any{"bad": make(chan int)}, nil); err == nil {
		t.Fatal("expected an encoding error")
	}
	if request.HeaderValue("missing") != "" {
		t.Fatal("unexpected header")
	}
	if len(NewCallbackRequest(nil, nil, nil).ParsedBody()) != 0 {
		t.Fatal("expected an empty body")
	}
}

func TestAcknowledgementDefaultsTo200(t *testing.T) {
	recorder := &statusRecorder{header: http.Header{}}
	if err := (Acknowledgement{}).Write(recorder); err != nil || recorder.status != http.StatusOK {
		t.Fatalf("unexpected status %d (%v)", recorder.status, err)
	}
}

type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header         { return r.header }
func (r *statusRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *statusRecorder) WriteHeader(status int)      { r.status = status }

func TestErrorMessages(t *testing.T) {
	invalid := &InvalidPaymentDataError{Errors: map[string]string{"b": "B is wrong.", "a": "A is wrong."}}
	if invalid.Error() != "myanmarpayments: Invalid payment data: A is wrong. B is wrong." {
		t.Fatalf("unexpected message %q", invalid.Error())
	}
	if (&APIError{Message: "KBZ Pay precreate failed."}).Error() != "myanmarpayments: KBZ Pay precreate failed." {
		t.Fatal("unexpected API error message")
	}
	if (&SignatureVerificationError{Message: "bad"}).Error() != "myanmarpayments: bad" {
		t.Fatal("unexpected signature error message")
	}
	if (&ConfigurationError{Gateway: "kbz_pay", Key: "app_key"}).Error() != "myanmarpayments: The kbz_pay configuration is missing [app_key]." {
		t.Fatal("unexpected configuration error message")
	}
	if (&ConfigurationError{Gateway: "wave_pay", Key: "time_to_live_in_seconds", Invalid: true}).Error() != "myanmarpayments: The wave_pay configuration [time_to_live_in_seconds] must be a whole number greater than 0." {
		t.Fatal("unexpected invalid configuration error message")
	}
	var apiErr *APIError
	if errors.As(errors.New("plain"), &apiErr) {
		t.Fatal("unexpected match")
	}
}

func TestMemoryTokenCacheDeleteAndNoExpiry(t *testing.T) {
	cache := NewMemoryTokenCache()
	cache.Set("forever", "v", 0)
	cache.Set("gone", "v", time.Hour)
	cache.Delete("gone")
	if _, ok := cache.Get("gone"); ok {
		t.Fatal("expected deleted key to be gone")
	}
	if value, ok := cache.Get("forever"); !ok || value != "v" {
		t.Fatal("expected a ttl of zero to never expire")
	}
	if _, ok := cache.Get("missing"); ok {
		t.Fatal("unexpected value")
	}
}

func TestFormPaymentHTMLEscapesOnlyTheFiveHTMLCharacters(t *testing.T) {
	payment := FormPayment{Action: "https://pay.test/", Fields: []FormField{{Name: "n", Value: "a\x00b <&>\"'"}}}
	if !strings.Contains(payment.HTML(), `value="a`+"\x00"+`b &lt;&amp;&gt;&#34;&#39;"`) {
		t.Fatalf("unexpected html %s", payment.HTML())
	}
}

func TestFormBodiesAreDecodedLikeTheOtherSDKs(t *testing.T) {
	request := NewCallbackRequest([]byte("decision=ACCEPT&bad=%zz&&semi=1;b=2&a+b=x%2By&a+b=second&flag&amount=10.50"), nil, nil)
	body := request.ParsedBody()
	want := map[string]any{"decision": "ACCEPT", "bad": "%zz", "semi": "1;b=2", "a b": "x+y", "flag": "", "amount": "10.50"}
	if fmt.Sprint(body) != fmt.Sprint(want) {
		t.Fatalf("body %v, want %v", body, want)
	}
}

func TestOnlyASCIIWhitespaceAroundTheBodyIsTrimmed(t *testing.T) {
	if got := NewCallbackRequest([]byte(" \t\n{\"a\":\"b\"}\r\x00\x0b"), nil, nil).ParsedBody(); got["a"] != "b" {
		t.Fatalf("body %v, want the JSON object", got)
	}
	if got := NewCallbackRequest([]byte("\ufeff{\"a\":\"b\"}"), nil, nil).ParsedBody(); got["a"] == "b" {
		t.Fatalf("a byte order mark must not be trimmed, got %v", got)
	}
}

func TestQueryFromHTTPIsDecodedLikeAForm(t *testing.T) {
	request, err := NewCallbackRequestFromHTTP(&http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/cb", RawQuery: "a=%zz&b=1;c=2&a=second"}, Header: http.Header{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := request.Input(); got["a"] != "%zz" || got["b"] != "1;c=2" {
		t.Fatalf("query %v", got)
	}
}

func TestRepeatedHeadersAreJoined(t *testing.T) {
	request := NewCallbackRequest(nil, http.Header{"X-Webhook-Secret": {"a", "b"}}, nil)
	if got := request.HeaderValue("x-webhook-secret"); got != "a, b" {
		t.Fatalf("HeaderValue = %q, want %q", got, "a, b")
	}
}

func TestOnlyAJSONObjectBodyIsReadAsJSON(t *testing.T) {
	for _, raw := range []string{`["a"]`, `{"a":"b"} trailing`} {
		if _, ok := NewCallbackRequest([]byte(raw), nil, nil).ParsedBody()["a"]; ok {
			t.Fatalf("%q should not decode as a JSON object", raw)
		}
	}
}

func TestEveryErrorIsAPaymentError(t *testing.T) {
	for _, err := range []error{
		&InvalidPaymentDataError{}, &APIError{}, &SignatureVerificationError{}, &ConfigurationError{},
		fmt.Errorf("wrapped: %w", &APIError{Message: "x"}),
	} {
		var paymentErr PaymentError
		if !errors.As(err, &paymentErr) {
			t.Errorf("%T is not a PaymentError", err)
		}
	}
	var paymentErr PaymentError
	if errors.As(errors.New("plain"), &paymentErr) {
		t.Error("a plain error is not a PaymentError")
	}
}

func TestPaymentStatusesListsEveryStatusInOrder(t *testing.T) {
	want := []PaymentStatus{"successful", "pending", "failed", "canceled", "expired", "unknown"}
	got := PaymentStatuses()
	if len(got) != len(want) {
		t.Fatalf("PaymentStatuses() = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PaymentStatuses() = %v, want %v", got, want)
		}
	}
	got[0] = "changed"
	if PaymentStatuses()[0] != StatusSuccessful {
		t.Fatal("PaymentStatuses returns a shared slice")
	}
}
