package myanmarpayments

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFormPaymentHTMLEscapesEverything(t *testing.T) {
	payment := FormPayment{Action: `https://pay.test/?a="b"`, Fields: []FormField{{Name: "x", Value: `"><script>alert(1)</script>`}}, Enctype: "multipart/form-data"}
	html := payment.HTML()
	if strings.Contains(html, "<script>alert(1)") || !strings.Contains(html, `enctype="multipart/form-data"`) || !strings.Contains(html, "&#34;&gt;&lt;script&gt;") {
		t.Fatalf("unexpected html %s", html)
	}
}

func TestCallbackRequestParsesJSONAndFormBodies(t *testing.T) {
	jsonRequest := NewCallbackRequest([]byte(`{"amount": 1000, "nested": {"a": "b"}}`), nil, url.Values{"q": {"1"}})
	if jsonRequest.ParsedBody()["amount"] != json.Number("1000") || jsonRequest.Input()["q"] != "1" {
		t.Fatalf("unexpected json parse %v", jsonRequest.Input())
	}

	formRequest := NewCallbackRequest([]byte("decision=ACCEPT&amount=10.50"), nil, url.Values{"decision": {"QUERY"}})
	if formRequest.Input()["decision"] != "ACCEPT" || formRequest.QueryInput()["decision"] != "QUERY" {
		t.Fatalf("unexpected precedence %v %v", formRequest.Input(), formRequest.QueryInput())
	}
}

func TestCallbackRequestFromHTTPKeepsTheBodyReadable(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/cb?x=1", bytes.NewBufferString(`{"a":"b"}`))
	r.Header.Set("X-Webhook-Secret", "shared")

	request, err := NewCallbackRequestFromHTTP(r)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := NewCallbackRequestFromHTTP(r)
	if request.HeaderValue("x-webhook-secret") != "shared" || request.Query.Get("x") != "1" || string(again.Body) != `{"a":"b"}` {
		t.Fatalf("unexpected request %+v", request)
	}
}

func TestCallbackRequestFromHTTPCapsTheBody(t *testing.T) {
	exact := httptest.NewRequest(http.MethodPost, "/cb", bytes.NewReader(bytes.Repeat([]byte("a"), MaxCallbackBodyBytes)))
	if request, err := NewCallbackRequestFromHTTP(exact); err != nil || len(request.Body) != MaxCallbackBodyBytes {
		t.Fatalf("a body of exactly the limit must be read, got %v", err)
	}

	large := httptest.NewRequest(http.MethodPost, "/cb", bytes.NewReader(bytes.Repeat([]byte("a"), MaxCallbackBodyBytes+1)))
	if _, err := NewCallbackRequestFromHTTP(large); !errors.Is(err, ErrCallbackBodyTooLarge) {
		t.Fatalf("expected ErrCallbackBodyTooLarge, got %v", err)
	}
}

func TestCallbackRequestFromHTTPReportsReadErrors(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/cb", nil)
	r.Body = io.NopCloser(failingReader{})
	if _, err := NewCallbackRequestFromHTTP(r); err == nil || errors.Is(err, ErrCallbackBodyTooLarge) {
		t.Fatalf("expected a read error, got %v", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestAcknowledgementIsWritten(t *testing.T) {
	recorder := httptest.NewRecorder()
	ack := DefaultAcknowledgement()
	ack.Body = "success"
	if err := ack.Write(recorder); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || recorder.Body.String() != "success" || recorder.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("unexpected response %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestMemoryTokenCacheExpires(t *testing.T) {
	cache := NewMemoryTokenCache()
	current := time.Unix(0, 0)
	cache.now = func() time.Time { return current }
	cache.Set("k", "v", time.Minute)

	if value, ok := cache.Get("k"); !ok || value != "v" {
		t.Fatal("expected cached value")
	}
	current = current.Add(time.Minute)
	if _, ok := cache.Get("k"); ok {
		t.Fatal("expected expiry")
	}
}

func TestStatusHelpersAndErrors(t *testing.T) {
	if ResolveStatus(map[string]PaymentStatus{"OK": StatusSuccessful}, " OK ") != StatusSuccessful || ResolveStatus(nil, "X") != StatusUnknown {
		t.Fatal("unexpected resolve")
	}
	if StatusPending.IsFinal() || !StatusExpired.IsFinal() {
		t.Fatal("unexpected IsFinal")
	}
	wrapped := &APIError{Message: "x", Err: errors.New("boom")}
	if !errors.Is(wrapped, wrapped.Err) {
		t.Fatal("APIError should unwrap")
	}
	if (QrPayment{QRImage: "abc"}).QRImageDataURI("") != "data:image/png;base64,abc" {
		t.Fatal("unexpected data URI")
	}
}
