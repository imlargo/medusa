// Package lifecycle runs one or more servers and owns their lifecycle: start
// hooks, concurrent start, termination signals and an ordered, graceful
// shutdown.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Server is anything App can start and stop.
type Server interface {
	// Start serves until the server is stopped or ctx is canceled. It returns
	// nil on a graceful stop and an error if serving fails.
	Start(ctx context.Context) error

	// Stop shuts the server down gracefully, waiting for in-flight work within
	// its own deadline.
	Stop(ctx context.Context) error
}

// App runs servers and owns their lifecycle.
type App struct {
	name    string
	servers []Server
	onStart []func(ctx context.Context) error
	onStop  []func(ctx context.Context) error
	logger  *slog.Logger
}

// Option configures an App.
type Option func(a *App)

// New creates an application from the given options. Without WithLogger it
// logs through slog.Default().
func New(opts ...Option) *App {
	a := &App{logger: slog.Default()}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// WithServer adds servers. They run concurrently and are stopped together.
func WithServer(servers ...Server) Option {
	return func(a *App) { a.servers = append(a.servers, servers...) }
}

// WithName sets the application name used in lifecycle logs.
func WithName(name string) Option {
	return func(a *App) { a.name = name }
}

// WithLogger sets the logger for lifecycle events. Pass the one the rest of
// the application uses, so the process keeps a single log stream.
func WithLogger(l *slog.Logger) Option {
	return func(a *App) { a.logger = l }
}

// WithOnStart registers a hook run before the servers start, in registration
// order. The first hook to fail aborts startup and Run returns its error.
func WithOnStart(fn func(ctx context.Context) error) Option {
	return func(a *App) { a.onStart = append(a.onStart, fn) }
}

// WithOnStop registers a hook run during shutdown, before the servers stop, in
// registration order. Every hook runs even if an earlier one fails; their
// errors are collected.
func WithOnStop(fn func(ctx context.Context) error) Option {
	return func(a *App) { a.onStop = append(a.onStop, fn) }
}

// Run starts every server and blocks until SIGINT or SIGTERM arrives, ctx is
// canceled, or a server fails.
//
// It returns nil when the application shut down cleanly, and otherwise every
// failure joined together: the server error that triggered the shutdown, any
// onStop hook error, and any error from stopping a server.
//
// Lifecycle:
//  1. run onStart hooks in order, aborting on the first failure
//  2. start every server concurrently
//  3. wait for a signal, ctx cancellation, or a server failure
//  4. run every onStop hook
//  5. stop every server gracefully and wait for them to return
func (a *App) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.logger.Info("starting application", slog.String("app", a.name))

	for i, fn := range a.onStart {
		if err := fn(ctx); err != nil {
			return fmt.Errorf("onStart hook %d: %w", i, err)
		}
	}

	// Buffered so a failing server never blocks on send when nobody is
	// receiving any more.
	serverErrs := make(chan error, len(a.servers))

	var wg sync.WaitGroup
	for _, srv := range a.servers {
		wg.Go(func() {
			if err := srv.Start(ctx); err != nil {
				serverErrs <- err
			}
		})
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	var errs []error

	select {
	case sig := <-signals:
		a.logger.Info("termination signal received, shutting down", slog.String("signal", sig.String()))
	case <-ctx.Done():
		a.logger.Info("context canceled, shutting down")
	case err := <-serverErrs:
		a.logger.Error("server failed, shutting down", slog.Any("error", err))
		errs = append(errs, err)
		cancel()
	}

	// Shutdown must not inherit the cancellation that triggered it, or the
	// hooks and graceful drains below would be dead on arrival.
	shutdownCtx := context.WithoutCancel(ctx)

	for i, fn := range a.onStop {
		if err := fn(shutdownCtx); err != nil {
			a.logger.Error("onStop hook failed", slog.Int("hook", i), slog.Any("error", err))
			errs = append(errs, fmt.Errorf("onStop hook %d: %w", i, err))
		}
	}

	for _, srv := range a.servers {
		if err := srv.Stop(shutdownCtx); err != nil {
			a.logger.Error("server stop failed", slog.Any("error", err))
			errs = append(errs, err)
		}
	}

	wg.Wait()

	// Drain whatever the other servers reported while shutting down.
	close(serverErrs)
	for err := range serverErrs {
		errs = append(errs, err)
	}

	a.logger.Info("application stopped", slog.String("app", a.name))

	return errors.Join(errs...)
}
