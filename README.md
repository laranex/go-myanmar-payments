# Go Myanmar Payments

Go SDK for Myanmar payment gateways: KBZ Pay (PWA, QR, In-App), Wave Money, AYA Payment Gateway, Yoma MMQR and CyberSource Secure Acceptance.

Every gateway takes a typed payment data struct, validates it against the gateway's official documentation, and returns a typed result. Standard library only.

**Documentation:** [laranex.vercel.app/go-myanmar-payments](https://laranex.vercel.app/go-myanmar-payments)

```bash
go get github.com/laranex/go-myanmar-payments
```

Requires Go 1.22+.

```go
import (
	myanmarpayments "github.com/laranex/go-myanmar-payments"
	"github.com/laranex/go-myanmar-payments/kbzpay"
)

kbz, err := kbzpay.New(kbzpay.ConfigFromEnv(os.Getenv), nil)

// Start a payment: a typed result per flow
payment, err := kbz.PWA(ctx, kbzpay.PaymentData{OrderID: "ORDER_1", Amount: myanmarpayments.Kyat(1000), CallbackURL: "https://shop.test/kbz/callback"})
http.Redirect(w, r, payment.URL, http.StatusFound)

// Handle the callback: verified, with a gateway-independent status
request, _ := myanmarpayments.NewCallbackRequestFromHTTP(r)
callback, err := kbz.HandleCallback(request)
if err == nil && callback.IsSuccessful() {
	// compare callback.Amount with your order, then fulfil callback.OrderID
}
callback.Acknowledgement.Write(w) // KBZ Pay expects a plain "success"
```

| Package | Start a payment | Status check |
|---|---|---|
| `kbzpay` | `PWA`, `QR`, `App` | `Status(ctx, orderID)` |
| `wavemoney` | `Initiate` | callback only |
| `ayapay` | `Services`, `Initiate` (form) | `Status(ctx, orderID)` |
| `yomammqr` | `Initiate`, `RenewQR` | `Status(ctx, reference)` |
| `cybersource` | `Initiate` (form) | callback only |

Every gateway verifies callbacks with `HandleCallback` and returns a `*myanmarpayments.PaymentCallback`.

## Amounts

Amounts are `myanmarpayments.Amount` values, kept as exact decimal text and never rounded through a float:

```go
myanmarpayments.Kyat(1000)                   // whole amount
myanmarpayments.MustParseAmount("1000.50")   // decimal, panics on bad input (constants, tests)
amount, err := myanmarpayments.ParseAmount(s) // decimal from user input
```

Each gateway's `Validate` applies its documented rules:

| Gateway | Decimals | Other rules |
|---|---|---|
| KBZ Pay | up to 2 places | greater than 0, MMK only |
| Wave Money | no | greater than 0, MMK only; the total defaults to the sum of the items |
| AYA Payment Gateway | no | greater than 0, MMK only |
| Yoma MMQR | no | greater than 0 |
| CyberSource | yes | 0 or more, at most 15 characters, any ISO 4217 currency |

A negative `Kyat(n)` does not panic: it fails validation like any other bad field. In JSON an `Amount` is written as a string (`"1000.50"`) and read from a string or a plain number, using the number's literal text.

## License

MIT. See [LICENSE.md](LICENSE.md).
