package httpclient_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/httpclient"
)

// server answers each request with the next handler, repeating the last one,
// and counts the requests it received.
func server(t *testing.T, handlers ...http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1)) - 1
		handlers[min(n, len(handlers)-1)](w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
}

func fast(opts ...httpclient.Option) *httpclient.Client {
	return httpclient.New(append([]httpclient.Option{httpclient.WithBackoff(time.Millisecond)}, opts...)...)
}

func TestJSONSendsAndDecodes(t *testing.T) {
	srv, _ := server(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/v1/users" || r.URL.Query().Get("page") != "2" {
			t.Errorf("URL = %s, want /v1/users?page=2", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("headers = %v", r.Header)
		}
		if string(body) != `{"name":"ada"}` {
			t.Errorf("body = %s", body)
		}
		_, _ = w.Write([]byte(`{"id":7}`))
	})
	client := fast(httpclient.WithBaseURL(srv.URL+"/v1"), httpclient.WithHeader("Authorization", "Bearer key"))

	var out struct{ ID int }
	if err := client.JSON(t.Context(), http.MethodPost, "/users?page=2", map[string]string{"name": "ada"}, &out); err != nil {
		t.Fatalf("JSON() = %v", err)
	}
	if out.ID != 7 {
		t.Errorf("decoded ID = %d, want 7", out.ID)
	}
}

// A request made while serving another carries that request's ID, so one ID
// follows the work across services.
func TestTheRequestIDIsSentAlong(t *testing.T) {
	srv, _ := server(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Request-ID"); got != "req-1" {
			t.Errorf("X-Request-ID = %q, want req-1", got)
		}
	})

	ctx := medusa.WithRequestID(t.Context(), "req-1")
	if err := fast().JSON(ctx, http.MethodGet, srv.URL, nil, nil); err != nil {
		t.Fatalf("JSON() = %v", err)
	}
}

// A rejection is the caller's to interpret, so it is not dressed up as an
// application error: unhandled, it must become a 500, not the dependency's
// status.
func TestARejectionIsAStatusError(t *testing.T) {
	srv, calls := server(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"no such customer"}`))
	})

	err := fast().JSON(t.Context(), http.MethodGet, srv.URL, nil, nil)

	statusErr, ok := errors.AsType[*httpclient.StatusError](err)
	if !ok || statusErr.Status != http.StatusNotFound || !strings.Contains(string(statusErr.Body), "no such customer") {
		t.Fatalf("err = %v, want a StatusError with the 404 and its body", err)
	}
	if got := apperr.From(err).Kind(); got != apperr.KindInternal {
		t.Errorf("kind = %s, want internal for an unhandled rejection", got)
	}
	if calls.Load() != 1 {
		t.Errorf("requests = %d, want 1: a rejection is not retried", calls.Load())
	}
}

func TestAnOverloadedServiceIsRetriedThenUnavailable(t *testing.T) {
	t.Run("recovers", func(t *testing.T) {
		srv, calls := server(t, status(http.StatusServiceUnavailable), status(http.StatusBadGateway), status(http.StatusOK))

		if err := fast().JSON(t.Context(), http.MethodGet, srv.URL, nil, nil); err != nil {
			t.Fatalf("JSON() = %v, want the third attempt to succeed", err)
		}
		if calls.Load() != 3 {
			t.Errorf("requests = %d, want 3", calls.Load())
		}
	})

	t.Run("gives up", func(t *testing.T) {
		srv, calls := server(t, status(http.StatusServiceUnavailable))

		err := fast(httpclient.WithRetries(1)).JSON(t.Context(), http.MethodGet, srv.URL, nil, nil)

		if !errors.Is(err, httpclient.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if statusErr, ok := errors.AsType[*httpclient.StatusError](err); !ok || statusErr.Status != http.StatusServiceUnavailable {
			t.Errorf("cause = %v, want the 503", err)
		}
		if calls.Load() != 2 {
			t.Errorf("requests = %d, want 2", calls.Load())
		}
	})
}

// The first attempt of a POST may have been applied, so sending it again can
// charge a card twice. An Idempotency-Key is the server's promise it cannot.
func TestAPostIsRetriedOnlyWithAnIdempotencyKey(t *testing.T) {
	post := func(t *testing.T, key string) int32 {
		srv, calls := server(t, status(http.StatusServiceUnavailable), status(http.StatusOK))

		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, strings.NewReader(`{"amount":10}`))
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		resp, err := fast().Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		return calls.Load()
	}

	if got := post(t, ""); got != 1 {
		t.Errorf("requests without a key = %d, want 1", got)
	}
	if got := post(t, "key-1"); got != 2 {
		t.Errorf("requests with a key = %d, want 2", got)
	}
}

func TestARetrySendsTheBodyAgain(t *testing.T) {
	var bodies []string
	srv, _ := server(t,
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(body))
			w.WriteHeader(http.StatusServiceUnavailable)
		},
		func(_ http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(body))
		},
	)

	if err := fast().JSON(t.Context(), http.MethodPut, srv.URL, map[string]int{"n": 1}, nil); err != nil {
		t.Fatalf("JSON() = %v", err)
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] || bodies[1] != `{"n":1}` {
		t.Errorf("bodies = %q, want the same body twice", bodies)
	}
}

func TestRetryAfterIsHonored(t *testing.T) {
	srv, _ := server(t,
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		},
		status(http.StatusOK),
	)

	start := time.Now()
	if err := fast().JSON(t.Context(), http.MethodGet, srv.URL, nil, nil); err != nil {
		t.Fatalf("JSON() = %v", err)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Errorf("retried after %s, want the second the server asked for", elapsed)
	}
}

func TestAnUnreachableServiceIsUnavailable(t *testing.T) {
	srv, _ := server(t, status(http.StatusOK))
	url := srv.URL
	srv.Close()

	err := fast(httpclient.WithRetries(0)).JSON(t.Context(), http.MethodGet, url, nil, nil)
	if !errors.Is(err, httpclient.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestASlowServiceIsUnavailableAfterTheTimeout(t *testing.T) {
	srv, _ := server(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})

	err := fast(httpclient.WithTimeout(50*time.Millisecond), httpclient.WithRetries(0)).JSON(t.Context(), http.MethodGet, srv.URL, nil, nil)
	if !errors.Is(err, httpclient.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

// When the caller's own context ends, the outcome is the caller's (canceled
// or out of time), not a claim that the other service is down.
func TestTheCallersCancellationIsNotBlamedOnTheService(t *testing.T) {
	srv, _ := server(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)

	err := fast().JSON(ctx, http.MethodGet, srv.URL, nil, nil)
	if !errors.Is(err, context.Canceled) || errors.Is(err, httpclient.ErrUnavailable) {
		t.Errorf("err = %v, want context.Canceled and not ErrUnavailable", err)
	}
}

func TestAResponseOverTheCapFails(t *testing.T) {
	srv, _ := server(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":"` + strings.Repeat("a", 4096) + `"}`))
	})

	var out map[string]string
	err := fast(httpclient.WithMaxResponseBytes(1024)).JSON(t.Context(), http.MethodGet, srv.URL, nil, &out)

	if _, ok := errors.AsType[*http.MaxBytesError](err); !ok {
		t.Errorf("err = %v, want *http.MaxBytesError", err)
	}
}
