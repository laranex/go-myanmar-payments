package yomammqr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
)

// tokenServer answers token requests after release is closed (or at once when release is nil)
// and counts them.
func tokenServer(t *testing.T, release <-chan struct{}, tokenRequests *int32) *Gateway {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/token") {
			atomic.AddInt32(tokenRequests, 1)
			if release != nil {
				<-release
			}
			_, _ = w.Write([]byte(`{"access_token":"token","expires_in":28800}`))
			return
		}
		_, _ = w.Write([]byte(`{"refLabel":"1","paymentStatus":"PENDING","errorCode":null}`))
	}))
	t.Cleanup(server.Close)

	gateway, err := New(Config{MerchantID: "M", ClientID: "c", ClientSecret: "s", WebhookHashKey: "h", APIVersion: "v1rc", TimeoutSeconds: 30, BaseURL: server.URL}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

func TestTokenCacheIsSafeForConcurrentUse(t *testing.T) {
	var tokenRequests int32
	gateway := tokenServer(t, nil, &tokenRequests)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := gateway.Status(context.Background(), "1"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentCallsShareOneTokenRequest(t *testing.T) {
	var tokenRequests int32
	release := make(chan struct{})
	gateway := tokenServer(t, release, &tokenRequests)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := gateway.Status(context.Background(), "1"); err != nil {
				t.Error(err)
			}
		}()
	}
	// Let every caller reach the shared request before the token arrives.
	for deadline := time.Now().Add(5 * time.Second); atomic.LoadInt32(&tokenRequests) == 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&tokenRequests); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
}

func TestACanceledCallerDoesNotFailTheSharedTokenRequest(t *testing.T) {
	var tokenRequests int32
	release := make(chan struct{})
	gateway := tokenServer(t, release, &tokenRequests)

	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := gateway.Status(ctx, "1")
		first <- err
	}()
	for deadline := time.Now().Add(5 * time.Second); atomic.LoadInt32(&tokenRequests) == 0 && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}

	second := make(chan error, 1)
	go func() {
		_, err := gateway.Status(context.Background(), "1")
		second <- err
	}()

	cancel()
	var apiErr *myanmarpayments.APIError
	if err := <-first; !errors.As(err, &apiErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller: got %v, want an APIError wrapping context.Canceled", err)
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatalf("other caller failed: %v", err)
	}
	if got := atomic.LoadInt32(&tokenRequests); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
}
