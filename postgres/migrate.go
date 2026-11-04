package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver goose runs on
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Migrate applies every pending goose migration in migrations to the
// database at url and logs each one. The .sql files sit at the root of
// migrations, which is usually an embed.FS next to them:
//
//	//go:embed *.sql
//	var FS embed.FS
//
// Run it as its own deploy step, never on API startup. Two runs at once (two
// deploys, or a retried job) are serialized by a Postgres advisory lock, so
// the second waits and then finds nothing to apply. The lock lives in the
// session, so the URL must not go through a pooler in transaction mode.
func Migrate(ctx context.Context, url string, migrations fs.FS, logger *slog.Logger) (err error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open postgres connection: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("build migration lock: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	results, err := provider.Up(ctx)
	for _, result := range results {
		logger.InfoContext(ctx, "migration applied",
			slog.Int64("version", result.Source.Version),
			slog.String("file", result.Source.Path),
			slog.Duration("duration", result.Duration),
		)
	}
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	if len(results) == 0 {
		logger.InfoContext(ctx, "schema already up to date")
	}

	return nil
}
