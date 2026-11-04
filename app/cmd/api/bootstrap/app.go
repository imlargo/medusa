// Package bootstrap assembles the API: it loads the configuration, wires every
// dependency into a Container, builds the router and returns a runnable App.
package bootstrap

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/imlargo/medusa/httpserver"
	"github.com/imlargo/medusa/lifecycle"

	"github.com/imlargo/medusa/app/internal/config"
	"github.com/imlargo/medusa/app/internal/hosturl"
)

// App is the application lifecycle together with the container holding its
// dependencies. Close releases them once Run returns.
type App struct {
	*lifecycle.App

	Container *Container
}

// New builds a fully featured application: database, Redis, object storage and
// metrics, as far as the environment configures them.
//
// ctx bounds the connectivity checks made while dialing dependencies, so a
// canceled context aborts a slow startup instead of hanging.
func New(ctx context.Context, name string) (*App, error) {
	return NewWithOptions(ctx, name, DefaultOptions())
}

// NewMinimal builds a lightweight application with only the database and JWT.
func NewMinimal(ctx context.Context, name string) (*App, error) {
	return NewWithOptions(ctx, name, MinimalOptions())
}

// NewWithOptions builds an application with an explicit set of optional
// components. It returns an error if the configuration is invalid or any
// required dependency is unreachable.
func NewWithOptions(ctx context.Context, name string, opts Options) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	container, err := NewContainer(ctx, cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("initialize %s: %w", name, err)
	}

	server := httpserver.New(
		newRouter(container),
		container.Logger,
		httpserver.WithHost(cfg.Server.Host),
		httpserver.WithPort(cfg.Server.Port),
		// The remainder of the shutdown budget, after the stream drain below has
		// taken its slice.
		httpserver.WithShutdownTimeout(cfg.Runtime.ServerShutdownTimeout()),
	)

	printBanner(os.Stdout, name, cfg)

	return &App{
		App: lifecycle.New(
			lifecycle.WithName(name),
			lifecycle.WithServer(server),
			lifecycle.WithLogger(container.Logger),
			// Both run before the HTTP server stops, in this order: first
			// the load balancer stops sending requests, then open SSE
			// streams end on their own instead of holding the shutdown open.
			lifecycle.WithOnStop(container.LeaveRotation),
			lifecycle.WithOnStop(container.DrainEventStreams),
		),
		Container: container,
	}, nil
}

// Close releases every resource held by the application. Defer it right after
// construction; calling it more than once is safe.
func (a *App) Close() error {
	if a == nil || a.Container == nil {
		return nil
	}
	return a.Container.Close()
}

// printBanner writes a short startup summary with the URLs the app serves.
func printBanner(w io.Writer, name string, cfg *config.Config) {
	const separator = "─────────────────────────────────"

	docs := "disabled"
	if cfg.Runtime.DocsEnabled {
		docs = hosturl.DocsURL(cfg.Server.Host, cfg.Server.Port)
	}

	fields := [][2]string{
		{"App", name},
		{"Env", string(cfg.Environment)},
		{"Server", hosturl.AppURL(cfg.Server.Host, cfg.Server.Port)},
		{"Docs", docs},
	}

	var banner strings.Builder
	fmt.Fprintf(&banner, "\n🪼 Medusa\n%s\n", separator)
	for _, field := range fields {
		fmt.Fprintf(&banner, "   %-7s %s\n", field[0]+":", field[1])
	}
	fmt.Fprintf(&banner, "%s\n\n", separator)

	// Nothing actionable if the banner cannot be written.
	_, _ = io.WriteString(w, banner.String())
}
