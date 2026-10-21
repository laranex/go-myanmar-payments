package wavepayminiapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/testutil"
)

const callbackURL = "https://shop.test/wave-pay-mini-app/callback"

func newGateway(t *testing.T, server *testutil.Server, cache myanmarpayments.TokenCache) *Gateway {
	t.Helper()
	config := testConfig()
	if server != nil {
		config.BaseURL = server.URL + "/"
	}
	gateway, err := New(config, nil, cache)
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

func hmacHex(message string) string {
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// callback returns a signed callback payload with overrides applied, hashed for signedURL.
func callback(overrides map[string]any, signedURL string) map[string]any {
	payload := map[string]any{"merchantRefId": "ref-1", "orderId": "ORDER_1", "amount": "5000", "msisdn": "9791009039", "transactionId": "T1", "transactionStatus": "successful"}
	for key, value := range overrides {
		payload[key] = value
	}
	payload["hashValue"] = hmacHex(payload["amount"].(string) + payload["orderId"].(string) + "MINI456" + payload["merchantRefId"].(string) + signedURL)
	return payload
}

func handle(t *testing.T, payload map[string]any) (*myanmarpayments.PaymentCallback, error) {
	t.Helper()
	request, err := myanmarpayments.NewCallbackRequestFromJSON(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	return newGateway(t, nil, nil).HandleCallback(request, callbackURL)
}

func TestPaymentSignsTheMakePaymentPayloadWithoutCallingWave(t *testing.T) {
	server := testutil.NewServer(t)
	data := &PaymentData{
		OrderID: "ORDER123", CallbackURL: "https://yourdomain.com/callback", MerchantReferenceID: "MERCHANT789",
		Items: []Item{{Name: "Ticket", Amount: myanmarpayments.Kyat(3000)}, {Name: "Snack", Amount: myanmarpayments.MustParseAmount("2000")}},
	}
	payment, err := newGateway(t, server, nil).Payment(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(server.Requests) != 0 {
		t.Fatalf("Payment sent %d requests", len(server.Requests))
	}
	if payment.Flow() != myanmarpayments.FlowMiniApp || payment.OrderID != "ORDER123" || payment.Amount.String() != "5000" ||
		payment.MiniAppID != "MINI456" || payment.MerchantReferenceID != "MERCHANT789" || payment.CallbackURL != "https://yourdomain.com/callback" {
		t.Fatalf("unexpected payment %+v", payment)
	}
	if payment.HashValue != hmacHex("5000ORDER123MINI456MERCHANT789https://yourdomain.com/callback") {
		t.Fatalf("unexpected hash %s", payment.HashValue)
	}
	want := `{"amount":5000,"miniAppID":"MINI456","merchantRefID":"MERCHANT789","orderID":"ORDER123","callBackUrl":"https://yourdomain.com/callback","hashValue":"` +
		payment.HashValue + `","items":[{"name":"Ticket","amount":3000},{"name":"Snack","amount":2000}]}`
	if payment.JSON() != want {
		t.Fatalf("json\n got %s\nwant %s", payment.JSON(), want)
	}
}

func TestPaymentGeneratesAMerchantReferenceOnlyForValidData(t *testing.T) {
	gateway := newGateway(t, nil, nil)
	first := &PaymentData{OrderID: "ORDER_1", CallbackURL: callbackURL, Items: []Item{{Name: "A", Amount: myanmarpayments.Kyat(1000)}}}
	second := *first
	second.Items = []Item{{Name: "A", Amount: myanmarpayments.Kyat(1000)}}
	if _, err := gateway.Payment(first); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.Payment(&second); err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`^[0-9a-f]{32}$`)
	if !pattern.MatchString(first.MerchantReferenceID) || !pattern.MatchString(second.MerchantReferenceID) || first.MerchantReferenceID == second.MerchantReferenceID {
		t.Fatalf("unexpected references %q %q", first.MerchantReferenceID, second.MerchantReferenceID)
	}

	invalid := &PaymentData{OrderID: "", CallbackURL: callbackURL, Items: []Item{{Name: "A", Amount: myanmarpayments.Kyat(1000)}}}
	var invalidErr *myanmarpayments.InvalidPaymentDataError
	if _, err := gateway.Payment(invalid); !errors.As(err, &invalidErr) || invalidErr.Errors["orderId"] != "The orderId field is required." || invalid.MerchantReferenceID != "" {
		t.Fatalf("unexpected result %v %q", err, invalid.MerchantReferenceID)
	}
}

func TestResolvedAmountIsTheExactSumOfTheItems(t *testing.T) {
	data := PaymentData{Items: []Item{{Name: "A", Amount: myanmarpayments.MustParseAmount("99999999999999999999")}, {Name: "B", Amount: myanmarpayments.Kyat(1)}}}
	if got := data.ResolvedAmount().String(); got != "100000000000000000000" {
		t.Fatalf("sum %s", got)
	}
	data.Amount = myanmarpayments.Kyat(7)
	if got := data.ResolvedAmount().String(); got != "7" {
		t.Fatalf("amount %s", got)
	}
	if (PaymentData{}).ResolvedAmount().IsSet() || (PaymentData{Items: []Item{{Name: "A"}}}).ResolvedAmount().IsSet() {
		t.Fatal("an unset or missing item amount must leave the total unset")
	}
}

func TestAccessTokenPostsTheClientCredentialsAndCachesTheToken(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Body: map[string]any{"token_type": "Bearer", "access_token": "token-1", "expires_in": 300}})
	cache := myanmarpayments.NewMemoryTokenCache()
	gateway := newGateway(t, server, cache)

	for range 2 {
		if token, err := gateway.AccessToken(context.Background()); err != nil || token != "token-1" {
			t.Fatalf("token %q, %v", token, err)
		}
	}
	if len(server.Requests) != 1 {
		t.Fatalf("%d token requests, want 1", len(server.Requests))
	}
	request := server.Last(t)
	form, _ := url.ParseQuery(string(request.Body))
	if request.Method != http.MethodPost || request.Path != TokenPath || request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" ||
		request.Header.Get("Accept") != "application/json" || form.Get("client_id") != "client" || form.Get("client_secret") != "client-secret" || form.Get("grant_type") != "client_credentials" {
		t.Fatalf("unexpected token request %+v", request)
	}

	sum := sha256.Sum256([]byte(server.URL + "|client"))
	if token, ok := cache.Get(TokenCachePrefix + hex.EncodeToString(sum[:])); !ok || token != "token-1" {
		t.Fatal("the token is not cached under the shared key")
	}
}

func TestAccessTokenReportsAFailedRequest(t *testing.T) {
	server := testutil.NewServer(t, testutil.Reply{Status: http.StatusUnauthorized, Body: map[string]any{"error": "invalid_client", "error_description": "Bad client"}})
	_, err := newGateway(t, server, nil).AccessToken(context.Background())
	var apiErr *myanmarpayments.APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "Wave Pay Mini App token request failed: [invalid_client] Bad client" ||
		apiErr.GatewayCode != "invalid_client" || apiErr.GatewayMessage != "Bad client" || apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestAccessTokenReportsANetworkError(t *testing.T) {
	config := testConfig()
	config.BaseURL = "http://127.0.0.1:1"
	gateway, err := New(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var apiErr *myanmarpayments.APIError
	if _, err := gateway.AccessToken(context.Background()); !errors.As(err, &apiErr) || apiErr.Err == nil {
		t.Fatalf("expected a network APIError, got %v", err)
	}
}

func TestConcurrentCallsShareOneTokenRequestAndForgetTokenDropsAnInflightOne(t *testing.T) {
	var requests int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","expires_in":300}`))
	}))
	t.Cleanup(server.Close)
	config := testConfig()
	config.BaseURL = server.URL
	cache := myanmarpayments.NewMemoryTokenCache()
	gateway, err := New(config, nil, cache)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if token, err := gateway.AccessToken(context.Background()); err != nil || token != "token" {
				t.Errorf("token %q, %v", token, err)
			}
		}()
	}
	for deadline := time.Now().Add(5 * time.Second); atomic.LoadInt32(&requests) == 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	gateway.ForgetToken()
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("%d token requests, want 1", got)
	}
	if _, ok := cache.Get(gateway.tokenCacheKey()); ok {
		t.Fatal("a token fetched before ForgetToken was cached")
	}
}

