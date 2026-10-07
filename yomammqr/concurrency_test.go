package yomammqr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestTokenCacheIsSafeForConcurrentUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/token") {
			_, _ = w.Write([]byte(`{"access_token":"token","expires_in":28800}`))
			return
		}
		_, _ = w.Write([]byte(`{"refLabel":"1","paymentStatus":"PENDING","errorCode":null}`))
	}))
	defer server.Close()

	gateway, err := New(Config{MerchantID: "M", ClientID: "c", ClientSecret: "s", WebhookHashKey: "h", BaseURL: server.URL}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

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
