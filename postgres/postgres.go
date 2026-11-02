// Package postgres is Medusa's Postgres battery: a pgx connection pool with
// query logging, embedded goose migrations, and helpers that classify driver
// errors.
//
// It is a separate module, so an app that does not use Postgres never
// downloads pgx or goose:
//
//	pool, err := postgres.Open(ctx, cfg.DatabaseURL, logger)
//	...
//	err = postgres.Migrate(ctx, cfg.DatabaseURL, migrations.FS, logger)
//
// Queries are the app's own (sqlc, or pgx directly). Conn is the interface
// they run on, so the same code works on the pool and inside a transaction.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connection pool defaults.
const (
	// DefaultMaxConns caps concurrent connections. Keep the total across all
	// replicas below the server's max_connections.
	DefaultMaxConns = 25

	// DefaultConnMaxLifetime recycles connections so a load balancer or a
	// failover never leaves the pool holding a dead one indefinitely.
	DefaultConnMaxLifetime = time.Hour

	// DefaultConnMaxIdleTime closes connections nobody is using.
	DefaultConnMaxIdleTime = 10 * time.Minute

	// SlowQueryThreshold is when a statement is logged as slow.
	SlowQueryThreshold = 200 * time.Millisecond
)

// Conn is what queries run on: a *pgxpool.Pool, or a pgx.Tx opened on it.
// It satisfies the DBTX interface sqlc generates, and Begin opens a
// transaction on the pool or a savepoint inside a transaction, so
// pgx.BeginFunc(ctx, conn, fn) nests correctly.
type Conn interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

var (
	_ Conn = (*pgxpool.Pool)(nil)
	_ Conn = pgx.Tx(nil)
)

// Option configures Open.
type Option func(*options)

type options struct {
	logQueries bool
}

// WithQueryLogging logs every statement with its arguments. Development only:
// the arguments are user data.
func WithQueryLogging(enabled bool) Option {
	return func(o *options) { o.logQueries = enabled }
}

// Open opens a connection pool and pings it, so a bad URL or an unreachable
// host fails at startup rather than on the first request. Statements slower
// than SlowQueryThreshold are logged as warnings.
func Open(ctx context.Context, url string, logger *slog.Logger, opts ...Option) (*pgxpool.Pool, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// The parse error can quote the URL, and the URL carries the password.
		return nil, errors.New("parse postgres url: invalid connection string")
	}

	cfg.MaxConns = DefaultMaxConns
	cfg.MaxConnLifetime = DefaultConnMaxLifetime
	cfg.MaxConnIdleTime = DefaultConnMaxIdleTime
	cfg.ConnConfig.Tracer = &queryTracer{logger: logger, verbose: o.logQueries}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}

// codeUniqueViolation is the SQLSTATE of a broken unique constraint.
const codeUniqueViolation = "23505"

// IsNoRows reports whether err means a query returned no row.
func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// IsUniqueViolation reports whether err is a broken unique constraint.
func IsUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == codeUniqueViolation
}

// queryTracer logs slow statements always, and every statement when verbose.
type queryTracer struct {
	logger  *slog.Logger
	verbose bool
}

type queryStartKey struct{}

type queryStart struct {
	at   time.Time
	sql  string
	args []any
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryStartKey{}, queryStart{at: time.Now(), sql: data.SQL, args: data.Args})
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	start, ok := ctx.Value(queryStartKey{}).(queryStart)
	if !ok {
		return
	}

	elapsed := time.Since(start.at)

	switch {
	case t.verbose:
		t.logger.InfoContext(ctx, "query",
			slog.String("sql", start.sql),
			slog.Any("args", start.args),
			slog.Duration("duration", elapsed),
		)
	case elapsed >= SlowQueryThreshold:
		// Without the arguments: outside development they are user data.
		t.logger.WarnContext(ctx, "slow query",
			slog.String("sql", start.sql),
			slog.Duration("duration", elapsed),
		)
	}
}
