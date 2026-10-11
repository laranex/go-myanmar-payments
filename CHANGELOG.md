# Changelog

All notable changes to `go-myanmar-payments` will be documented in this file.

## v4.0.0 - Unreleased

Initial release. The version number matches the other Laranex packages (`php-myanmar-payments`, `laravel-myanmar-payments`), so the module path carries the major version: import `github.com/laranex/go-myanmar-payments/v4` and its sub-packages (`.../v4/kbzpay`, `.../v4/wavemoney`, `.../v4/ayapay`, `.../v4/yomammqr`, `.../v4/cybersource`).

### Added
- `payments.New(payments.Config{...}, payments.Options{...})` and `payments.FromEnv(getenv, options)` build each gateway (`KBZPay()`, `WaveMoney()`, `AYAPay()`, `YomaMMQR()`, `CyberSource()`) once, on first use, and are safe for concurrent use.
- Gateways: KBZ Pay (`PWA`, `QR`, `App`, `Status`), Wave Money (`Initiate`), AYA Payment Gateway (`Services`, `Initiate`, `Status`, `VerifyRedirect`), Yoma MMQR (`Initiate`, `RenewQR`, `Status`, `ForgetToken`) and CyberSource Secure Acceptance (`Initiate`); every gateway validates its `PaymentData` and verifies callbacks with `HandleCallback`.
- Exact `Amount` type (`Kyat`, `ParseAmount`, `MustParseAmount`) kept as decimal text, so amounts never pass through a float; JSON marshals as a string and unmarshals from a string or a number.
- A `Config` struct per gateway plus `ConfigFromEnv(getenv)`, which reads the same credential, setting and URL variables as the PHP packages and reports missing values as `ConfigurationError`.
- Result types: `RedirectPayment`, `FormPayment`, `QrPayment` and `AppPayment` for payment flows, `PaymentCallback` (with a gateway-independent `PaymentStatus` and the `Acknowledgement` each gateway expects) for verified callbacks, and `PaymentStatusResult` for status checks.
- Callback, return and cancel URLs only need to be valid absolute http or https URLs; there is no HTTPS-only or port-443 rule (gateways may still require HTTPS in production).
- Wave Money: `Initiate` validates `PaymentData` before filling an empty `MerchantReferenceID`, so invalid data is returned untouched; callbacks fall back to `merchantReferenceId` for the order id when `orderId` is missing, null or empty (same rule as `php-myanmar-payments`).
- CyberSource callbacks only trust signed fields: `decision` and `req_reference_number` must be listed in `signed_field_names`, and the amount and `transaction_id` are read only when signed, and `Raw` keeps only the signed fields plus `signature`, so the signed request form cannot be replayed as a payment result.
- AYA: a `+` in the base64 `payload` that arrived as a space (unencoded return query string) is mapped back before decoding; the checksum is still verified.
- Typed errors: `InvalidPaymentDataError`, `APIError`, `SignatureVerificationError` and `ConfigurationError`.
- `HTTPDoer` and `TokenCache` abstractions so any HTTP client or cache can be plugged in; `MemoryTokenCache` ships with the module.
- Field, amount and currency rules follow each gateway's official documentation.
- Every gateway uses its production endpoints; the URL settings are optional overrides, for example to test against UAT.
- Configuration: the HTTP timeout (`TimeoutSeconds`, `MYANMAR_PAYMENTS_HTTP_TIMEOUT`) is optional and defaults to 30 seconds (`myanmarpayments.DefaultTimeoutSeconds`); a nil HTTP client becomes an `*http.Client` with that timeout, and a client you pass keeps its own. The time to live settings are optional too: KBZ Pay's `TimeToLiveMinutes` (`KBZ_PAY_TIME_TO_LIVE_IN_MINUTES`, 1 to 120) and Wave Money's `TimeToLiveSeconds` (`WAVE_MONEY_TIME_TO_LIVE_IN_SECONDS`, greater than 0). Every other setting of a gateway you use is required, including Wave Money's `MerchantName` and Yoma MMQR's `APIVersion`. A missing setting returns the `ConfigurationError` naming it; a value out of range returns one with `Invalid` set, e.g. `The kbz_pay configuration [time_to_live_in_minutes] must be a whole number from 1 to 120.`
- Time to live per payment: `kbzpay.PaymentData.TimeToLiveMinutes` (`*int`, 1 to 120) and `wavemoney.PaymentData.TimeToLiveSeconds` (`*int`) override the configured value; a payment with neither its own nor a configured time to live returns `*InvalidPaymentDataError` before any request is sent. `wavemoney.PaymentData` also takes an optional `MerchantName`. CyberSource payments take `Currency`, `Locale` and `TransactionType`.
- `Amount.Equals(text)` (by value: `01000`, `1000` and `1000.00` are equal) and `Amount.WholePart()`; `(*kbzpay.Gateway).Signer()` for custom calls; the `PaymentError` interface implemented by all four error types; `PaymentStatuses()` listing every status.
- Yoma MMQR: concurrent calls share one in-flight token request; a caller whose context ends stops waiting without failing the others.
- Behavior is identical to `php-`, `node-` and `python-myanmar-payments`, checked by the shared vectors in `testdata/parity/vectors.json`: signing (a nested value in a signed field fails verification), callback decoding, AYA's base64 rules, the Yoma MMQR token cache key (`myanmar-payments.yoma-mmqr.token.<sha256(baseUrl|clientId)>`), `FormPayment.HTML` and every error message after the `myanmarpayments: ` prefix.
- Standard library only; Go 1.22 or higher.
- Agent skill in `skills/go-myanmar-payments` so coding agents (Claude Code, Codex, Cursor and others) integrate the module correctly; install it with `npx skills add laranex/go-myanmar-payments`.
