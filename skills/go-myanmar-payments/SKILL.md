---
name: go-myanmar-payments
description: >
  Integrate Myanmar payment gateways (KBZ Pay, Wave Pay, Wave Pay Mini App, AYA Pay, Yoma MMQR, CyberSource) in a Go service with github.com/laranex/go-myanmar-payments/v4.
license: MIT
metadata:
  author: Nay Thu Khant
---

# Go Myanmar Payments

## When to use

Use this skill when a Go service takes payments through KBZ Pay, Wave Pay, Wave Pay Mini App (a web page running inside the WavePay app), AYA Payment Gateway, Yoma MMQR or CyberSource. Start payments and verify callbacks with the module's typed API; never build gateway signatures by hand. In Goravel, use `github.com/laranex/goravel-myanmar-payments/v4` instead; it wraps this module.

## Install

```bash
go get github.com/laranex/go-myanmar-payments/v4
```

Requires Go 1.22+ and uses only the standard library. Import the root as `myanmarpayments "github.com/laranex/go-myanmar-payments/v4"`, the facade as `.../v4/payments` and the gateway sub-packages `.../v4/kbzpay`, `wavepay`, `wavepayminiapp`, `ayapay`, `yomammqr` and `cybersource`.

## Configure

`payments.FromEnv(os.Getenv, payments.Options{})` reads `KBZ_PAY_*`, `WAVE_PAY_*`, `WAVE_PAY_MINI_APP_*`, `AYA_PAY_*`, `YOMA_MMQR_*` and `CYBER_SOURCE_*` (the same variables as the PHP, Node and Python SDKs), plus `MYANMAR_PAYMENTS_HTTP_TIMEOUT`.

- Every setting of a gateway you use is required and has no default, except the HTTP timeout (`TimeoutSeconds`, `MYANMAR_PAYMENTS_HTTP_TIMEOUT`, not used by CyberSource): 0 or unset means 30 seconds (`myanmarpayments.DefaultTimeoutSeconds`); a negative or non-whole value, or one above 2147483647, is rejected. The default client never follows redirects.
- URLs: `KBZ_PAY_BASE_URL`, `KBZ_PAY_PWA_BASE_REDIRECT_URL`, `WAVE_PAY_BASE_URL`, `WAVE_PAY_BASE_REDIRECT_URL`, `WAVE_PAY_MINI_APP_BASE_URL`, `AYA_PAY_BASE_URL`, `YOMA_MMQR_BASE_URL`, `CYBER_SOURCE_BASE_URL`, set to production's or the gateway's UAT ones. The `Production*` constants (e.g. `kbzpay.ProductionAPIURL`, `wavepay.ProductionURL`, `wavepayminiapp.ProductionURL`) name production's; they are not defaults.
- Time to live: `KBZ_PAY_TIME_TO_LIVE_IN_MINUTES` (`TimeToLiveMinutes`, 1 to 120) and `WAVE_PAY_TIME_TO_LIVE_IN_SECONDS` (`TimeToLiveSeconds`, greater than 0); a payment's own value overrides them.
- Also required: `WAVE_PAY_MERCHANT_NAME`, `YOMA_MMQR_API_VERSION` (e.g. `v1rc`), `YOMA_MMQR_WEBHOOK_SECRET` (every Yoma callback must carry it in `X-Webhook-Secret`) and Wave Pay Mini App's `WAVE_PAY_MINI_APP_CLIENT_ID`, `WAVE_PAY_MINI_APP_CLIENT_SECRET`, `WAVE_PAY_MINI_APP_MINI_APP_ID` and `WAVE_PAY_MINI_APP_SECRET_KEY`.

```go
gateways := payments.FromEnv(os.Getenv, payments.Options{}) // create once, share across requests
kbz, err := gateways.KBZPay() // also WavePay(), WavePayMiniApp(), AYAPay(), YomaMMQR(), CyberSource()
```

