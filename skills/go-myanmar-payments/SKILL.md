---
name: go-myanmar-payments
description: >
  Integrate Myanmar payment gateways (KBZ Pay, Wave Money, AYA Pay, Yoma MMQR, CyberSource) in a Go service with github.com/laranex/go-myanmar-payments/v4.
license: MIT
metadata:
  author: Nay Thu Khant
---

# Go Myanmar Payments

Use this skill when a Go service takes payments through KBZ Pay, Wave Money, AYA Payment Gateway, Yoma MMQR or CyberSource.

## Primary Goal

- start payments and verify callbacks with the module's typed API, never with hand-built signatures

## Workflow

### 1. Install and build the gateway

- `go get github.com/laranex/go-myanmar-payments/v4` (Go 1.22+, standard library only); import the root as `myanmarpayments "github.com/laranex/go-myanmar-payments/v4"` and the gateway sub-packages `.../v4/kbzpay`, `wavemoney`, `ayapay`, `yomammqr`, `cybersource`
- each sub-package has a `Config` struct and `ConfigFromEnv(os.Getenv)`, reading the same `KBZ_PAY_*`, `WAVE_MONEY_*`, `AYA_PAY_*`, `YOMA_MMQR_*`, `CYBER_SOURCE_*` variables as the PHP packages; `*_SANDBOX=false` (or `Production: true`) for production
- build with `New`: `kbzpay.New(cfg, client)`, `wavemoney.New(cfg, client)`, `ayapay.New(cfg, client)`, `yomammqr.New(cfg, client, cache)`, `cybersource.New(cfg)`; `client` is any `myanmarpayments.HTTPDoer` (`*http.Client` works; nil uses `DefaultHTTPClient()`), `cache` any `myanmarpayments.TokenCache` (nil uses `NewMemoryTokenCache()`; share a Redis-backed one across processes); a missing credential returns `*ConfigurationError`

### 2. Start a payment

- fill the gateway's `PaymentData` struct (`kbzpay`, `wavemoney` with `[]wavemoney.Item`, `ayapay` with `ayapay.Method`, `yomammqr`, `cybersource` with `cybersource.TransactionType`); bad input returns `*InvalidPaymentDataError` with `Errors`
- amounts are `myanmarpayments.Amount`: `Kyat(1000)` or `ParseAmount("1000.50")` (returns an error; `MustParseAmount` for constants), never floats; only KBZ Pay (≤2 decimals) and CyberSource accept decimals, Wave, AYA and Yoma take whole kyat
- pass the request `ctx` to every network call and act on the typed result:
  - `*RedirectPayment` (`kbz.PWA(ctx, data)`, `wave.Initiate(ctx, &data)`): `http.Redirect(w, r, payment.URL, http.StatusFound)`
  - `*FormPayment` (`aya.Initiate(data)`, `cs.Initiate(data)`, no network call): write `payment.HTML()` (auto-submitting page) or render `Action` and `Fields` yourself
  - `*QrPayment`: KBZ (`kbz.QR`) gives `QRString` to encode; Yoma (`yoma.Initiate`) gives `QRImage` (base64, `QRImageDataURI("image/png")`), `ExpiresAt` and `Reference`
  - `*AppPayment` (`kbz.App`): JSON-encode it for the mobile app
- store the order id; for Wave store `data.MerchantReferenceID` (filled by `Initiate` when empty), for Yoma the QR `Reference`

### 3. Handle the callback

- `request, err := myanmarpayments.NewCallbackRequestFromHTTP(r)` (`NewCallbackRequestFromJSON` to replay a stored payload)
- `callback, err := gateway.HandleCallback(request)` verifies the signature; on failure `err` is `*SignatureVerificationError` (use `errors.As`); AYA's browser return is checked with `aya.VerifyRedirect(request)`
- check `callback.Status` (`StatusSuccessful`, ...) or `IsSuccessful()`, compare `callback.Amount` with the order, make fulfillment idempotent (gateways retry)
- reply with `callback.Acknowledgement.Write(w)`

### 4. Check status and handle errors

- `kbz.Status(ctx, orderID)`, `aya.Status(ctx, orderID)`, `yoma.Status(ctx, reference)` return `*PaymentStatusResult`
- gateway failures return `*APIError` (`GatewayCode`, `GatewayMessage`, `HTTPStatus`, `Raw`; `Unwrap` exposes transport errors such as a canceled `ctx`)

## Gateway Gotchas

- Wave: `MerchantReferenceID` must be unique per attempt; `CallbackURL` must be HTTPS on port 443; pass `*PaymentData`
- KBZ Pay PWA: works only on a phone with the KBZ Pay app, and the Referer must match the URL registered with KBZ; the acknowledgement is a plain `success`
- AYA: pick `Channel` from `aya.Services(ctx)` (`Service.Key`, check `Supports(method)`)
- Yoma: a QR lives 120 seconds (`yomammqr.QRLifetime`); call `Initiate` once per order, then `RenewQR(ctx, orderID)` after expiry

## Examples

- KBZ Pay PWA: `kbz.PWA(ctx, kbzpay.PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/kbz/callback"})` then redirect to `payment.URL`
- AYA checkout: `aya.Initiate(ayapay.PaymentData{OrderID: "ORDER123", Amount: myanmarpayments.Kyat(1000), Channel: "kbz_pay", Method: ayapay.MethodQR})` then write `payment.HTML()`

## Anti-patterns

- do not fulfill from return pages or query strings; fulfill from the verified callback or a status check
- do not convert amounts through `float64` or treat `StatusPending` / `StatusUnknown` as paid
- do not reuse a Wave `MerchantReferenceID` or re-run Yoma `Initiate` for the same order
