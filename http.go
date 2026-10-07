package myanmarpayments

import (
	"net/http"
	"time"
)

// HTTPDoer sends HTTP requests. *http.Client satisfies it; pass your own to add timeouts,
// proxies, tracing or test doubles.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTPClient returns the client used when none is given: an *http.Client with a
// 30 second timeout.
func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