- Or build one gateway: `kbzpay.New(kbzpay.ConfigFromEnv(os.Getenv), nil)` or `kbzpay.New(kbzpay.Config{AppID: "...", AppKey: "...", MerchantCode: "...", APIURL: kbzpay.ProductionAPIURL, PWAURL: kbzpay.ProductionPWAURL, TimeToLiveMinutes: 15}, nil)`.
- Or pass the settings directly: `payments.New(payments.Config{KBZPay: &kbzpay.Config{...}}, payments.Options{})`.
- Options: `HTTPClient` is any `myanmarpayments.HTTPDoer` (nil uses an `*http.Client` with the config's `TimeoutSeconds`, 30 by default; a client you pass keeps its own timeout). Yoma, Wave Pay Mini App (`wavepayminiapp.New(config, client, cache)`) and the facade also take a `TokenCache` for the access token (nil uses `NewMemoryTokenCache()`; back it with Redis when you run several processes).
- A missing or blank setting returns `*myanmarpayments.ConfigurationError` (`Gateway`, `Key`), e.g. `The wave_pay configuration is missing [base_redirect_url].`; a value out of range sets `Invalid`, e.g. `The kbz_pay configuration [time_to_live_in_minutes] must be a whole number from 1 to 120.`

## Use

### Amounts

Amounts are `myanmarpayments.Kyat(1000)` or `ParseAmount("1000.50")` (`MustParseAmount` for constants); never a `float64`. Only KBZ Pay (up to 2 decimals) and CyberSource accept decimals; Wave Pay, Wave Pay Mini App, AYA and Yoma take whole kyat. Invalid data returns `*InvalidPaymentDataError` with `Errors` per field, before any request is sent (a negative `Kyat(-1)` gives `The amount field must not be negative.`). Callback, return and cancel URLs must be ASCII `http://` or `https://` URLs with a valid host name (no spaces, `_` or non-ASCII letters; use the `xn--` form). Compare a gateway's amount by value with `amount.Equals(callback.Amount)`.

### Start a payment

Each gateway takes its `PaymentData` struct (`wavepay` with `[]wavepay.Item`, `ayapay` with an `ayapay.Method`, `cybersource` with the required `Currency` (e.g. `MMK`), `Locale` (e.g. `en-us`) and `TransactionType` (e.g. `cybersource.Sale`); KBZ's optional `TimeToLiveMinutes` is an `*int`, 1 to 120) and returns a typed result:

```go
payment, err := kbz.PWA(ctx, kbzpay.PaymentData{
    OrderID:     "ORDER_1",
    Amount:      myanmarpayments.Kyat(1000),
    CallbackURL: "https://shop.test/payments/kbz/callback",
})
if err != nil {
    return err
}
http.Redirect(w, r, payment.URL, http.StatusFound)
```

- `*RedirectPayment` (`URL`) from `kbz.PWA(ctx, data)` and `wave.Initiate(ctx, &data)`. Wave fills `data.MerchantReferenceID` when empty; store it. Wave's `OrderID` and `MerchantReferenceID` may only hold letters, numbers, `_` and `-` (at most 100 characters); `CallbackURL`, `ReturnURL` and `Description` take at most 255. Wave's optional `MerchantName` (`string`, empty uses the config) and `TimeToLiveSeconds` (`*int`, nil uses the config, 0 is rejected) override the configured values for that payment only, and `AdditionalField3` to `AdditionalField5` (at most 50 characters each, sent only when non-empty) carry extra information Wave echoes in the callback.
- `*FormPayment` from `aya.Initiate(data)` and `cs.Initiate(data)` (no network call): write `payment.HTML()` for an auto-submitting page (`payment.HTMLWithNonce(nonce)` adds your Content-Security-Policy nonce to its inline script), or render `Action`, `Fields` and `Enctype` yourself.
- `*QrPayment` from `kbz.QR(ctx, data)` (encode `QRString`) and `yoma.Initiate(ctx, data)` (`QRImageDataURI("image/png")`, `ExpiresAt`, `Reference`). A Yoma QR lives `yomammqr.QRLifetime` (120 seconds); renew it with `yoma.RenewQR(ctx, orderID)`.
- `*AppPayment` from `kbz.App(ctx, data)`: JSON-encode it (`orderId`, `orderInfo`, `sign`, `signType`) for your mobile app.
- `*MiniAppPayment` from `miniApp.Payment(&data)` (no network call): write `payment.JSON()` to your mini app page, which passes it unchanged to `WaveJsSDK.paymentModule.makePayment(payload)`. It is exactly Wave's object: `amount`, `miniAppID`, `merchantRefID`, `orderID`, `callBackUrl`, `hashValue`, `items` (amounts as JSON integers). `json.Marshal` escapes `<`, `>` and `&`; use `JSON()` or an encoder with `SetEscapeHTML(false)`.

Wave Pay Mini App runs your web page inside the WavePay app. Your server gives the page `miniApp.AccessToken(ctx)` (cached; the page calls `WaveJsSDK.authModule.loginWithAccessToken(token)`; never request the token from the browser, it needs the client secret), then signs the payment:

```go
miniApp, err := gateways.WavePayMiniApp()
if err != nil {
    return err
}
data := wavepayminiapp.PaymentData{
    OrderID:     "ORDER_1",
    CallbackURL: "https://shop.test/payments/wave-pay-mini-app/callback",
    Items:       []wavepayminiapp.Item{{Name: "Ticket", Amount: myanmarpayments.Kyat(5000)}}, // Amount defaults to the items' sum
}
payment, err := miniApp.Payment(&data)
if err != nil {
    return err
}
io.WriteString(w, payment.JSON()) // the page calls WaveJsSDK.paymentModule.makePayment(payload)
```

`Payment` fills `data.MerchantReferenceID` when empty (32 hex characters); store it with the order.

AYA needs a channel: `aya.Services(ctx)` lists `ayapay.Service` entries (`Key`, `Supports(method)`), then `aya.Initiate(ayapay.PaymentData{OrderID: "ORDER123", Amount: myanmarpayments.Kyat(1000), Channel: "kbz_pay", Method: ayapay.MethodQR})`. AYA's `OrderID`, `Description` and `UserRefs` must not contain `:` (a user ref's error is under `userRefs`).

### Handle the callback

Build a `CallbackRequest` from the raw request, verify it, then reply:

```go
request, err := myanmarpayments.NewCallbackRequestFromHTTP(r)
if err != nil {
    return err
}
callback, err := kbz.HandleCallback(request)
var sigErr *myanmarpayments.SignatureVerificationError
if errors.As(err, &sigErr) {
    http.Error(w, "invalid signature", http.StatusBadRequest)
    return nil
} else if err != nil {
    return err
}
if callback.IsSuccessful() {
    // compare callback.Amount with the order, then fulfill callback.OrderID once
}
return callback.Acknowledgement.Write(w)
```

- Give the package the raw body: `NewCallbackRequestFromHTTP` reads it once and puts it back, so don't call `r.ParseForm()` first. Bodies over `myanmarpayments.MaxCallbackBodyBytes` (64 KiB) return an error wrapping `ErrCallbackBodyTooLarge`.
- A bad or wrong signature fails with `… signature`/`hash`/`checksum verification failed.`; a correctly signed callback with bad values fails with `Wave Pay callback is malformed.`, `AYA Pay {callback|redirect|enquiry response} payload is malformed.` or `CyberSource callback is malformed.`. Both are `*SignatureVerificationError`.
- Check AYA's browser return with `aya.VerifyRedirect(request)`.
- Wave Pay Mini App callbacks carry neither the mini app id nor the callback URL, so pass the callback URL the payment was signed with: `miniApp.HandleCallback(request, "https://shop.test/payments/wave-pay-mini-app/callback")`. `successful` is `StatusSuccessful`, `processing` is `StatusPending`, `failed` is `StatusFailed`; `merchantRefId` is in `callback.Raw`. Wave expects an empty 200. A correctly hashed callback with bad values fails with `Wave Pay Mini App callback is malformed.`.
- Yoma MMQR callbacks need the `X-Webhook-Secret` header (`YOMA_MMQR_WEBHOOK_SECRET`) and carry no amount: confirm with `yoma.Status(ctx, reference)` and trust your stored amount.
- For production, store the verified callback, acknowledge immediately, then process it once in the background.

### Check status and handle errors

- `kbz.Status(ctx, orderID)`, `aya.Status(ctx, orderID)` and `yoma.Status(ctx, reference)` return `*PaymentStatusResult` with `Status` and `IsSuccessful()`. Wave Pay, Wave Pay Mini App and CyberSource have no status API.
- Statuses: `StatusSuccessful`, `StatusPending`, `StatusFailed`, `StatusCanceled`, `StatusExpired`, `StatusUnknown`; `status.IsFinal()`.
- Gateway failures return `*APIError` (`GatewayCode`, `GatewayMessage`, `HTTPStatus`, `Raw`; `Unwrap` exposes network errors such as a canceled `ctx`). All errors implement `myanmarpayments.PaymentError`.

## Test your app

- Point the gateway at an `httptest.Server` by setting the config's URL to it (`config.APIURL = server.URL`), or pass an `HTTPDoer` stub that returns canned responses.
- Replay a stored callback with `myanmarpayments.NewCallbackRequestFromJSON(payload, header)` or `NewCallbackRequest(body, header, query)`; it is still signature-checked.
- To test your own fulfillment code, build `myanmarpayments.PaymentCallback{OrderID: "ORDER_1", Status: myanmarpayments.StatusSuccessful, GatewayStatus: "PAY_SUCCESS"}` yourself.

## Avoid

- Fulfilling orders from return pages or query strings; fulfill only from a verified callback or a status check.
- Treating `StatusPending` or `StatusUnknown` as paid.
- Converting amounts through `float64`; use `ParseAmount()`.
- Building the `CallbackRequest` from `r.Form` or a decoded struct instead of the raw body.
- Requesting the Wave Pay Mini App access token or computing its hash in the browser (both need secrets).
- Reusing a Wave `MerchantReferenceID`, or calling Yoma `Initiate` twice for one order (use `RenewQR`).
- Opening a KBZ Pay PWA link outside a phone with the KBZ Pay app.
