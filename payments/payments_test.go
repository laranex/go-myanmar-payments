package payments

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"testing"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/ayapay"
	"github.com/laranex/go-myanmar-payments/v4/cybersource"
	"github.com/laranex/go-myanmar-payments/v4/kbzpay"
	"github.com/laranex/go-myanmar-payments/v4/wavemoney"
	"github.com/laranex/go-myanmar-payments/v4/yomammqr"
)

func configured() Config {
	return Config{
		KBZPay:      &kbzpay.Config{AppID: "kp123", AppKey: "key", MerchantCode: "100001"},
		WaveMoney:   &wavemoney.Config{MerchantID: "merchant", SecretKey: "secret", MerchantName: "Shop"},
		AYAPay:      &ayapay.Config{AppKey: "app-key", AppSecret: "secret"},
		YomaMMQR:    &yomammqr.Config{MerchantID: "M", ClientID: "client", ClientSecret: "secret", WebhookHashKey: "hash"},
		CyberSource: &cybersource.Config{ProfileID: "profile", AccessKey: "access", SecretKey: "secret"},
	}
}

func TestGatewaysAreBuiltOnceAndReused(t *testing.T) {
	gateways := New(configured(), Options{})

	kbz, err := gateways.KBZPay()
	if err != nil {
		t.Fatal(err)
	}
	again, _ := gateways.KBZPay()
	if kbz != again || kbz.Config().AppID != "kp123" {
		t.Fatal("KBZPay is not reused")
	}
	wave, err := gateways.WaveMoney()
	if err != nil || wave.Config().MerchantName != "Shop" {
		t.Fatalf("unexpected Wave Money %v", err)
	}
	aya, err := gateways.AYAPay()
	if err != nil || aya.Config().AppKey != "app-key" {
		t.Fatalf("unexpected AYA %v", err)
	}
	yoma, err := gateways.YomaMMQR()
	if err != nil || yoma.Config().ClientID != "client" {
		t.Fatalf("unexpected Yoma %v", err)
	}
	cyber, err := gateways.CyberSource()
	if err != nil || cyber.Config().ProfileID != "profile" {
		t.Fatalf("unexpected CyberSource %v", err)
	}
}

func TestAMissingGatewayNamesItsFirstKey(t *testing.T) {
	gateways := New(Config{}, Options{})
	cases := map[string]func() error{
		"kbz_pay/app_id":          func() error { _, err := gateways.KBZPay(); return err },
		"wave_money/merchant_id":  func() error { _, err := gateways.WaveMoney(); return err },
		"aya_pay/app_key":         func() error { _, err := gateways.AYAPay(); return err },
		"yoma_mmqr/merchant_id":   func() error { _, err := gateways.YomaMMQR(); return err },
		"cyber_source/profile_id": func() error { _, err := gateways.CyberSource(); return err },
	}
	for want, call := range cases {
		var configErr *myanmarpayments.ConfigurationError
		if err := call(); !errors.As(err, &configErr) || configErr.Gateway+"/"+configErr.Key != want {
			t.Errorf("want ConfigurationError %s, got %v", want, err)
		}
	}
}

func TestFromEnvReadsTheEnvironmentOnFirstUseAndRetriesAfterAFailure(t *testing.T) {
	env := map[string]string{"KBZ_PAY_APP_ID": "kp123", "KBZ_PAY_APP_KEY": "key", "KBZ_PAY_SANDBOX": "false"}
	var mu sync.Mutex
	gateways := FromEnv(func(key string) string {
		mu.Lock()
		defer mu.Unlock()
		return env[key]
	}, Options{HTTPClient: http.DefaultClient})

	var configErr *myanmarpayments.ConfigurationError
	if _, err := gateways.KBZPay(); !errors.As(err, &configErr) || configErr.Key != "merchant_code" {
		t.Fatalf("want missing merchant_code, got %v", err)
	}

	mu.Lock()
	env["KBZ_PAY_MERCHANT_CODE"] = "100001"
	mu.Unlock()
	kbz, err := gateways.KBZPay()
	if err != nil || !kbz.Config().Production || kbz.Config().MerchantCode != "100001" {
		t.Fatalf("unexpected gateway %v", err)
	}
	if _, err := gateways.CyberSource(); !errors.As(err, &configErr) || configErr.Key != "profile_id" {
		t.Fatalf("want missing profile_id, got %v", err)
	}
}

func TestYomaUsesTheSharedTokenCache(t *testing.T) {
	cache := myanmarpayments.NewMemoryTokenCache()
	config := configured()
	gateways := New(config, Options{TokenCache: cache})
	yoma, err := gateways.YomaMMQR()
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256([]byte(yomammqr.SandboxURL + "|client"))
	key := "myanmar-payments.yoma-mmqr.token." + hex.EncodeToString(sum[:])
	cache.Set(key, "cached", 0)
	yoma.ForgetToken()
	if _, ok := cache.Get(key); ok {
		t.Fatal("the Yoma gateway does not use the shared token cache")
	}
}

func TestGatewaysAreSafeForConcurrentUse(t *testing.T) {
	gateways := New(configured(), Options{})
	results := make(chan *kbzpay.Gateway, 50)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kbz, err := gateways.KBZPay()
			if err != nil {
				t.Error(err)
			}
			results <- kbz
		}()
	}
	wg.Wait()
	close(results)
	first := <-results
	for kbz := range results {
		if kbz != first {
			t.Fatal("concurrent calls built more than one gateway")
		}
	}
}
