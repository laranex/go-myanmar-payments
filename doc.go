// Package myanmarpayments holds the gateway-independent types of Go Myanmar Payments:
// payment statuses, the result returned for each payment flow, verified callbacks,
// status results, errors and the HTTP and token cache abstractions.
//
// Each gateway lives in its own sub-package: kbzpay, wavemoney, ayapay, yomammqr and
// cybersource. Every gateway takes a typed payment data struct, validates it against the
// gateway's official documentation and returns one of the typed results in this package.
package myanmarpayments
