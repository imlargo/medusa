// Package httpclient calls other services over HTTP with the defaults every
// integration ends up needing: a timeout on every request, a cap on the
// response body, retries only where repeating a request is safe, the request
// ID sent along, and failures classified as application errors.
//
//	stripe := httpclient.New(
//	    httpclient.WithBaseURL("https://api.stripe.com"),
//	    httpclient.WithHeader("Authorization", "Bearer "+key),
//	)
//
//	var customer Customer
//	err := stripe.JSON(ctx, http.MethodGet, "/v1/customers/"+id, nil, &customer)
//
// # What an error means
//
// A response outside 2xx is an error, a *[StatusError] carrying the status
// and the start of the body:
//
//	if statusErr, ok := errors.AsType[*httpclient.StatusError](err); ok && statusErr.Status == http.StatusNotFound { ... }
//
// A failure that is the other service's (it cannot be reached, it timed out,
// it answered 429, 502, 503 or 504) is [ErrUnavailable] with the detail as
// its cause, so a handler that returns it answers 503 instead of 500. Any
// other status is returned as the *StatusError alone: a 400 or a 404 from a
// dependency is this service's bug or this service's decision, and only the
// caller knows which, so left unhandled it becomes a 500.
//
// # What is retried
//
// GET, HEAD, OPTIONS, PUT and DELETE, and any request that carries an
// Idempotency-Key header, are retried on the failures listed above, waiting
// longer each time and honoring Retry-After. A POST without that header is
// never retried: the first attempt may have been applied.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/internal/reqid"
)

// Defaults applied when the matching option is not given.
const (
	// DefaultTimeout bounds one attempt, from connecting to the last byte of
	// the body.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxResponseBytes caps a response body. Reading past it fails
	// with *http.MaxBytesError instead of filling memory.
	DefaultMaxResponseBytes = 10 << 20

	// DefaultRetries is how many times a retryable request is sent again.
	DefaultRetries = 2

	// DefaultBackoff is the wait before the first retry. Each later one
	// doubles it, up to MaxBackoff.
	DefaultBackoff = 200 * time.Millisecond

	// MaxBackoff caps the wait between attempts, including one a server asks
	// for with Retry-After.
	MaxBackoff = 10 * time.Second
)

// requestIDHeader carries the request ID to the service being called.
const requestIDHeader = "X-Request-ID"

// idempotencyHeader marks a request its server will apply at most once.
const idempotencyHeader = "Idempotency-Key"

// errorBodyLimit is how much of an error response a StatusError keeps.
const errorBodyLimit = 4 << 10

// ErrUnavailable: a service this request depends on could not be reached,
// timed out or reported it is overloaded. Its cause says which and why.
var ErrUnavailable = apperr.New(apperr.KindUnavailable, "upstream.unavailable", "a service this request depends on is not available, retry later")

// StatusError is a response with a status outside 2xx.
type StatusError struct {
	Method string
	URL    string
	Status int

	// Body is the start of the response body, for the log. It comes from
	// another service: never send it to a client.
	Body []byte

	// Header is the response's headers.
	Header http.Header
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s %s: status %d", e.Method, e.URL, e.Status)
}

// Client sends requests. It is safe for concurrent use; build one per service
// called, so each has its own base URL, credentials and limits.
type Client struct {
	http     *http.Client
	baseURL  *url.URL
	header   http.Header
	maxBytes int64
	retries  int
	backoff  time.Duration
}

// Option configures a Client.
type Option func(*Client)

// New builds a client. It panics on a base URL that does not parse, which is
// a wiring mistake found at startup.
func New(opts ...Option) *Client {
	c := &Client{
		http:     &http.Client{Timeout: DefaultTimeout},
		header:   http.Header{},
		maxBytes: DefaultMaxResponseBytes,
		retries:  DefaultRetries,
		backoff:  DefaultBackoff,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithBaseURL resolves every request path against base, so calls name only
// the path: "https://api.example.com/v1/" plus "users" is
// "https://api.example.com/v1/users".
func WithBaseURL(base string) Option {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		panic(fmt.Sprintf("httpclient: base URL %q must be absolute", base))
	}
	return func(c *Client) { c.baseURL = parsed }
}

// WithHeader sends a header on every request, unless the request sets it.
func WithHeader(name, value string) Option {
	return func(c *Client) { c.header.Set(name, value) }
}

// WithTimeout bounds each attempt. The request's context still applies, and
// whichever ends first wins.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.http.Timeout = d }
}

