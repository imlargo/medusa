// Package server defines the core server interface and abstractions for Medusa.
//
// The Server interface provides a standard contract for server lifecycle management,
// allowing different server implementations (HTTP, gRPC, etc.) to be managed uniformly
// by the application framework.
//
// All servers must implement graceful startup and shutdown, respecting context
// cancellation for coordinated termination.
package server

import (
	"context"
	"net/url"
)

// Server represents any server that can be started and stopped.
// Implementations must respect context cancellation and implement graceful shutdown.
//
// The Start method should block until the server is stopped or an error occurs.
// The Stop method should initiate graceful shutdown and wait for in-flight requests
// to complete (within a reasonable timeout).
type Server interface {
	// Start begins serving requests. This method should block until the server
	// is stopped via Stop() or the context is cancelled. Returns an error if
	// the server fails to start or encounters a fatal error while running.
	Start(context.Context) error

	// Stop initiates graceful shutdown of the server. It should wait for
	// in-flight requests to complete (within the context deadline) before returning.
	// Returns an error if shutdown fails or times out.
	Stop(context.Context) error
}

// Endpointer is an interface for servers that can provide their network endpoint.
// This is useful for service discovery and health checking.
type Endpointer interface {
	// Endpoint returns the server's network endpoint as a URL.
	// Returns an error if the endpoint cannot be determined.
	Endpoint() (*url.URL, error)
}
