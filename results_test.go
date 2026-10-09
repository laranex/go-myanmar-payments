package myanmarpayments

import (
	"encoding/json"
	"errors"
	"net/http"
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
	}
	for want, result := range cases {
		if result.Flow() != want {
			t.Fatalf("%T: got %s, want %s", result, result.Flow(), want)
		}
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
	if invalid.Error() != "myanmarpayments: invalid payment data: A is wrong. B is wrong." {
		t.Fatalf("unexpected message %q", invalid.Error())
	}
	if (&APIError{Message: "KBZ Pay precreate failed."}).Error() != "myanmarpayments: KBZ Pay precreate failed." {
		t.Fatal("unexpected API error message")
	}
	if (&SignatureVerificationError{Message: "bad"}).Error() != "myanmarpayments: bad" {
		t.Fatal("unexpected signature error message")
	}
	if (&ConfigurationError{Gateway: "kbz_pay", Key: "app_key"}).Error() != "myanmarpayments: the kbz_pay configuration is missing [app_key]" {
		t.Fatal("unexpected configuration error message")
	}
	var apiErr *APIError
	if errors.As(errors.New("plain"), &apiErr) {
		t.Fatal("unexpected match")
	}
}

func TestDefaultHTTPClientHasATimeout(t *testing.T) {
	if DefaultHTTPClient().Timeout != 30*time.Second {
		t.Fatal("expected a 30 second timeout")
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
