package payments

import (
	"context"
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
	"github.com/laranex/go-myanmar-payments/v4/wavepay"
	"github.com/laranex/go-myanmar-payments/v4/wavepayminiapp"
	"github.com/laranex/go-myanmar-payments/v4/yomammqr"
)

func configured() Config {
	return Config{
		KBZPay: &kbzpay.Config{
			AppID: "kp123", AppKey: "key", MerchantCode: "100001", APIURL: kbzpay.ProductionAPIURL, PWAURL: kbzpay.ProductionPWAURL,
			TimeToLiveMinutes: 15, TimeoutSeconds: 30,
		},
		WavePay: &wavepay.Config{
			MerchantID: "merchant", SecretKey: "secret", MerchantName: "Shop", BaseURL: wavepay.ProductionURL,
			BaseRedirectURL: wavepay.ProductionURL, TimeToLiveSeconds: 300, TimeoutSeconds: 30,
		},
		WavePayMiniApp: &wavepayminiapp.Config{
			ClientID: "mini-client", ClientSecret: "secret", MiniAppID: "MINI456", SecretKey: "secret", BaseURL: wavepayminiapp.ProductionURL,
		},
		AYAPay: &ayapay.Config{AppKey: "app-key", AppSecret: "secret", BaseURL: ayapay.ProductionURL, TimeoutSeconds: 30},
		YomaMMQR: &yomammqr.Config{
			MerchantID: "M", ClientID: "client", ClientSecret: "secret", WebhookHashKey: "hash", APIVersion: "v1rc",
			WebhookSecret: "webhook-secret", BaseURL: yomammqr.ProductionURL, TimeoutSeconds: 30,
		},
		CyberSource: &cybersource.Config{ProfileID: "profile", AccessKey: "access", SecretKey: "secret", BaseURL: cybersource.ProductionURL},
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
	wave, err := gateways.WavePay()
	if err != nil || wave.Config().MerchantName != "Shop" {
		t.Fatalf("unexpected Wave Pay %v", err)
	}
	miniApp, err := gateways.WavePayMiniApp()
	if err != nil || miniApp.Config().MiniAppID != "MINI456" {
		t.Fatalf("unexpected Wave Pay Mini App %v", err)
	}
	if again, _ := gateways.WavePayMiniApp(); again != miniApp {
		t.Fatal("WavePayMiniApp is not reused")
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
		"kbz_pay/app_id":              func() error { _, err := gateways.KBZPay(); return err },
		"wave_pay/merchant_id":        func() error { _, err := gateways.WavePay(); return err },
		"wave_pay_mini_app/client_id": func() error { _, err := gateways.WavePayMiniApp(); return err },
		"aya_pay/app_key":             func() error { _, err := gateways.AYAPay(); return err },
		"yoma_mmqr/merchant_id":       func() error { _, err := gateways.YomaMMQR(); return err },
		"cyber_source/profile_id":     func() error { _, err := gateways.CyberSource(); return err },
	}
	for want, call := range cases {
		var configErr *myanmarpayments.ConfigurationError
		if err := call(); !errors.As(err, &configErr) || configErr.Gateway+"/"+configErr.Key != want {
			t.Errorf("want ConfigurationError %s, got %v", want, err)
		}
	}
}

func TestFromEnvReadsTheEnvironmentOnFirstUseAndRetriesAfterAFailure(t *testing.T) {
	env := map[string]string{
		"KBZ_PAY_APP_ID": "kp123", "KBZ_PAY_APP_KEY": "key", "KBZ_PAY_BASE_URL": kbzpay.ProductionAPIURL,
		"KBZ_PAY_PWA_BASE_REDIRECT_URL": kbzpay.ProductionPWAURL, "KBZ_PAY_TIME_TO_LIVE_IN_MINUTES": "15", "MYANMAR_PAYMENTS_HTTP_TIMEOUT": "30",
	}
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
	if err != nil || kbz.Config().ResolvedAPIURL() != kbzpay.ProductionAPIURL || kbz.Config().MerchantCode != "100001" {
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

	sum := sha256.Sum256([]byte(yomammqr.ProductionURL + "|client"))
	key := "myanmar-payments.yoma-mmqr.token." + hex.EncodeToString(sum[:])
	cache.Set(key, "cached", 0)
	yoma.ForgetToken()
	if _, ok := cache.Get(key); ok {
		t.Fatal("the Yoma gateway does not use the shared token cache")
	}
}

func TestWavePayMiniAppUsesTheSharedTokenCache(t *testing.T) {
	cache := myanmarpayments.NewMemoryTokenCache()
	miniApp, err := New(configured(), Options{TokenCache: cache}).WavePayMiniApp()
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256([]byte(wavepayminiapp.ProductionURL + "|mini-client"))
	key := wavepayminiapp.TokenCachePrefix + hex.EncodeToString(sum[:])
	cache.Set(key, "cached", 0)
	if token, err := miniApp.AccessToken(context.Background()); err != nil || token != "cached" {
		t.Fatalf("token %q, %v; want the shared cache's", token, err)
	}
	miniApp.ForgetToken()
	if _, ok := cache.Get(key); ok {
		t.Fatal("the Wave Pay Mini App gateway does not use the shared token cache")
	}
}

func TestFromEnvReadsTheWavePayMiniAppVariables(t *testing.T) {
	env := map[string]string{
		"WAVE_PAY_MINI_APP_CLIENT_ID": "c", "WAVE_PAY_MINI_APP_CLIENT_SECRET": "s", "WAVE_PAY_MINI_APP_MINI_APP_ID": "m",
		"WAVE_PAY_MINI_APP_SECRET_KEY": "k", "WAVE_PAY_MINI_APP_BASE_URL": "https://preprodnewapi.wavemoney.io/",
	}
	miniApp, err := FromEnv(func(key string) string { return env[key] }, Options{}).WavePayMiniApp()
	if err != nil || miniApp.Config().MiniAppID != "m" || miniApp.Config().ResolvedBaseURL() != "https://preprodnewapi.wavemoney.io" {
		t.Fatalf("unexpected gateway %v", err)
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
