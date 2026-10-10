package myanmarpayments

import "net/http"

// HTTPDoer sends HTTP requests. *http.Client satisfies it; pass your own to add proxies,
// tracing or test doubles. A client you pass keeps its own timeout.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}
