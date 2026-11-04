package postgres

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestOpenKeepsThePasswordOutOfTheError(t *testing.T) {
	_, err := Open(t.Context(), "postgres://user:hunter2@localhost:not-a-port/db", slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("Open() = nil, want an error for an invalid URL")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("Open() error %q contains the password", err)
	}
}

func TestErrorClassification(t *testing.T) {
	unique := fmt.Errorf("create user: %w", &pgconn.PgError{Code: "23505"})
	other := &pgconn.PgError{Code: "23503"}
	noRows := fmt.Errorf("get user: %w", pgx.ErrNoRows)

	tests := []struct {
		name       string
		err        error
		wantNoRows bool
		wantUnique bool
	}{
		{name: "wrapped no rows", err: noRows, wantNoRows: true},
		{name: "wrapped unique violation", err: unique, wantUnique: true},
		{name: "other sqlstate", err: other},
		{name: "plain error", err: errors.New("boom")},
		{name: "nil", err: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNoRows(tt.err); got != tt.wantNoRows {
				t.Errorf("IsNoRows() = %v, want %v", got, tt.wantNoRows)
			}
			if got := IsUniqueViolation(tt.err); got != tt.wantUnique {
				t.Errorf("IsUniqueViolation() = %v, want %v", got, tt.wantUnique)
			}
		})
	}
}

func TestTracerLogsSlowQueriesWithoutArguments(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		tracer := &queryTracer{logger: slog.New(slog.NewTextHandler(&buf, nil))}

		trace := func(sql string, took time.Duration) {
			ctx := tracer.TraceQueryStart(t.Context(), nil, pgx.TraceQueryStartData{SQL: sql, Args: []any{"secret-arg"}})
			time.Sleep(took)
			tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
		}

		trace("SELECT fast", time.Millisecond)
		trace("SELECT slow", SlowQueryThreshold)

		out := buf.String()
		if strings.Contains(out, "SELECT fast") {
			t.Errorf("log %q includes a fast query", out)
		}
		if !strings.Contains(out, "slow query") || !strings.Contains(out, "SELECT slow") {
			t.Errorf("log %q is missing the slow query", out)
		}
		if strings.Contains(out, "secret-arg") {
			t.Errorf("log %q includes query arguments", out)
		}
	})
}

func TestTracerLogsEveryQueryWhenVerbose(t *testing.T) {
	var buf bytes.Buffer
	tracer := &queryTracer{logger: slog.New(slog.NewTextHandler(&buf, nil)), verbose: true}

	ctx := tracer.TraceQueryStart(t.Context(), nil, pgx.TraceQueryStartData{SQL: "SELECT 1", Args: []any{42}})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	if out := buf.String(); !strings.Contains(out, "SELECT 1") || !strings.Contains(out, "42") {
		t.Errorf("log %q, want the statement and its arguments", out)
	}
}

// A deployment tunes the pool from the URL; Medusa's defaults fill only what
// the URL leaves out.
func TestPoolSettingsInTheURLWinOverTheDefaults(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	defaults, err := poolConfig("postgres://u:p@localhost/db", logger)
	if err != nil {
		t.Fatalf("poolConfig() = %v", err)
	}
	if defaults.MaxConns != DefaultMaxConns || defaults.MaxConnLifetime != DefaultConnMaxLifetime {
		t.Errorf("MaxConns, MaxConnLifetime = %d, %s, want the defaults", defaults.MaxConns, defaults.MaxConnLifetime)
	}

	tuned, err := poolConfig("postgres://u:p@localhost/db?pool_max_conns=7&pool_max_conn_lifetime=5m", logger)
	if err != nil {
		t.Fatalf("poolConfig() = %v", err)
	}
	if tuned.MaxConns != 7 || tuned.MaxConnLifetime != 5*time.Minute {
		t.Errorf("MaxConns, MaxConnLifetime = %d, %s, want 7, 5m from the url", tuned.MaxConns, tuned.MaxConnLifetime)
	}
	if tuned.MaxConnIdleTime != DefaultConnMaxIdleTime {
		t.Errorf("MaxConnIdleTime = %s, want the default", tuned.MaxConnIdleTime)
	}
}
