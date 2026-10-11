package myanmarpayments

import "net/http"

// DefaultTimeoutSeconds is the timeout, in seconds, of the HTTP client a gateway builds when its
// config leaves TimeoutSeconds at 0 or MYANMAR_PAYMENTS_HTTP_TIMEOUT is unset or blank.
const DefaultTimeoutSeconds = 30

// HTTPDoer sends HTTP requests. *http.Client satisfies it; pass your own to add proxies,
// tracing or test doubles. A client you pass keeps its own timeout.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}
