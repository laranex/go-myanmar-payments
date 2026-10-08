# Go Myanmar Payments

[![Go Reference](https://pkg.go.dev/badge/github.com/laranex/go-myanmar-payments/v4.svg)](https://pkg.go.dev/github.com/laranex/go-myanmar-payments/v4)
[![Tests](https://img.shields.io/github/actions/workflow/status/laranex/go-myanmar-payments/tests.yml?branch=main&label=tests&style=flat-square)](https://github.com/laranex/go-myanmar-payments/actions/workflows/tests.yml)
[![License](https://img.shields.io/github/license/laranex/go-myanmar-payments.svg?style=flat-square)](LICENSE.md)

Go SDK for Myanmar payment gateways: KBZ Pay (PWA, QR, In-App), Wave Money, AYA Payment Gateway, Yoma MMQR and CyberSource Secure Acceptance. Every gateway takes a typed payment data struct, validates it against the gateway's official documentation, verifies callbacks and returns typed results with a gateway-independent payment status. It is for Go services that need to accept payments in Myanmar without hand-rolling each gateway's signing and callback rules.

## Documentation

Full documentation lives at **[laranex.vercel.app/go-myanmar-payments](https://laranex.vercel.app/go-myanmar-payments)**.

## Requirements

- Go 1.22 or higher, standard library only

## Installation

```bash
go get github.com/laranex/go-myanmar-payments/v4
```

## Usage

```go
import (
	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/kbzpay"
)

kbz, err := kbzpay.New(kbzpay.ConfigFromEnv(os.Getenv), nil)

// Start a payment: a typed result per flow
payment, err := kbz.PWA(ctx, kbzpay.PaymentData{
	OrderID:     "ORDER_1",
	Amount:      myanmarpayments.Kyat(1000),
	CallbackURL: "https://shop.test/kbz/callback",
})
http.Redirect(w, r, payment.URL, http.StatusFound)

// Handle the callback: verified, with a gateway-independent status
request, _ := myanmarpayments.NewCallbackRequestFromHTTP(r)
callback, err := kbz.HandleCallback(request)
if err == nil && callback.IsSuccessful() {
	// compare callback.Amount with your order, then fulfill callback.OrderID
}
callback.Acknowledgement.Write(w) // KBZ Pay expects a plain "success"
```

The other gateways follow the same shape: `wavemoney.Initiate`, `ayapay.Initiate` (form), `yomammqr.Initiate` / `RenewQR` and `cybersource.Initiate` (form), each with `HandleCallback`; KBZ Pay, AYA and Yoma MMQR also offer `Status`. Amounts are exact `myanmarpayments.Amount` values (`Kyat`, `ParseAmount`, `MustParseAmount`) and never pass through a float. See the [documentation](https://laranex.vercel.app/go-myanmar-payments) for every gateway, the amount rules and callback handling.

## Built for humans and AI agents

The documentation is written for developers, and the package ships an agent skill so AI coding agents use it the way it's meant to be used.

- Install it with `npx skills add laranex/go-myanmar-payments` (Claude Code, Codex, Cursor and others), or copy `skills/go-myanmar-payments` into your project's `.claude/skills` or `.agents/skills`.

## Testing

```bash
go test ./...
```

## Changelog

Please see [CHANGELOG](CHANGELOG.md) for more information on what has changed recently.

## Contributing

Please see [CONTRIBUTING](.github/CONTRIBUTING.md) for details.

## Security Vulnerabilities

Please review [our security policy](.github/SECURITY.md) on how to report security vulnerabilities.

## Credits

- [Nay Thu Khant](https://github.com/NayThuKhant)
- [All Contributors](../../contributors)

## License

The MIT License (MIT). Please see [License File](LICENSE.md) for more information.
