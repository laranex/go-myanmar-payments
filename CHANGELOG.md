# Changelog

All notable changes to `go-myanmar-payments` will be documented in this file.

## v4.0.0 - Unreleased

Initial release. The version number matches the other Laranex packages (`php-myanmar-payments`, `laravel-myanmar-payments`), so the module path carries the major version: import `github.com/laranex/go-myanmar-payments/v4` and its sub-packages (`.../v4/kbzpay`, `.../v4/wavemoney`, `.../v4/ayapay`, `.../v4/yomammqr`, `.../v4/cybersource`).

### Added
- Gateways: KBZ Pay (`PWA`, `QR`, `App`, `Status`), Wave Money (`Initiate`), AYA Payment Gateway (`Services`, `Initiate`, `Status`, `VerifyRedirect`), Yoma MMQR (`Initiate`, `RenewQR`, `Status`, `ForgetToken`) and CyberSource Secure Acceptance (`Initiate`); every gateway validates its `PaymentData` and verifies callbacks with `HandleCallback`.
- Exact `Amount` type (`Kyat`, `ParseAmount`, `MustParseAmount`) kept as decimal text, so amounts never pass through a float; JSON marshals as a string and unmarshals from a string or a number.
- A `Config` struct per gateway plus `ConfigFromEnv(getenv)`, which reads the same `*_SANDBOX`, credential and URL variables as the PHP packages and reports missing values as `ConfigurationError`.
- Result types: `RedirectPayment`, `FormPayment`, `QrPayment` and `AppPayment` for payment flows, `PaymentCallback` (with a gateway-independent `PaymentStatus` and the `Acknowledgement` each gateway expects) for verified callbacks, and `PaymentStatusResult` for status checks.
- Callback, return and cancel URLs only need to be valid absolute http or https URLs; there is no HTTPS-only or port-443 rule (gateways may still require HTTPS in production).
- Wave Money's sandbox is `https://preprodpayments.wavemoney.io:8107` (`wavemoney.SandboxURL`), with checkout at `https://preprodpayments.wavemoney.io/authenticate` (`wavemoney.SandboxAuthenticateURL`).
- Wave Money: `Initiate` validates `PaymentData` before filling an empty `MerchantReferenceID`, so invalid data is returned untouched; callbacks fall back to `merchantReferenceId` for the order id when `orderId` is missing, null or empty (same rule as `php-myanmar-payments`).
- CyberSource callbacks only trust signed fields: `decision` and `req_reference_number` must be listed in `signed_field_names`, and the amount and `transaction_id` are read only when signed, and `Raw` keeps only the signed fields plus `signature`, so the signed request form cannot be replayed as a payment result.
- AYA: a `+` in the base64 `payload` that arrived as a space (unencoded return query string) is mapped back before decoding; the checksum is still verified.
- Typed errors: `InvalidPaymentDataError`, `APIError`, `SignatureVerificationError` and `ConfigurationError`.
- `HTTPDoer` and `TokenCache` abstractions so any HTTP client or cache can be plugged in; `DefaultHTTPClient` and `MemoryTokenCache` ship with the module.
- Field, amount and currency rules follow each gateway's official documentation; test vectors are shared with `laranex/php-myanmar-payments`.
- Standard library only; Go 1.22 or higher.
- Agent skill in `skills/go-myanmar-payments` so coding agents (Claude Code, Codex, Cursor and others) integrate the module correctly; install it with `npx skills add laranex/go-myanmar-payments`.

### Changed since the pre-releases
- `StatusCancelled` is renamed to `StatusCanceled` and its value from `"cancelled"` to `"canceled"` (American English), with no alias. Code or stored statuses from `v4.0.0-alpha.1` need the new name; gateway status literals such as Wave Money's `PAYMENT_REQUEST_CANCELLED` are unchanged.
