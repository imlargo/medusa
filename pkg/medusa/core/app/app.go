// Package app provides application lifecycle management for Medusa applications.
// It handles graceful startup and shutdown of servers, signal handling, and context propagation.
package app

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/imlargo/medusa/pkg/medusa/core/server"
)

// App represents a Medusa application with one or more servers.
// It manages the application lifecycle including graceful shutdown.
type App struct {
	name    string
	servers []server.Server
}

// Option is a functional option for configuring an App.
type Option func(a *App)

// NewApp creates a new Medusa application with the provided options.
func NewApp(opts ...Option) *App {
	a := &App{}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// WithServer adds one or more servers to the application.
// Multiple servers can run concurrently and will be managed together.
func WithServer(servers ...server.Server) Option {
	return func(a *App) {
		a.servers = servers
	}
}

// WithName sets the application name.
func WithName(name string) Option {
	return func(a *App) {
		a.name = name
	}
}

// Run starts the application and all configured servers.
// It blocks until a termination signal (SIGINT, SIGTERM) is received or the context is canceled.
// All servers are started concurrently and stopped gracefully on shutdown.
func (a *App) Run(ctx context.Context) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	for _, srv := range a.servers {
		go func(srv server.Server) {
			err := srv.Start(ctx)
			if err != nil {
				log.Printf("Server start err: %v", err)
			}
		}(srv)
	}

	select {
	case <-signals:
		// Received termination signal
		log.Println("Received termination signal")
	case <-ctx.Done():
		// Context canceled
		log.Println("Context canceled")
	}

	// Gracefully stop the servers
	for _, srv := range a.servers {
		err := srv.Stop(ctx)
		if err != nil {
			log.Printf("Server stop err: %v", err)
		}
	}

	return nil
}
