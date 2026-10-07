# Changelog

All notable changes to `go-myanmar-payments` will be documented in this file.

## v0.1.0 - Unreleased

- KBZ Pay (PWA, QR, In-App), Wave Money, AYA Payment Gateway, Yoma MMQR and CyberSource Secure Acceptance
- One typed payment data struct per gateway and one result type per flow: `RedirectPayment`, `FormPayment`, `QrPayment`, `AppPayment`
- Verified callbacks return `PaymentCallback` with a gateway-independent `PaymentStatus` and the acknowledgement each gateway expects
- Status checks for KBZ Pay, AYA and Yoma MMQR return `PaymentStatusResult`
- Exact `Amount` type (`Kyat`, `ParseAmount`, `MustParseAmount`) for every gateway, so amounts never pass through a float
- Field, amount and currency rules follow each gateway's official documentation, shared test vectors with `laranex/php-myanmar-payments`
