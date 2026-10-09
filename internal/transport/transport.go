// Package transport sends the JSON and form posts the gateways use.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	myanmarpayments "github.com/laranex/go-myanmar-payments/v4"
	"github.com/laranex/go-myanmar-payments/v4/internal/values"
)

// Client posts requests through an HTTPDoer.
type Client struct {
	Doer myanmarpayments.HTTPDoer
}

// New returns a Client using doer, or the default client when doer is nil.
func New(doer myanmarpayments.HTTPDoer) *Client {
	if doer == nil {
		doer = myanmarpayments.DefaultHTTPClient()
	}
	return &Client{Doer: doer}
}

// Response is a gateway response.
type Response struct {
	Status int
	Body   []byte
}

// Successful reports a 2xx status.
func (r Response) Successful() bool { return r.Status >= 200 && r.Status < 300 }

// JSON decodes the body as an object, keeping numbers as json.Number.
func (r Response) JSON() map[string]any {
	decoded, ok := values.DecodeObject(r.Body)
	if !ok {
		return map[string]any{}
	}
	return decoded
}

// PostJSON sends data as a JSON body.
func (c *Client) PostJSON(ctx context.Context, endpoint string, data any, headers map[string]string) (Response, error) {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		return Response{}, fmt.Errorf("myanmarpayments: encode request: %w", err)
	}

	return c.post(ctx, endpoint, bytes.TrimRight(body.Bytes(), "\n"), merge(map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, headers))
}

// PostForm sends data as an urlencoded form.
func (c *Client) PostForm(ctx context.Context, endpoint string, data url.Values, headers map[string]string) (Response, error) {
	return c.post(ctx, endpoint, []byte(data.Encode()), merge(map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "application/json",
	}, headers))
}

func (c *Client) post(ctx context.Context, endpoint string, body []byte, headers map[string]string) (Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, &myanmarpayments.APIError{Message: fmt.Sprintf("could not build request to %s: %v", endpoint, err), Err: err}
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	response, err := c.Doer.Do(request)
	if err != nil {
		return Response{}, &myanmarpayments.APIError{Message: fmt.Sprintf("could not reach %s: %v", endpoint, err), Err: err}
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return Response{}, &myanmarpayments.APIError{Message: fmt.Sprintf("could not read response from %s: %v", endpoint, err), HTTPStatus: response.StatusCode, Err: err}
	}

	return Response{Status: response.StatusCode, Body: responseBody}, nil
}

func merge(base, extra map[string]string) map[string]string {
	for key, value := range extra {
		base[key] = value
	}
	return base
}

// TrimURL removes trailing slashes.
func TrimURL(u string) string { return strings.TrimRight(u, "/") }
