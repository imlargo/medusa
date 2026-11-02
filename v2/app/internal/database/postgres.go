// Package database opens the connections the application depends on and
// applies the schema migrations.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
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

	// slowQueryThreshold is when a statement is logged as slow.
	slowQueryThreshold = 200 * time.Millisecond
)

// PostgresOption configures the pool.
type PostgresOption func(*postgresConfig)

type postgresConfig struct {
	logQueries bool
}

// WithQueryLogging logs every statement with its arguments. Development only:
// the arguments are user data.
func WithQueryLogging(enabled bool) PostgresOption {
	return func(c *postgresConfig) { c.logQueries = enabled }
}

// NewPostgres opens a connection pool and pings it, so a bad URL or an
// unreachable host fails at startup rather than on the first request.
func NewPostgres(ctx context.Context, url string, logger *slog.Logger, opts ...PostgresOption) (*pgxpool.Pool, error) {
	var cfg postgresConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	poolCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// The parse error can quote the URL, and the URL carries the password.
		return nil, fmt.Errorf("parse postgres url: invalid connection string")
	}

	poolCfg.MaxConns = DefaultMaxConns
	poolCfg.MaxConnLifetime = DefaultConnMaxLifetime
	poolCfg.MaxConnIdleTime = DefaultConnMaxIdleTime
	poolCfg.ConnConfig.Tracer = &queryTracer{logger: logger, verbose: cfg.logQueries}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
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
	case elapsed >= slowQueryThreshold:
		// Without the arguments: outside development they are user data.
		t.logger.WarnContext(ctx, "slow query",
			slog.String("sql", start.sql),
			slog.Duration("duration", elapsed),
		)
	}
}
