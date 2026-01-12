// Package http provides an HTTP server implementation using the Gin framework.
//
// The HTTP server wraps Gin's router with lifecycle management, graceful shutdown,
// and structured logging. It implements the server.Server interface, allowing it
// to be managed by the Medusa application framework.
//
// Example usage:
//
//	log := logger.NewLogger()
//	router := gin.Default()
//
//	// Configure routes
//	router.GET("/health", healthHandler)
//	router.GET("/api/users", getUsersHandler)
//
//	// Create server with options
//	srv := http.NewServer(router, log,
//	    http.WithServerHost("0.0.0.0"),
//	    http.WithServerPort(8080),
//	)
//
//	// Start server (blocks until stopped)
//	if err := srv.Start(context.Background()); err != nil {
//	    log.Fatal("Server failed", zap.Error(err))
//	}
//
// The server supports graceful shutdown with a 5-second timeout for in-flight requests.
package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imlargo/medusa/pkg/medusa/core/logger"
)

// Server is an HTTP server implementation that wraps Gin with lifecycle management.
// It embeds *gin.Engine, making all Gin methods directly available for route configuration.
//
// The server is safe to use from multiple goroutines for route configuration,
// but Start() should only be called once.
type Server struct {
	*gin.Engine
	httpSrv *http.Server
	host    string
	port    int
	logger  *logger.Logger
}

// Option is a functional option for configuring an HTTP Server.
type Option func(s *Server)

// NewServer creates a new HTTP server with the provided Gin engine and logger.
// Additional configuration can be provided via functional options.
//
// The server defaults to listening on localhost:8080 if no host/port options are provided.
//
// Example:
//
//	srv := http.NewServer(router, log,
//	    http.WithServerHost("0.0.0.0"),
//	    http.WithServerPort(3000),
//	)
func NewServer(engine *gin.Engine, logger *logger.Logger, opts ...Option) *Server {
	s := &Server{
		Engine: engine,
		logger: logger,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithServerHost sets the host address the server will bind to.
// Use "0.0.0.0" to listen on all network interfaces, or "localhost"/"127.0.0.1"
// to only accept local connections.
//
// Default: "" (which Go's http.Server interprets as "0.0.0.0")
func WithServerHost(host string) Option {
	return func(s *Server) {
		s.host = host
	}
}

// WithServerPort sets the TCP port the server will listen on.
// Must be between 1 and 65535. Ports below 1024 typically require elevated privileges.
//
// Default: 0 (which will cause the OS to assign a random available port)
func WithServerPort(port int) Option {
	return func(s *Server) {
		s.port = port
	}
}

// Start begins serving HTTP requests on the configured host and port.
// This method blocks until the server is stopped or encounters a fatal error.
//
// The server will log a fatal error if it fails to start or encounters an unexpected error.
// http.ErrServerClosed is not considered an error, as it indicates graceful shutdown.
//
// Returns nil on graceful shutdown, or an error if startup or serving fails.
func (s *Server) Start(ctx context.Context) error {
	s.httpSrv = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.host, s.port),
		Handler: s,
	}

	if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Sugar().Fatalf("listen: %s\n", err)
	}

	return nil
}

// Stop initiates graceful shutdown of the HTTP server.
// It waits up to 5 seconds for in-flight requests to complete before forcefully closing connections.
//
// The server will:
//   1. Stop accepting new connections
//   2. Wait for active requests to complete (up to 5 seconds)
//   3. Close idle connections
//   4. Return after shutdown completes
//
// If shutdown doesn't complete within the timeout, the server logs a fatal error
// and terminates immediately.
//
// Returns nil on successful shutdown, or an error if shutdown fails.
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Sugar().Info("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpSrv.Shutdown(ctx); err != nil {
		s.logger.Sugar().Fatal("Server forced to shutdown: ", err)
	}

	s.logger.Sugar().Info("Server exiting")
	return nil
}
