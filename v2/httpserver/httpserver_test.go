package httpserver

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/imlargo/medusa/app"
)

// Server is managed by app.App, so it must keep satisfying app.Server.
var _ app.Server = (*Server)(nil)

func quietLogger(t *testing.T) *slog.Logger {
	t.Helper()

	return slog.New(slog.DiscardHandler)
}

// freePort asks the OS for a port nobody is using.
func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func newTestServer(t *testing.T, opts ...Option) *Server {
	t.Helper()

	router := http.NewServeMux()
	router.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})

	opts = append([]Option{WithHost("127.0.0.1"), WithPort(freePort(t))}, opts...)
	return New(router, quietLogger(t), opts...)
}

// A failed listen used to call logger.Fatalf, killing the process from inside a
// library. It must be an ordinary error now.
func TestStartReturnsAnErrorInsteadOfExiting(t *testing.T) {
	port := freePort(t)

	occupied, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("occupy the port: %v", err)
	}
	defer occupied.Close()

	srv := New(http.NewServeMux(), quietLogger(t), WithHost("127.0.0.1"), WithPort(port))

	startErr := srv.Start(context.Background())

	if startErr == nil {
		t.Fatal("Start() = nil, want an error for an address already in use")
	}
}

func TestStartServesUntilStopped(t *testing.T) {
	srv := newTestServer(t)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(context.Background()) }()

	waitUntilServing(t, srv.Addr())

	resp, err := http.Get("http://" + srv.Addr() + "/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if stopErr := srv.Stop(context.Background()); stopErr != nil {
		t.Errorf("Stop() = %v, want nil", stopErr)
	}

	// A graceful shutdown is not an error.
	if startErr := <-errCh; startErr != nil {
		t.Errorf("Start() = %v, want nil after a graceful stop", startErr)
	}
}

// The Server contract promises Start also returns when ctx is done, which the
// previous implementation ignored entirely.
func TestStartReturnsWhenTheContextIsCanceled(t *testing.T) {
	srv := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	waitUntilServing(t, srv.Addr())
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after the context was canceled")
	}
}

// Stop used to discard the context it was given and dereference a nil server
// when called before Start.
func TestStopBeforeStartIsANoop(t *testing.T) {
	srv := newTestServer(t)

	if err := srv.Stop(context.Background()); err != nil {
		t.Errorf("Stop() = %v, want nil when the server never started", err)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	srv := newTestServer(t)

	go func() { _ = srv.Start(context.Background()) }()
	waitUntilServing(t, srv.Addr())

	for i := range 3 {
		if err := srv.Stop(context.Background()); err != nil {
			t.Errorf("Stop() call %d = %v, want nil", i, err)
		}
	}
}

// Shutdown must not inherit a canceled context: it would abort in-flight
// requests instead of draining them.
func TestStopDrainsEvenWithACanceledContext(t *testing.T) {
	router := http.NewServeMux()
	released := make(chan struct{})
	router.HandleFunc("GET /slow", func(w http.ResponseWriter, _ *http.Request) {
		<-released
		_, _ = w.Write([]byte("done"))
	})

	srv := New(router, quietLogger(t),
		WithHost("127.0.0.1"),
		WithPort(freePort(t)),
		WithShutdownTimeout(3*time.Second),
	)

	go func() { _ = srv.Start(context.Background()) }()
	waitUntilServing(t, srv.Addr())

	respCh := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get("http://" + srv.Addr() + "/slow")
		if err == nil {
			respCh <- resp
		} else {
			respCh <- nil
		}
	}()

	// Give the request time to land in the handler.
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stopDone := make(chan error, 1)
	go func() { stopDone <- srv.Stop(ctx) }()

	// Let the in-flight request finish; a shutdown that inherited the
	// cancellation would already have torn the connection down.
	close(released)

	if err := <-stopDone; err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}

	resp := <-respCh
	if resp == nil {
		t.Fatal("the in-flight request was aborted, want it drained")
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("in-flight request status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestReadHeaderTimeoutDefaultsAndCanBeDisabled(t *testing.T) {
	if got := newTestServer(t).readHeaderTimeout; got != DefaultReadHeaderTimeout {
		t.Errorf("readHeaderTimeout = %s, want %s", got, DefaultReadHeaderTimeout)
	}

	srv := newTestServer(t, WithReadHeaderTimeout(-1))
	if got := srv.readHeaderTimeout; got >= 0 {
		t.Errorf("readHeaderTimeout = %s, want a negative value to mean disabled", got)
	}
}

func TestAddrJoinsHostAndPortForIPv6(t *testing.T) {
	srv := New(http.NewServeMux(), quietLogger(t), WithHost("::1"), WithPort(8080))

	if got, want := srv.Addr(), "[::1]:8080"; got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
}

// waitUntilServing blocks until the address accepts connections.
func waitUntilServing(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("server at %s never started accepting connections", addr)
}
