---
name: go-myanmar-payments
description: >
  Integrate Myanmar payment gateways (KBZ Pay, Wave Money, AYA Pay, Yoma MMQR, CyberSource) in a Go service with github.com/laranex/go-myanmar-payments/v4.
license: MIT
metadata:
  author: Nay Thu Khant
---

# Go Myanmar Payments

## When to use

Use this skill when a Go service takes payments through KBZ Pay, Wave Money, AYA Payment Gateway, Yoma MMQR or CyberSource. Start payments and verify callbacks with the module's typed API; never build gateway signatures by hand. In Goravel, use `github.com/laranex/goravel-myanmar-payments/v4` instead; it wraps this module.

## Install

```bash
go get github.com/laranex/go-myanmar-payments/v4
```

Requires Go 1.22+ and uses only the standard library. Import the root as `myanmarpayments "github.com/laranex/go-myanmar-payments/v4"`, the facade as `.../v4/payments` and the gateway sub-packages `.../v4/kbzpay`, `wavemoney`, `ayapay`, `yomammqr` and `cybersource`.

## Configure

`payments.FromEnv(os.Getenv, payments.Options{})` reads `KBZ_PAY_*`, `WAVE_MONEY_*`, `AYA_PAY_*` (or `AYA_PGW_*`), `YOMA_MMQR_*` and `CYBER_SOURCE_*` (the same variables as the PHP, Node and Python SDKs). The zero value targets the sandbox; set `*_SANDBOX=false` (or `Production: true`) in production.

```go
gateways := payments.FromEnv(os.Getenv, payments.Options{}) // create once, share across requests
kbz, err := gateways.KBZPay() // also WaveMoney(), AYAPay(), YomaMMQR(), CyberSource()
```

- Or build one gateway: `kbzpay.New(kbzpay.Config{AppID: "...", AppKey: "...", MerchantCode: "..."}, nil)` or `kbzpay.New(kbzpay.ConfigFromEnv(os.Getenv), nil)`.
- Or pass the settings directly: `payments.New(payments.Config{KBZPay: &kbzpay.Config{...}}, payments.Options{})`.
- Options: `HTTPClient` is any `myanmarpayments.HTTPDoer` (nil uses `DefaultHTTPClient()`, an `*http.Client` with a 30 second timeout). Yoma and the facade also take a `TokenCache` for the access token (nil uses `NewMemoryTokenCache()`; back it with Redis when you run several processes).
- A missing credential returns `*myanmarpayments.ConfigurationError` (`Gateway`, `Key`).

## Use

### Amounts

Amounts are `myanmarpayments.Kyat(1000)` or `ParseAmount("1000.50")` (`MustParseAmount` for constants); never a `float64`. Only KBZ Pay (up to 2 decimals) and CyberSource accept decimals; Wave, AYA and Yoma take whole kyat. Invalid data returns `*InvalidPaymentDataError` with `Errors` per field, before any request is sent. Compare a gateway's amount by value with `amount.Equals(callback.Amount)`.

### Start a payment

Each gateway takes its `PaymentData` struct (`wavemoney` with `[]wavemoney.Item`, `ayapay` with an `ayapay.Method`, `cybersource` with a `cybersource.TransactionType`; KBZ's optional `TimeoutMinutes` is an `*int`) and returns a typed result:

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

- `*RedirectPayment` (`URL`) from `kbz.PWA(ctx, data)` and `wave.Initiate(ctx, &data)`. Wave fills `data.MerchantReferenceID` when empty; store it.
- `*FormPayment` from `aya.Initiate(data)` and `cs.Initiate(data)` (no network call): write `payment.HTML()` for an auto-submitting page, or render `Action`, `Fields` and `Enctype` yourself.
- `*QrPayment` from `kbz.QR(ctx, data)` (encode `QRString`) and `yoma.Initiate(ctx, data)` (`QRImageDataURI("image/png")`, `ExpiresAt`, `Reference`). A Yoma QR lives `yomammqr.QRLifetime` (120 seconds); renew it with `yoma.RenewQR(ctx, orderID)`.
- `*AppPayment` from `kbz.App(ctx, data)`: JSON-encode it (`orderId`, `orderInfo`, `sign`, `signType`) for your mobile app.

AYA needs a channel: `aya.Services(ctx)` lists `ayapay.Service` entries (`Key`, `Supports(method)`), then `aya.Initiate(ayapay.PaymentData{OrderID: "ORDER123", Amount: myanmarpayments.Kyat(1000), Channel: "kbz_pay", Method: ayapay.MethodQR})`.

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

- Give the package the raw body: `NewCallbackRequestFromHTTP` reads it once and puts it back, so don't call `r.ParseForm()` first.
- Check AYA's browser return with `aya.VerifyRedirect(request)`.
- For production, store the verified callback, acknowledge immediately, then process it once in the background.

### Check status and handle errors

- `kbz.Status(ctx, orderID)`, `aya.Status(ctx, orderID)` and `yoma.Status(ctx, reference)` return `*PaymentStatusResult` with `Status` and `IsSuccessful()`. Wave Money and CyberSource have no status API.
- Statuses: `StatusSuccessful`, `StatusPending`, `StatusFailed`, `StatusCanceled`, `StatusExpired`, `StatusUnknown`; `status.IsFinal()`.
- Gateway failures return `*APIError` (`GatewayCode`, `GatewayMessage`, `HTTPStatus`, `Raw`; `Unwrap` exposes network errors such as a canceled `ctx`). All errors implement `myanmarpayments.PaymentError`.

## Test your app

- Point the gateway at an `httptest.Server` with the config's URL override (`kbzpay.Config{APIURL: server.URL}`), or pass an `HTTPDoer` stub that returns canned responses.
- Replay a stored callback with `myanmarpayments.NewCallbackRequestFromJSON(payload, header)` or `NewCallbackRequest(body, header, query)`; it is still signature-checked.
- To test your own fulfillment code, build `myanmarpayments.PaymentCallback{OrderID: "ORDER_1", Status: myanmarpayments.StatusSuccessful, GatewayStatus: "PAY_SUCCESS"}` yourself.

## Avoid

- Fulfilling orders from return pages or query strings; fulfill only from a verified callback or a status check.
- Treating `StatusPending` or `StatusUnknown` as paid.
- Converting amounts through `float64`; use `ParseAmount()`.
- Building the `CallbackRequest` from `r.Form` or a decoded struct instead of the raw body.
- Reusing a Wave `MerchantReferenceID`, or calling Yoma `Initiate` twice for one order (use `RenewQR`).
- Opening a KBZ Pay PWA link outside a phone with the KBZ Pay app.
