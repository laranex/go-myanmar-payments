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

Use this skill when a Go service takes payments through KBZ Pay, Wave Money, AYA Payment Gateway, Yoma MMQR or CyberSource. Start payments and verify callbacks with the module's typed API; never build gateway signatures by hand.

## Install

```bash
go get github.com/laranex/go-myanmar-payments/v4
```

Requires Go 1.22+ and uses only the standard library. Import the root as `myanmarpayments "github.com/laranex/go-myanmar-payments/v4"` and the gateway sub-packages `.../v4/kbzpay`, `wavemoney`, `ayapay`, `yomammqr` and `cybersource`.

## Configure

Each sub-package has a `Config` struct and `ConfigFromEnv(os.Getenv)`, which reads `KBZ_PAY_*`, `WAVE_MONEY_*`, `AYA_PAY_*`, `YOMA_MMQR_*` and `CYBER_SOURCE_*` (the same variables as the PHP packages). The zero value targets the sandbox; set `*_SANDBOX=false` or `Production: true` in production.

```go
kbz, err := kbzpay.New(kbzpay.ConfigFromEnv(os.Getenv), nil)
```

- `kbzpay.New(cfg, client)`, `wavemoney.New(cfg, client)`, `ayapay.New(cfg, client)`, `yomammqr.New(cfg, client, cache)`, `cybersource.New(cfg)`
- `client` is any `myanmarpayments.HTTPDoer` (`*http.Client` works; nil uses `DefaultHTTPClient()`)
- `cache` is any `myanmarpayments.TokenCache` for the Yoma access token (nil uses `NewMemoryTokenCache()`; share a Redis-backed one across processes)
- a missing credential returns `*myanmarpayments.ConfigurationError`

## Use

### Amounts

Amounts are `myanmarpayments.Amount`: `Kyat(1000)` or `ParseAmount("1000.50")` (returns an error; `MustParseAmount` for constants), never `float64`. Only KBZ Pay (up to 2 decimals) and CyberSource accept decimals; Wave, AYA and Yoma take whole kyat. Compare a gateway's amount with `amount.Equals(callback.Amount)` (by value, so `1000` equals `1000.00`). KBZ's optional `TimeoutMinutes` is an `*int` (1 to 120). Invalid data returns `*InvalidPaymentDataError` with `Errors`.

### Start a payment

Fill the gateway's `PaymentData` struct (`wavemoney` takes `[]wavemoney.Item`, `ayapay` an `ayapay.Method`, `cybersource` a `cybersource.TransactionType`), pass the request `ctx` to every network call and act on the typed result:

```go
payment, err := kbz.PWA(ctx, kbzpay.PaymentData{
    OrderID:     "ORDER_1",
    Amount:      myanmarpayments.Kyat(1000),
    CallbackURL: "https://shop.test/kbz/callback",
})
if err != nil {
    return err
}
http.Redirect(w, r, payment.URL, http.StatusFound)
```

- `*RedirectPayment` from `kbz.PWA(ctx, data)` and `wave.Initiate(ctx, &data)`: redirect to `payment.URL`. For Wave, store `data.MerchantReferenceID` (filled by `Initiate` when empty).
- `*FormPayment` from `aya.Initiate(data)` and `cs.Initiate(data)` (no network call): write `payment.HTML()` for an auto-submitting page, or render `Action` and `Fields` yourself.
- `*QrPayment` from `kbz.QR(ctx, data)` (encode `QRString`) and `yoma.Initiate(ctx, data)` (`QRImage` as base64, `QRImageDataURI("image/png")`, `ExpiresAt`, `Reference`). A Yoma QR lives `yomammqr.QRLifetime` (120 seconds); renew it with `yoma.RenewQR(ctx, orderID)`.
- `*AppPayment` from `kbz.App(ctx, data)`: JSON-encode it for the mobile app.

AYA needs a channel: list them with `aya.Services(ctx)` (each `Service` has `Key` and `Supports(method)`), then `aya.Initiate(ayapay.PaymentData{OrderID: "ORDER123", Amount: myanmarpayments.Kyat(1000), Channel: "kbz_pay", Method: ayapay.MethodQR})`.

### Handle the callback

```go
request, err := myanmarpayments.NewCallbackRequestFromHTTP(r)
if err != nil {
    return err
}

callback, err := kbz.HandleCallback(request)
if err != nil {
    var sigErr *myanmarpayments.SignatureVerificationError
    if errors.As(err, &sigErr) {
        http.Error(w, "invalid signature", http.StatusBadRequest)
        return nil
    }
    return err
}

if callback.IsSuccessful() {
    // check order.Amount.Equals(callback.Amount), then fulfill callback.OrderID once
}

return callback.Acknowledgement.Write(w)
```

Check AYA's browser return with `aya.VerifyRedirect(request)`.

### Check status and handle errors

- `kbz.Status(ctx, orderID)`, `aya.Status(ctx, orderID)` and `yoma.Status(ctx, reference)` return `*PaymentStatusResult` with `Status` and `IsSuccessful()`.
- Statuses are `StatusSuccessful`, `StatusPending`, `StatusFailed`, `StatusCanceled`, `StatusExpired` and `StatusUnknown`.
- Gateway failures return `*APIError` (`GatewayCode`, `GatewayMessage`, `HTTPStatus`, `Raw`); `Unwrap` exposes transport errors such as a canceled `ctx`.

## Test your app

- Point the gateway at an `httptest.Server` with the config's URL override (for example `kbzpay.Config{APIURL: server.URL}`), or pass an `HTTPDoer` stub that returns canned responses.
- Replay a stored callback with `myanmarpayments.NewCallbackRequestFromJSON(payload, header)`; it is still signature-checked, so use a payload the gateway actually signed.
- To test your own fulfillment code, build a `myanmarpayments.PaymentCallback{OrderID: "ORDER_1", Status: myanmarpayments.StatusSuccessful}` yourself instead of calling the gateway.

## Avoid

- Fulfilling orders from return pages or query strings; fulfill only from a verified callback or a status check.
- Treating `StatusPending` or `StatusUnknown` as paid.
- Converting amounts through `float64`.
- Reusing a Wave `MerchantReferenceID` (unique per attempt), or calling Yoma `Initiate` twice for the same order (use `RenewQR`).
- Opening a KBZ Pay PWA link outside a phone with the KBZ Pay app.
