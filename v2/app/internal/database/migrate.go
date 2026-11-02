package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver goose runs on
	"github.com/pressly/goose/v3"
)

// migrations holds the versioned SQL files. They are also the schema sqlc
// generates the queries from, so the two cannot drift.
//
//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies every pending migration to the database at url and logs
// each one. It is run by cmd/migrate as its own deploy step, never by the API
// on startup, so replicas cannot race each other through the same migration.
func Migrate(ctx context.Context, url string, logger *slog.Logger) (err error) {
	files, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open postgres connection: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	results, err := provider.Up(ctx)
	for _, result := range results {
		logger.Info("migration applied",
			slog.Int64("version", result.Source.Version),
			slog.String("file", result.Source.Path),
			slog.Duration("duration", result.Duration),
		)
	}
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	if len(results) == 0 {
		logger.Info("schema already up to date")
	}

	return nil
}
