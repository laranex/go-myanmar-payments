// Package wavepayminiapp integrates Wave Pay Mini App (Wave Money JS SDK spec v1.8): the
// customer pays inside the WavePay app, on your web page loaded in its webview. Your server
// covers three steps:
//
//  1. AccessToken for the page's WaveJsSDK.authModule.loginWithAccessToken(token).
//  2. Payment signs the object the page passes to WaveJsSDK.paymentModule.makePayment(payload).
//  3. HandleCallback verifies the result Wave posts to your callback URL.
//
// Wave has no status API, so the callback is the only result. Access tokens are cached in a
// TokenCache; pass a shared one so they survive between processes. The cache key is shared with
// the other Laranex SDKs.
//
// The callback carries neither the mini app id nor the callback URL, so HandleCallback also takes
// the callback URL the payment was signed with.
package wavepayminiapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/env"
	"github.com/laranex/go-myanmar-payments/v4/internal/transport"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

// TokenCachePrefix is the prefix of the access token's cache key, the same in every Laranex SDK
// so services in different languages can share one cache. The key ends with the SHA-256 of
// "<base url>|<client id>".
const TokenCachePrefix = "myanmar-payments.wave-pay-mini-app.token."

// TokenPath is where access tokens are requested, under Config.BaseURL.
const TokenPath = "/auth/v1/external-utility/token"

var statuses = map[string]myanmarpayments.PaymentStatus{
	"successful": myanmarpayments.StatusSuccessful,
	"processing": myanmarpayments.StatusPending,
	"failed":     myanmarpayments.StatusFailed,
}

// callbackFields are the callback fields Wave documents. A nested (object or list) value in any
// of them fails verification.
var callbackFields = []string{"merchantRefId", "orderId", "amount", "msisdn", "hashValue", "transactionId", "transactionStatus"}

var wholeNumber = regexp.MustCompile(`^[0-9]+$`)

// Gateway talks to Wave Pay Mini App.
type Gateway struct {
	config    Config
	transport *transport.Client
	signer    Signer
	cache     myanmarpayments.TokenCache

	// tokenTimeout bounds a shared token request, which runs without any one caller's context.
	tokenTimeout time.Duration

	mu         sync.Mutex
	inflight   *tokenCall
	generation uint64
}

// tokenCall is a token request that concurrent callers share.
type tokenCall struct {
	done  chan struct{}
	token string
	err   error
}

// New returns a Gateway. A nil client uses an *http.Client with config.TimeoutSeconds (or
// myanmarpayments.DefaultTimeoutSeconds when it is 0) and a nil cache uses an in-memory cache.
func New(config Config, client myanmarpayments.HTTPDoer, cache myanmarpayments.TokenCache) (*Gateway, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	config.TimeoutSeconds = env.TimeoutSeconds(config.TimeoutSeconds)
	if cache == nil {
		cache = myanmarpayments.NewMemoryTokenCache()
	}
	return &Gateway{
		config:       config,
		transport:    transport.New(client, config.TimeoutSeconds),
		signer:       NewSigner(config.SecretKey),
		cache:        cache,
		tokenTimeout: time.Duration(config.TimeoutSeconds) * time.Second,
	}, nil
}

// Config returns the configuration in use.
func (g *Gateway) Config() Config { return g.config }

// Signer returns the payment and callback hash signer.
func (g *Gateway) Signer() Signer { return g.signer }

// AccessToken returns an access token for the mini app page's
// WaveJsSDK.authModule.loginWithAccessToken(token), from the cache or a form post of the client
// credentials (client_id, client_secret, grant_type=client_credentials) to Config.BaseURL's
// /auth/v1/external-utility/token. It is cached for its expires_in minus a minute (at least a
// minute; an hour when Wave sends none). Never call the token endpoint from the page: it needs
// the client secret.
//
// Concurrent calls share one token request, bounded by the configured timeout; a caller whose
// ctx ends stops waiting without failing the others. A failed request, a response that is not
// JSON or one without access_token returns an *myanmarpayments.APIError.
func (g *Gateway) AccessToken(ctx context.Context) (string, error) {
	if token, ok := g.cache.Get(g.tokenCacheKey()); ok && token != "" {
		return token, nil
	}

	g.mu.Lock()
	call := g.inflight
	if call == nil {
		call = &tokenCall{done: make(chan struct{})}
		g.inflight = call
		generation := g.generation
		go func() {
			fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), g.tokenTimeout)
			defer cancel()
			token, ttl, err := g.fetchToken(fetchCtx)
			g.mu.Lock()
			if err == nil && generation == g.generation {
				g.cache.Set(g.tokenCacheKey(), token, ttl)
			}
			if g.inflight == call {
				g.inflight = nil
			}
			g.mu.Unlock()
			call.token, call.err = token, err
			close(call.done)
		}()
	}
	g.mu.Unlock()

	select {
	case <-call.done:
		return call.token, call.err
	case <-ctx.Done():
		return "", &myanmarpayments.APIError{
			Message: fmt.Sprintf("Could not reach %s%s: %v", g.config.ResolvedBaseURL(), TokenPath, ctx.Err()),
			Err:     ctx.Err(),
		}
	}
}

// ForgetToken drops the cached access token, e.g. after rotating the client secret. A token
// request already in flight is not cached when it completes, and later calls request a new one.
func (g *Gateway) ForgetToken() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.generation++
	g.inflight = nil
	g.cache.Delete(g.tokenCacheKey())
}

