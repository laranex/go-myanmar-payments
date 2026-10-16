# Go Myanmar Payments

[![Go Reference](https://pkg.go.dev/badge/github.com/laranex/go-myanmar-payments/v4.svg)](https://pkg.go.dev/github.com/laranex/go-myanmar-payments/v4)
[![Tests](https://github.com/laranex/go-myanmar-payments/actions/workflows/tests.yml/badge.svg)](https://github.com/laranex/go-myanmar-payments/actions/workflows/tests.yml)
[![License](https://img.shields.io/github/license/laranex/go-myanmar-payments.svg?style=flat-square)](LICENSE.md)

Go SDK for Myanmar payment gateways: KBZ Pay, Wave Pay, AYA Pay, Yoma MMQR and CyberSource. Typed requests and results, exact amounts, standard library only. Built for humans and AI agents.

Every Myanmar payment gateway signs its requests and callbacks its own way, reports its own status codes and has its own rules for amounts. Go Myanmar Payments handles all of that behind one typed API for `net/http` and every router built on it: build the payment data, start the payment, then verify the callback the gateway sends back.

## Why Go Myanmar Payments

- **Five gateways, one pattern.** KBZ Pay (PWA, QR and in-app), Wave Pay, AYA Pay, Yoma MMQR and CyberSource, each in its own package with the same three steps.
- **One typed result per flow.** `RedirectPayment`, `FormPayment`, `QrPayment` or `AppPayment` holds exactly what the next step needs: a URL to redirect to, an auto-submitting form, a QR code or the signed order for the KBZ Pay mobile SDK. See [Payment Flows](https://laranex.vercel.app/go-myanmar-payments/payment-flows).
- **Exact amounts, checked against each gateway's rules.** An `Amount` is kept as decimal text and never passes through a `float64`. Decimals, currency and limits follow each gateway's official documentation and are checked before any request is sent. See [Amounts](https://laranex.vercel.app/go-myanmar-payments/amounts).
- **Verified callbacks.** `HandleCallback()` checks the gateway's signature without a network call, maps its status onto one `PaymentStatus` and gives you the acknowledgement the gateway expects. When a callback is late, ask KBZ Pay, AYA Pay or Yoma MMQR with `Status()`. See [Callbacks & Status](https://laranex.vercel.app/go-myanmar-payments/callbacks).
- **Typed errors.** A missing setting, an invalid field, a gateway error and a bad signature each return their own error struct, matched with `errors.As`, with a message that names the setting, the field or the gateway's code.
- **Standard library only.** No dependencies: requests go through `net/http` or any `HTTPDoer` you pass, every network call takes a `context.Context`, and gateways are safe for concurrent use. Yoma MMQR tokens go in any `TokenCache`, such as Redis. Goravel apps use [Goravel Myanmar Payments](https://laranex.vercel.app/goravel-myanmar-payments/introduction), which wraps this module.
- **Same behavior in every language.** The PHP, Go, Node.js and Python SDKs pass one shared set of test vectors for signing and callback verification, and read the same environment variables, so one `.env` file and one Yoma MMQR token cache serve them all.
- **Nothing stored for you.** The module keeps no tables and no webhook log. [Handling Webhooks](https://laranex.vercel.app/go-myanmar-payments/webhooks) shows the recommended flow as code you copy: verify, store, acknowledge, then process each call once.
- **Easy to test.** Every gateway call goes through the `HTTPDoer` you pass, so your tests fake the responses with `httptest`, replay signed callbacks and never touch the network. See [Testing](https://laranex.vercel.app/go-myanmar-payments/testing).

## Installation

> **Requires** Go 1.22+. No dependencies outside the standard library: it sends the gateway requests with `net/http`.

```bash
go get github.com/laranex/go-myanmar-payments/v4
```

## Documentation

The full documentation lives at **[laranex.vercel.app/go-myanmar-payments](https://laranex.vercel.app/go-myanmar-payments)**:

- [Introduction](https://laranex.vercel.app/go-myanmar-payments/introduction): why the package exists and what it covers
- [Installation](https://laranex.vercel.app/go-myanmar-payments/installation): requirements, install command and a quick start
- [AI Agent Skill](https://laranex.vercel.app/go-myanmar-payments/ai-agent-skill): the skill that teaches coding agents to use the package
- [Configuration](https://laranex.vercel.app/go-myanmar-payments/configuration): credentials, environment variables, endpoints, UAT, HTTP client and token cache
- [Amounts](https://laranex.vercel.app/go-myanmar-payments/amounts): exact amounts and each gateway's amount rules
- [Payment Flows](https://laranex.vercel.app/go-myanmar-payments/payment-flows): redirect, form, QR and in-app payments
- [Callbacks & Status](https://laranex.vercel.app/go-myanmar-payments/callbacks): verified callbacks, payment statuses and status checks
- [Handling Webhooks](https://laranex.vercel.app/go-myanmar-payments/webhooks): the recommended verify, store, acknowledge, process-once flow
- [Framework Integration](https://laranex.vercel.app/go-myanmar-payments/framework-integration): `net/http`, chi, Gin and Echo
- [Testing](https://laranex.vercel.app/go-myanmar-payments/testing): faking gateway calls and replaying signed callbacks in your tests
- [KBZ Pay](https://laranex.vercel.app/go-myanmar-payments/drivers/kbz-pay): PWA redirect, QR and in-app payments, status checks and callbacks
- [Wave Pay](https://laranex.vercel.app/go-myanmar-payments/drivers/wave-pay): redirect payments with line items and verified callbacks
- [AYA Pay](https://laranex.vercel.app/go-myanmar-payments/drivers/aya-pay): hosted checkout for AYA Pay, other wallets and cards, with status checks
- [Yoma MMQR](https://laranex.vercel.app/go-myanmar-payments/drivers/yoma-mmqr): QR images, QR renewal, status checks and verified callbacks
- [CyberSource](https://laranex.vercel.app/go-myanmar-payments/drivers/cyber-source): Secure Acceptance card payments through a signed hosted checkout form
- [Results](https://laranex.vercel.app/go-myanmar-payments/references/results): every property of the payment result types
- [PaymentCallback & Status](https://laranex.vercel.app/go-myanmar-payments/references/payment-callback): callbacks, status results, acknowledgements and the PaymentStatus values
- [Errors](https://laranex.vercel.app/go-myanmar-payments/references/errors): validation, API, signature and configuration errors

## License

MIT. See [LICENSE](LICENSE.md).