func TestAccessTokenStopsWaitingWhenTheContextEnds(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte(`{"access_token":"token"}`))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	config := testConfig()
	config.BaseURL = server.URL
	gateway, err := New(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var apiErr *myanmarpayments.APIError
	if _, err := gateway.AccessToken(ctx); !errors.As(err, &apiErr) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the context error, got %v", err)
	}
}

func TestHandleCallbackMapsTheStatus(t *testing.T) {
	for status, want := range map[string]myanmarpayments.PaymentStatus{
		"successful":        myanmarpayments.StatusSuccessful,
		"PROCESSING":        myanmarpayments.StatusPending,
		" failed ":          myanmarpayments.StatusFailed,
		"PAYMENT_COMPLETED": myanmarpayments.StatusUnknown,
	} {
		result, err := handle(t, callback(map[string]any{"transactionStatus": status}, callbackURL))
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if result.Status != want || result.OrderID != "ORDER_1" || result.GatewayReference != "T1" || result.Amount != "5000" ||
			result.Raw["merchantRefId"] != "ref-1" || result.Acknowledgement.Status != http.StatusOK || result.Acknowledgement.Body != "" {
			t.Fatalf("%s: unexpected callback %+v", status, result)
		}
	}
}

func TestHandleCallbackRejectsAnotherCallbackURL(t *testing.T) {
	var sigErr *myanmarpayments.SignatureVerificationError
	if _, err := handle(t, callback(nil, "https://shop.test/other")); !errors.As(err, &sigErr) || sigErr.Message != "Wave Pay Mini App callback hash verification failed." {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestHandleCallbackRejectsMalformedValuesAfterTheHashMatches(t *testing.T) {
	for _, overrides := range []map[string]any{{"amount": "50.5"}, {"orderId": ""}, {"merchantRefId": ""}} {
		var sigErr *myanmarpayments.SignatureVerificationError
		if _, err := handle(t, callback(overrides, callbackURL)); !errors.As(err, &sigErr) || sigErr.Message != "Wave Pay Mini App callback is malformed." {
			t.Fatalf("%v: unexpected error %v", overrides, err)
		}
	}
}

func TestHandleCallbackRejectsNestedValuesAndAMissingHash(t *testing.T) {
	nested := callback(nil, callbackURL)
	nested["msisdn"] = []any{"9791009039"}
	missing := callback(nil, callbackURL)
	delete(missing, "hashValue")
	for _, payload := range []map[string]any{nested, missing} {
		var sigErr *myanmarpayments.SignatureVerificationError
		if _, err := handle(t, payload); !errors.As(err, &sigErr) || sigErr.Message != "Wave Pay Mini App callback hash verification failed." {
			t.Fatalf("%v: unexpected error %v", payload, err)
		}
	}
}

func TestSignerVerifiesInConstantTimeIgnoringCase(t *testing.T) {
	signer := newGateway(t, nil, nil).Signer()
	hash := signer.Hash("5000", "ORDER123", "MINI456", "MERCHANT789", "https://yourdomain.com/callback")
	if !signer.Verify(hash, "5000", "ORDER123", "MINI456", "MERCHANT789", "https://yourdomain.com/callback") ||
		!signer.Verify(regexp.MustCompile(`[a-f]`).ReplaceAllStringFunc(hash, func(s string) string { return string(s[0] - 32) }), "5000", "ORDER123", "MINI456", "MERCHANT789", "https://yourdomain.com/callback") ||
		signer.Verify(hash, "5001", "ORDER123", "MINI456", "MERCHANT789", "https://yourdomain.com/callback") {
		t.Fatal("unexpected verification")
	}
}
