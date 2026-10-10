# Changelog

All notable changes to `go-myanmar-payments` will be documented in this file.

## v4.0.0 - Unreleased

Initial release. The version number matches the other Laranex packages (`php-myanmar-payments`, `laravel-myanmar-payments`), so the module path carries the major version: import `github.com/laranex/go-myanmar-payments/v4` and its sub-packages (`.../v4/kbzpay`, `.../v4/wavemoney`, `.../v4/ayapay`, `.../v4/yomammqr`, `.../v4/cybersource`).

### Added
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
- Field, amount and currency rules follow each gateway's official documentation; test vectors are shared with `laranex/php-myanmar-payments`.
- Standard library only; Go 1.22 or higher.
- Agent skill in `skills/go-myanmar-payments` so coding agents (Claude Code, Codex, Cursor and others) integrate the module correctly; install it with `npx skills add laranex/go-myanmar-payments`.

### Changed since the pre-releases
- No sandbox switch: the sandbox setting (`Production`) and the `*_SANDBOX` variables are removed, and every gateway defaults to its production endpoints. Every other setting is required: the HTTP timeout (`TimeoutSeconds`, `timeout_in_seconds`, `MYANMAR_PAYMENTS_HTTP_TIMEOUT`) for every gateway that calls an API, Wave Money's `TimeToLiveSeconds` (`time_to_live_in_seconds`) and `MerchantName` (no `APP_NAME` fallback), Yoma MMQR's `APIVersion` (`api_version`), and CyberSource's `Currency`, `Locale` and `TransactionType` on each payment. A missing setting returns the `ConfigurationError` naming it; a time setting that is not a whole number greater than 0 returns `The <gateway> configuration [<key>] must be a whole number greater than 0.` (`ConfigurationError.Invalid` is true). `SandboxAPIURL`, `SandboxPWAURL`, `SandboxURL`, `SandboxAuthenticateURL`, `yomammqr.DefaultAPIVersion`, `DefaultHTTPClient`, `wavemoney.Config.ResolvedTimeToLive` and `yomammqr.Config.ResolvedAPIVersion` are removed; a nil HTTP client becomes an `*http.Client` with the config's `TimeoutSeconds`, and a client you pass keeps its own timeout. Upgrading: delete `*_SANDBOX`; to keep testing against UAT, set the URL overrides to the UAT URLs listed in the docs; set the newly required settings.
- Behavior aligned with `php-`, `node-` and `python-myanmar-payments`, checked by the shared vectors in `testdata/parity/vectors.json`:
  - KBZ Pay `PaymentData.TimeoutMinutes` is now `*int`: nil means KBZ's default, and any value outside 1 to 120, including 0, is rejected (0 used to mean unset).
  - A nested value (object or array) in a signed or hashed callback field now fails verification: any KBZ Pay field, Wave Money's hashed fields, AYA's signed payload fields and Yoma's `orderNumber`/`status` (Wave used to hash it as `null`, KBZ and AYA ignored it).
  - AYA's base64 `payload` may be padded or unpadded; partial padding, the URL-safe alphabet, whitespace and payloads that are not UTF-8 are rejected.
  - The Yoma MMQR token cache key is `myanmar-payments.yoma-mmqr.token.<sha256(baseUrl|clientId)>` (was `go-myanmar-payments.`-prefixed), the same in every SDK so one shared cache serves them all; `expires_in` is read from its leading digits (`28800.0` is 28800 seconds).
  - New `Amount.Equals(text)` (compares by value: `01000`, `1000` and `1000.00` are equal; text that is not plain digits never is) and `Amount.WholePart()`.
  - New `(*kbzpay.Gateway).Signer()` for custom calls.
  - `FormPayment.HTML` escapes exactly `&`, `<`, `>`, `"` (`&#34;`) and `'` (`&#39;`), producing the same page as the other SDKs.
  - Wave Money's `items` JSON no longer HTML-escapes `<`, `>` and `&` in item names.
  - A callback body is read as JSON only when it is a single JSON object; a malformed pair in a urlencoded body is skipped instead of discarding the whole form.
  - `APIError` messages no longer end with a space when the gateway sends a code without a message.
  - Error texts match the other SDKs after the `myanmarpayments: ` prefix: `Invalid payment data: ...`, `The kbz_pay configuration is missing [app_id].`, `Could not reach <url>: ...`.
  - New `payments` package, the facade of the other SDKs: `payments.New(payments.Config{...}, payments.Options{...})` and `payments.FromEnv(getenv, options)` build each gateway (`KBZPay()`, `WaveMoney()`, `AYAPay()`, `YomaMMQR()`, `CyberSource()`) once, on first use, and are safe for concurrent use; a gateway without configuration returns a `ConfigurationError`.
  - New `PaymentError` interface implemented by all four error types, and `PaymentStatuses()` listing every status.
  - CyberSource's `FormPayment.Enctype` is `application/x-www-form-urlencoded` (was empty, which `HTML` already treated the same way).
  - Yoma MMQR: concurrent calls share one in-flight token request; a caller whose context ends stops waiting without failing the others.
- `StatusCancelled` is renamed to `StatusCanceled` and its value from `"cancelled"` to `"canceled"` (American English), with no alias. Code or stored statuses from `v4.0.0-alpha.1` need the new name; gateway status literals such as Wave Money's `PAYMENT_REQUEST_CANCELLED` are unchanged.
