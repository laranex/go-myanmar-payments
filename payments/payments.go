// Package payments builds every gateway from one configuration (or the environment), like the
// MyanmarPayments facade of the PHP, Node and Python SDKs. Gateways are created on first use and
// reused, so only the gateways you call need to be configured.
//
//	gateways := payments.FromEnv(os.Getenv, payments.Options{})
//	kbz, err := gateways.KBZPay() // *kbzpay.Gateway
//
// It lives in its own package because the gateway packages import the root package.
package payments

import (
	"sync"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/ayapay"
	"github.com/laranex/go-myanmar-payments/v4/cybersource"
	"github.com/laranex/go-myanmar-payments/v4/kbzpay"
	"github.com/laranex/go-myanmar-payments/v4/wavemoney"
	"github.com/laranex/go-myanmar-payments/v4/yomammqr"
)

// Config holds the configuration of every gateway; set only the ones you use. A nil entry
// makes that gateway's method return a *myanmarpayments.ConfigurationError.
type Config struct {
	KBZPay      *kbzpay.Config
	WaveMoney   *wavemoney.Config
	AYAPay      *ayapay.Config
	YomaMMQR    *yomammqr.Config
	CyberSource *cybersource.Config
}

// Options are shared by every gateway the facade builds.
type Options struct {
	// HTTPClient sends the gateways' requests; nil gives each gateway an *http.Client with
	// its config's TimeoutSeconds.
	HTTPClient myanmarpayments.HTTPDoer
	// TokenCache keeps Yoma MMQR's access token; nil uses one MemoryTokenCache.
	TokenCache myanmarpayments.TokenCache
}

// Gateways builds each gateway on first use and returns the same one afterwards. It is safe for
// concurrent use; create one and share it.
type Gateways struct {
	mu      sync.Mutex
	config  func() Config
	options Options

	kbzPay      *kbzpay.Gateway
	waveMoney   *wavemoney.Gateway
	ayaPay      *ayapay.Gateway
	yomaMMQR    *yomammqr.Gateway
	cyberSource *cybersource.Gateway
}

// New returns Gateways for config.
func New(config Config, options Options) *Gateways {
	return newGateways(func() Config { return config }, options)
}

// FromEnv returns Gateways that read every gateway's configuration from environment variables
// (KBZ_PAY_*, WAVE_MONEY_*, AYA_PAY_*, YOMA_MMQR_*, CYBER_SOURCE_* and MYANMAR_PAYMENTS_HTTP_TIMEOUT) through getenv, e.g.
// os.Getenv, when the gateway is first used.
func FromEnv(getenv func(string) string, options Options) *Gateways {
	return newGateways(func() Config {
		kbz := kbzpay.ConfigFromEnv(getenv)
		wave := wavemoney.ConfigFromEnv(getenv)
		aya := ayapay.ConfigFromEnv(getenv)
		yoma := yomammqr.ConfigFromEnv(getenv)
		cyber := cybersource.ConfigFromEnv(getenv)
		return Config{KBZPay: &kbz, WaveMoney: &wave, AYAPay: &aya, YomaMMQR: &yoma, CyberSource: &cyber}
	}, options)
}

func newGateways(config func() Config, options Options) *Gateways {
	if options.TokenCache == nil {
		options.TokenCache = myanmarpayments.NewMemoryTokenCache()
	}
	return &Gateways{config: config, options: options}
}

// KBZPay returns the KBZ Pay gateway, or a ConfigurationError when it is not configured.
func (g *Gateways) KBZPay() (*kbzpay.Gateway, error) {
	return build(g, &g.kbzPay, func(c Config) *kbzpay.Config { return c.KBZPay }, "kbz_pay", "app_id",
		func(c kbzpay.Config) (*kbzpay.Gateway, error) { return kbzpay.New(c, g.options.HTTPClient) })
}

// WaveMoney returns the Wave Money gateway, or a ConfigurationError when it is not configured.
func (g *Gateways) WaveMoney() (*wavemoney.Gateway, error) {
	return build(g, &g.waveMoney, func(c Config) *wavemoney.Config { return c.WaveMoney }, "wave_money", "merchant_id",
		func(c wavemoney.Config) (*wavemoney.Gateway, error) { return wavemoney.New(c, g.options.HTTPClient) })
}

// AYAPay returns the AYA Payment Gateway, or a ConfigurationError when it is not configured.
func (g *Gateways) AYAPay() (*ayapay.Gateway, error) {
	return build(g, &g.ayaPay, func(c Config) *ayapay.Config { return c.AYAPay }, "aya_pay", "app_key",
		func(c ayapay.Config) (*ayapay.Gateway, error) { return ayapay.New(c, g.options.HTTPClient) })
}

// YomaMMQR returns the Yoma MMQR gateway, or a ConfigurationError when it is not configured.
func (g *Gateways) YomaMMQR() (*yomammqr.Gateway, error) {
	return build(g, &g.yomaMMQR, func(c Config) *yomammqr.Config { return c.YomaMMQR }, "yoma_mmqr", "merchant_id",
		func(c yomammqr.Config) (*yomammqr.Gateway, error) {
			return yomammqr.New(c, g.options.HTTPClient, g.options.TokenCache)
		})
}

// CyberSource returns the CyberSource gateway, or a ConfigurationError when it is not configured.
func (g *Gateways) CyberSource() (*cybersource.Gateway, error) {
	return build(g, &g.cyberSource, func(c Config) *cybersource.Config { return c.CyberSource }, "cyber_source", "profile_id",
		cybersource.New)
}

// build returns the cached gateway or creates it. A failure is not cached, so a later call
// tries again (e.g. after the environment is fixed).
func build[C any, G any](g *Gateways, slot **G, pick func(Config) *C, gateway, firstKey string, create func(C) (*G, error)) (*G, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if *slot != nil {
		return *slot, nil
	}
	config := pick(g.config())
	if config == nil {
		return nil, &myanmarpayments.ConfigurationError{Gateway: gateway, Key: firstKey}
	}
	created, err := create(*config)
	if err != nil {
		return nil, err
	}
	*slot = created
	return created, nil
}
