// Command migrate applies the pending database migrations and exits. Run it
// as its own deploy step, before the new API version starts.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/imlargo/medusa/postgres"
	"github.com/imlargo/medusa/postgres/queue"

	"github.com/imlargo/medusa/app/internal/config"
	"github.com/imlargo/medusa/app/internal/database/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// A deploy that is canceled mid-migration stops between statements
	// instead of being killed inside one.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := config.LoadDatabase()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	if err := postgres.Migrate(ctx, db.URL, migrations.FS, logger); err != nil {
		return err
	}

	// The job queue keeps its own table up to date, next to the app's schema.
	pool, err := postgres.Open(ctx, db.URL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	return queue.Install(ctx, pool)
}