// Payment signs a payment for the page's WaveJsSDK.paymentModule.makePayment(payload). No
// request is sent: pass the result's JSON() to the page unchanged. Once data passes validation it
// fills data.MerchantReferenceID with a random 32-character hex id when empty, so store it
// afterwards. Invalid data is returned untouched as an *myanmarpayments.InvalidPaymentDataError.
//
// The hash is HMAC-SHA256 over amount, orderID, miniAppID, merchantRefID and callBackUrl (see
// Signer).
func (g *Gateway) Payment(data *PaymentData) (*myanmarpayments.MiniAppPayment, error) {
	if err := data.Validate(); err != nil {
		return nil, err
	}
	if data.MerchantReferenceID == "" {
		data.MerchantReferenceID = randomReference()
	}

	amount := data.ResolvedAmount()
	items := make([]myanmarpayments.MiniAppItem, len(data.Items))
	for i, item := range data.Items {
		items[i] = myanmarpayments.MiniAppItem{Name: item.Name, Amount: item.Amount}
	}

	return &myanmarpayments.MiniAppPayment{
		OrderID:             data.OrderID,
		Amount:              amount,
		MiniAppID:           g.config.MiniAppID,
		MerchantReferenceID: data.MerchantReferenceID,
		CallbackURL:         data.CallbackURL,
		HashValue:           g.signer.Hash(amount.WholePart(), data.OrderID, g.config.MiniAppID, data.MerchantReferenceID, data.CallbackURL),
		Items:               items,
	}, nil
}

// HandleCallback verifies Wave's callback. Only successful means the customer paid; processing
// is pending and failed failed (matched trimmed and case-insensitively); anything else is
// unknown.
//
// The callback carries neither the mini app id nor the callback URL, so pass the callback URL
// the payment was signed with: normally the URL this callback was posted to, without its query
// string. The mini app id comes from the configuration. A nested value in a documented field, a
// hashValue that is not a string or a wrong hash fails with "Wave Pay Mini App callback hash
// verification failed."; after the hash matches, an empty orderId or merchantRefId or an amount
// that is not a whole number fails with "Wave Pay Mini App callback is malformed.".
// merchantRefId is in Raw. Wave defines no response body: the acknowledgement is an empty 200.
func (g *Gateway) HandleCallback(request *myanmarpayments.CallbackRequest, callbackURL string) (*myanmarpayments.PaymentCallback, error) {
	payload := request.ParsedBody()

	for _, field := range callbackFields {
		if values.IsNested(payload[field]) {
			return nil, &myanmarpayments.SignatureVerificationError{Message: "Wave Pay Mini App callback hash verification failed.", Raw: payload}
		}
	}

	orderID, referenceID, amount := values.Get(payload, "orderId"), values.Get(payload, "merchantRefId"), values.Get(payload, "amount")
	hashValue, isString := payload["hashValue"].(string)
	if !isString || !g.signer.Verify(hashValue, amount, orderID, g.config.MiniAppID, referenceID, callbackURL) {
		return nil, &myanmarpayments.SignatureVerificationError{Message: "Wave Pay Mini App callback hash verification failed.", Raw: payload}
	}

	if orderID == "" || referenceID == "" || !wholeNumber.MatchString(amount) {
		return nil, &myanmarpayments.SignatureVerificationError{Message: "Wave Pay Mini App callback is malformed.", Raw: payload}
	}

	gatewayStatus := values.Trimmed(payload, "transactionStatus")

	return &myanmarpayments.PaymentCallback{
		OrderID:          orderID,
		Status:           myanmarpayments.ResolveStatus(statuses, strings.ToLower(gatewayStatus)),
		GatewayStatus:    gatewayStatus,
		GatewayReference: values.Get(payload, "transactionId"),
		Amount:           amount,
		Raw:              payload,
		Acknowledgement:  myanmarpayments.DefaultAcknowledgement(),
	}, nil
}

func (g *Gateway) fetchToken(ctx context.Context) (string, time.Duration, error) {
	response, err := g.transport.PostForm(ctx, g.config.ResolvedBaseURL()+TokenPath, url.Values{
		"client_id":     {g.config.ClientID},
		"client_secret": {g.config.ClientSecret},
		"grant_type":    {"client_credentials"},
	}, nil)
	if err != nil {
		return "", 0, err
	}

	body := response.JSON()
	token := values.Get(body, "access_token")
	if !response.Successful() || token == "" {
		code := values.Get(body, "error")
		message := values.Get(body, "error_description")
		if message == "" {
			message = values.Get(body, "message")
		}
		text := fmt.Sprintf("Wave Pay Mini App token request failed with HTTP %d.", response.Status)
		if code != "" {
			text = strings.TrimRight(fmt.Sprintf("Wave Pay Mini App token request failed: [%s] %s", code, message), values.ASCIISpace)
		}
		return "", 0, &myanmarpayments.APIError{Message: text, GatewayCode: code, GatewayMessage: message, HTTPStatus: response.Status, Raw: body}
	}

	expiresIn := leadingInt(values.Get(body, "expires_in"))
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	ttl := time.Duration(expiresIn-60) * time.Second
	if ttl < time.Minute {
		ttl = time.Minute
	}

	return token, ttl, nil
}

func (g *Gateway) tokenCacheKey() string {
	sum := sha256.Sum256([]byte(g.config.ResolvedBaseURL() + "|" + g.config.ClientID))
	return TokenCachePrefix + hex.EncodeToString(sum[:])
}

// leadingInt reads the integer digits at the start of text after any leading whitespace, e.g.
// 300 for "300.0" or " 300"; 0 when there are none or they overflow.
func leadingInt(text string) int {
	text = strings.TrimLeft(text, " \t\n\r\v\f")
	end := 0
	for end < len(text) && text[end] >= '0' && text[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(text[:end])
	if err != nil {
		return 0
	}
	return n
}
