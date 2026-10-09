package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

type recorded struct {
	method      string
	path        string
	contentType string
	accept      string
	custom      string
	body        string
}

func server(t *testing.T, status int, body string) (*httptest.Server, *recorded) {
	t.Helper()
	got := &recorded{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*got = recorded{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
			accept:      r.Header.Get("Accept"),
			custom:      r.Header.Get("X-Custom"),
			body:        string(raw),
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(s.Close)
	return s, got
}

func TestNewUsesTheDefaultClientWhenNil(t *testing.T) {
	if client := New(nil); client.Doer == nil {
		t.Fatal("New(nil) left Doer nil")
	}

	custom := &http.Client{}
	if client := New(custom); client.Doer != custom {
		t.Fatal("New(doer) did not keep the given doer")
	}
}

func TestPostJSONSendsACompactBodyWithoutHTMLEscaping(t *testing.T) {
	s, got := server(t, http.StatusOK, `{"result":"SUCCESS","amount":1000.50}`)

	response, err := New(nil).PostJSON(context.Background(), s.URL+"/precreate", map[string]any{"url": "https://shop.test/?a=1&b=2"}, map[string]string{"X-Custom": "yes", "Accept": "text/plain"})
	if err != nil {
		t.Fatal(err)
	}

	if got.method != http.MethodPost || got.path != "/precreate" {
		t.Fatalf("sent %s %s, want POST /precreate", got.method, got.path)
	}
	if got.body != `{"url":"https://shop.test/?a=1&b=2"}` {
		t.Fatalf("body = %q, want compact unescaped JSON", got.body)
	}
	if got.contentType != "application/json" || got.custom != "yes" || got.accept != "text/plain" {
		t.Fatalf("headers = %+v, want JSON content type, the custom header and the overridden Accept", *got)
	}

	if !response.Successful() || response.Status != http.StatusOK {
		t.Fatalf("response %d should be successful", response.Status)
	}
	decoded := response.JSON()
	if decoded["result"] != "SUCCESS" {
		t.Fatalf("JSON()[result] = %v, want SUCCESS", decoded["result"])
	}
	if number, ok := decoded["amount"].(json.Number); !ok || number.String() != "1000.50" {
		t.Fatalf("JSON()[amount] = %#v, want json.Number 1000.50", decoded["amount"])
	}
}

func TestPostJSONRejectsUnencodableData(t *testing.T) {
	_, err := New(nil).PostJSON(context.Background(), "http://unused.test", map[string]any{"bad": make(chan int)}, nil)
	if err == nil || !strings.Contains(err.Error(), "Could not encode the request") {
		t.Fatalf("err = %v, want an encode error", err)
	}
}

func TestPostFormSendsURLEncodedValues(t *testing.T) {
	s, got := server(t, http.StatusBadRequest, "not json")

	response, err := New(nil).PostForm(context.Background(), s.URL+"/pay", url.Values{"amount": {"1000"}, "order id": {"A&B"}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got.contentType != "application/x-www-form-urlencoded" || got.accept != "application/json" {
		t.Fatalf("headers = %+v, want form content type and JSON accept", *got)
	}
	if got.body != "amount=1000&order+id=A%26B" {
		t.Fatalf("body = %q, want the urlencoded form", got.body)
	}
	if response.Successful() {
		t.Fatalf("a %d response must not be successful", response.Status)
	}
	if decoded := response.JSON(); len(decoded) != 0 {
		t.Fatalf("JSON() on a non-JSON body = %v, want an empty map", decoded)
	}
}

func TestResponseJSONHandlesNullAndArrays(t *testing.T) {
	for _, body := range []string{"null", "[1,2]", ""} {
		if decoded := (Response{Body: []byte(body)}).JSON(); len(decoded) != 0 {
			t.Errorf("JSON() of %q = %v, want an empty map", body, decoded)
		}
	}
}

func TestNetworkFailuresBecomeAPIErrors(t *testing.T) {
	s, _ := server(t, http.StatusOK, "{}")
	s.Close()

	_, err := New(nil).PostJSON(context.Background(), s.URL, map[string]any{}, nil)
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T (%v), want *APIError", err, err)
	}
	if !strings.Contains(apiErr.Message, "Could not reach") || apiErr.Err == nil {
		t.Fatalf("APIError = %+v, want a 'Could not reach' message wrapping the cause", *apiErr)
	}
}

func TestInvalidEndpointsBecomeAPIErrors(t *testing.T) {
	_, err := New(nil).PostJSON(context.Background(), "://bad", map[string]any{}, nil)
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) || !strings.Contains(apiErr.Message, "Could not build request") {
		t.Fatalf("err = %v, want a 'Could not build request' APIError", err)
	}
}

func TestCanceledContextStopsTheRequest(t *testing.T) {
	s, _ := server(t, http.StatusOK, "{}")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := New(nil).PostForm(ctx, s.URL, url.Values{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want a wrapped context.Canceled", err)
	}
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("boom") }
func (brokenBody) Close() error             { return nil }

type brokenDoer struct{}

func (brokenDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusBadGateway, Body: brokenBody{}}, nil
}

func TestUnreadableBodiesBecomeAPIErrorsWithTheStatus(t *testing.T) {
	_, err := New(brokenDoer{}).PostJSON(context.Background(), "http://unused.test", map[string]any{}, nil)
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T, want *APIError", err)
	}
	if apiErr.HTTPStatus != http.StatusBadGateway || !strings.Contains(apiErr.Message, "Could not read response") {
		t.Fatalf("APIError = %+v, want status 502 and a 'Could not read response' message", *apiErr)
	}
}

func TestTrimURLRemovesTrailingSlashes(t *testing.T) {
	if got := TrimURL("https://api.test///"); got != "https://api.test" {
		t.Fatalf("TrimURL = %q", got)
	}
	if got := TrimURL("https://api.test"); got != "https://api.test" {
		t.Fatalf("TrimURL without slashes = %q", got)
	}
}