// WithMaxResponseBytes caps the response body.
func WithMaxResponseBytes(n int64) Option {
	return func(c *Client) { c.maxBytes = n }
}

// WithRetries sets how many times a retryable request is sent again. Zero
// turns retries off.
func WithRetries(n int) Option {
	return func(c *Client) { c.retries = n }
}

// WithBackoff sets the wait before the first retry.
func WithBackoff(d time.Duration) Option {
	return func(c *Client) { c.backoff = d }
}

// WithTransport replaces the transport, for a proxy, custom TLS or tracing.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *Client) { c.http.Transport = rt }
}

// Do sends req and returns its response when the status is 2xx. The caller
// must close the body, which fails with *http.MaxBytesError past the cap.
// Build req with http.NewRequestWithContext: its context bounds the whole
// call, retries and waits included.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	c.prepare(req)

	for attempt := 0; ; attempt++ {
		resp, err := c.http.Do(req)

		var wait time.Duration
		switch {
		case err != nil:
			if ctx.Err() != nil {
				// The caller gave up or ran out of time: its own outcome,
				// not the other service's.
				return nil, fmt.Errorf("%s %s: %w", req.Method, req.URL, ctx.Err())
			}
			err = ErrUnavailable.WithCause(fmt.Errorf("%s %s: %w", req.Method, req.URL, err))
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			resp.Body = http.MaxBytesReader(nil, resp.Body, c.maxBytes)
			return resp, nil
		default:
			statusErr := newStatusError(req, resp)
			if !isUnavailable(resp.StatusCode) {
				return nil, statusErr
			}
			wait = retryAfter(resp.Header)
			err = ErrUnavailable.WithCause(statusErr)
		}

		if attempt >= c.retries || !canRetry(req) {
			return nil, err
		}
		if wait == 0 {
			wait = c.backoffFor(attempt)
		}
		if sleepErr := sleep(ctx, min(wait, MaxBackoff)); sleepErr != nil {
			return nil, err
		}
		if req.GetBody != nil {
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				return nil, err
			}
			req.Body = body
		}
	}
}

// JSON sends in as a JSON body (none when in is nil) and decodes a JSON
// response into out (ignored when out is nil). path is resolved against the
// base URL. It returns the errors Do does.
func (c *Client) JSON(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if out == nil {
		// Read to the end so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", req.Method, req.URL, err)
	}
	return nil
}

// prepare resolves the URL against the base and adds the client's headers
// and the request ID.
func (c *Client) prepare(req *http.Request) {
	if c.baseURL != nil && !req.URL.IsAbs() {
		// A base of ".../v1" and a path of "/users" should meet at
		// ".../v1/users", which ResolveReference alone would not do.
		base := *c.baseURL
		base.Path = strings.TrimSuffix(base.Path, "/") + "/"
		ref := *req.URL
		ref.Path = strings.TrimPrefix(ref.Path, "/")
		req.URL = base.ResolveReference(&ref)
		req.Host = ""
	}

	for name, values := range c.header {
		if req.Header.Get(name) == "" {
			req.Header[name] = values
		}
	}
	if id := reqid.From(req.Context()); id != "" && req.Header.Get(requestIDHeader) == "" {
		req.Header.Set(requestIDHeader, id)
	}
}

// backoffFor is the wait before retry number attempt+1: the base doubled per
// attempt, with up to half of it taken off at random so clients that failed
// together do not retry together.
func (c *Client) backoffFor(attempt int) time.Duration {
	wait := min(c.backoff<<attempt, MaxBackoff)
	if wait <= 0 {
		return 0
	}
	return wait - rand.N(wait/2+1)
}

func newStatusError(req *http.Request, resp *http.Response) *StatusError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
	_ = resp.Body.Close()

	return &StatusError{
		Method: req.Method,
		URL:    req.URL.Redacted(),
		Status: resp.StatusCode,
		Body:   body,
		Header: resp.Header,
	}
}

// isUnavailable reports whether a status is the other service saying "not
// now" rather than "no".
func isUnavailable(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// canRetry reports whether sending req again cannot apply it twice, and its
// body can be sent again.
func canRetry(req *http.Request) bool {
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return false
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	}
	return req.Header.Get(idempotencyHeader) != ""
}

// retryAfter reads a Retry-After header, in seconds or as a date. Zero means
// it is missing or unreadable.
func retryAfter(header http.Header) time.Duration {
	value := header.Get("Retry-After")
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(time.Until(at), 0)
	}
	return 0
}

// sleep waits d, or returns early with the context's error.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
