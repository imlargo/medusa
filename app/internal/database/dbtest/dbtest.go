// Package dbtest gives integration tests a real, migrated Postgres database.
//
// Each call to New creates a fresh database on the server at
// TEST_DATABASE_URL, applies every migration and drops it when the test ends,
// so tests never share rows and can run in parallel:
//
//	func TestCreateUser(t *testing.T) {
//	    pool := dbtest.New(t)
//	    users := repositories.NewUserRepository(sqlc.New(pool))
//	    ...
//	}
//
// Without TEST_DATABASE_URL the test is skipped. Set REQUIRE_DB_TESTS=1 in CI
// so a missing database fails the run instead of skipping it silently.
package dbtest

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imlargo/medusa/app/internal/database"
)

const (
	// envURL points at a Postgres server the tests may create databases on,
	// for example postgres://postgres:postgres@localhost:5432/postgres.
	envURL = "TEST_DATABASE_URL"

	// envRequire turns a missing envURL into a failure instead of a skip.
	envRequire = "REQUIRE_DB_TESTS"

	// setupTimeout bounds creating and migrating one database.
	setupTimeout = 30 * time.Second
)

// New returns a pool connected to a new database with every migration
// applied. The pool is closed and the database dropped when t ends.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()

	serverURL := os.Getenv(envURL)
	if serverURL == "" {
		if os.Getenv(envRequire) == "1" {
			t.Fatalf("%s is not set and %s=1 requires database tests to run", envURL, envRequire)
		}
		t.Skipf("set %s to run database tests", envURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatalf("connect to %s: %v", envURL, err)
	}
	defer func() { _ = admin.Close(context.Background()) }()

	name := "test_" + strings.ToLower(rand.Text())
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { dropDatabase(t, serverURL, ident) })

	dbURL, err := withDatabase(serverURL, name)
	if err != nil {
		t.Fatalf("build test database url: %v", err)
	}

	logger := slog.New(slog.DiscardHandler)
	if err := database.Migrate(ctx, dbURL, logger); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	pool, err := database.NewPostgres(ctx, dbURL, logger)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// withDatabase returns serverURL pointed at the database name.
func withDatabase(serverURL, name string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		// The parse error would quote the URL, and the URL carries the password.
		return "", fmt.Errorf("%s is not a valid URL", envURL)
	}
	u.Path = "/" + name
	return u.String(), nil
}

// dropDatabase removes a test database, closing any connection a test leaked.
func dropDatabase(t testing.TB, serverURL, ident string) {
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Errorf("connect to drop test database: %v", err)
		return
	}
	defer func() { _ = admin.Close(context.Background()) }()

	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
		t.Errorf("drop test database: %v", err)
	}
}
