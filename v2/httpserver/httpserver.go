// Package httpserver runs an http.Handler with lifecycle management and
// graceful shutdown. It implements app.Server, so app.App can manage it next
// to other servers.
//
// Any http.Handler works; a *gin.Engine is one:
//
//	router := gin.New()
//	srv := httpserver.New(router, logger,
//	    httpserver.WithHost("0.0.0.0"),
//	    httpserver.WithPort(8080),
//	    httpserver.WithShutdownTimeout(15*time.Second),
//	)
//
// Start returns its error rather than terminating the process: deciding
// whether a failed listen is fatal belongs to the application.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Defaults applied when the matching option is not supplied.
const (
	// DefaultPort is the TCP port used without WithPort.
	DefaultPort = 8080

	// DefaultShutdownTimeout bounds how long in-flight requests get to finish.
	DefaultShutdownTimeout = 10 * time.Second

	// DefaultReadHeaderTimeout caps how long a client may take to send its
	// request headers. Without it a handful of connections trickling headers
	// one byte at a time can hold the server open indefinitely (Slowloris).
	DefaultReadHeaderTimeout = 10 * time.Second
)

// Server serves an http.Handler. Start must be called once.
type Server struct {
	handler           http.Handler
	host              string
	port              int
	shutdownTimeout   time.Duration
	readHeaderTimeout time.Duration
	logger            *slog.Logger

	// mu guards httpSrv, which Start publishes and Stop reads, possibly from
	// another goroutine.
	mu       sync.Mutex
	httpSrv  *http.Server
	stopOnce sync.Once
}

// Option configures a Server.
type Option func(s *Server)

// New creates a server for handler. It listens on DefaultPort of every
// interface unless told otherwise. A nil logger means slog.Default().
func New(handler http.Handler, logger *slog.Logger, opts ...Option) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		handler:           handler,
		port:              DefaultPort,
		shutdownTimeout:   DefaultShutdownTimeout,
		readHeaderTimeout: DefaultReadHeaderTimeout,
		logger:            logger,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// WithHost sets the bind address. Use "0.0.0.0" for every interface, or
// "localhost" to accept only local connections. Empty means every interface.
func WithHost(host string) Option {
	return func(s *Server) { s.host = host }
}

// WithPort sets the TCP port to listen on.
func WithPort(port int) Option {
	return func(s *Server) { s.port = port }
}

// WithShutdownTimeout sets how long Stop waits for in-flight requests before
// closing connections. It should exceed your slowest normal request and stay
// below your orchestrator's termination grace period.
func WithShutdownTimeout(d time.Duration) Option {
	return func(s *Server) { s.shutdownTimeout = d }
}

// WithReadHeaderTimeout caps how long a client may take to send request
// headers. A negative duration disables the limit, which is only reasonable
// behind a proxy that already enforces one.
func WithReadHeaderTimeout(d time.Duration) Option {
	return func(s *Server) { s.readHeaderTimeout = d }
}

// Addr returns the address the server listens on.
func (s *Server) Addr() string {
	return net.JoinHostPort(s.host, strconv.Itoa(s.port))
}

// Start serves HTTP requests and blocks until the server is stopped, the
// context is canceled, or serving fails.
//
// It returns nil on a graceful shutdown and an error if the listener cannot
// be opened or serving fails unexpectedly.
func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.Addr(),
		Handler:           s.handler,
		ReadHeaderTimeout: max(s.readHeaderTimeout, 0),
	}

	s.mu.Lock()
	s.httpSrv = srv
	s.mu.Unlock()

	// Start also returns when ctx is done, not only when Stop is called, so
	// cancellation has to reach Shutdown. The watcher exits either way.
	returned := make(chan struct{})
	defer close(returned)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Stop(context.WithoutCancel(ctx))
		case <-returned:
		}
	}()

	s.logger.Info("http server listening", slog.String("addr", srv.Addr))

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http on %s: %w", srv.Addr, err)
	}

	return nil
}

// Stop gracefully shuts the server down, waiting up to the configured
// shutdown timeout for in-flight requests to finish before closing
// connections. Calling it more than once, or before Start, does nothing.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpSrv
	s.mu.Unlock()

	if srv == nil {
		return nil
	}

	var err error
	s.stopOnce.Do(func() {
		s.logger.Info("shutting down http server", slog.Duration("timeout", s.shutdownTimeout))

		// The parent's cancellation is dropped on purpose: shutdown is often
		// triggered by a canceled context, and inheriting it would abort
		// in-flight requests instead of draining them. The timeout bounds the
		// wait.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
		defer cancel()

		if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
			err = fmt.Errorf("shutdown http server: %w", shutdownErr)
			s.logger.Error("graceful shutdown failed, connections were closed", slog.Any("error", shutdownErr))
			return
		}

		s.logger.Info("http server stopped")
	})

	return err
}
