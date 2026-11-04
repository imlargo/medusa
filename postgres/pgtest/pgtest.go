// Package pgtest gives integration tests a real, migrated Postgres database.
//
// Each call to New creates a fresh database on the server at
// TEST_DATABASE_URL and drops it when the test ends, so tests never share
// rows and can run in parallel. The database is cloned from a template
// migrated once per set of migrations, so a test pays for a copy, not for
// running every migration again:
//
//	func TestCreateUser(t *testing.T) {
//	    pool := pgtest.New(t, migrations.FS)
//	    users := repositories.NewUserRepository(sqlc.New(pool))
//	    ...
//	}
//
// Without TEST_DATABASE_URL the test is skipped. Set REQUIRE_DB_TESTS=1 in CI
// so a missing database fails the run instead of skipping it silently.
package pgtest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imlargo/medusa/postgres"
)

const (
	// EnvURL points at a Postgres server the tests may create databases on,
	// for example postgres://postgres:postgres@localhost:5432/postgres.
	EnvURL = "TEST_DATABASE_URL"

	// EnvRequire, set to 1, turns a missing EnvURL into a failure instead of
	// a skip.
	EnvRequire = "REQUIRE_DB_TESTS"

	// setupTimeout bounds creating and migrating one database.
	setupTimeout = 30 * time.Second
)

// New returns a pool connected to a new database with every migration in
// migrations applied (see postgres.Migrate). The pool is closed and the
// database dropped when t ends.
func New(t testing.TB, migrations fs.FS) *pgxpool.Pool {
	t.Helper()

	serverURL := os.Getenv(EnvURL)
	if serverURL == "" {
		if os.Getenv(EnvRequire) == "1" {
			t.Fatalf("%s is not set and %s=1 requires database tests to run", EnvURL, EnvRequire)
		}
		t.Skipf("set %s to run database tests", EnvURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatalf("connect to %s: %v", EnvURL, err)
	}
	defer func() { _ = admin.Close(context.Background()) }()

	template, err := migratedTemplate(ctx, admin, serverURL, migrations)
	if err != nil {
		t.Fatalf("prepare template database: %v", err)
	}

	name := "test_" + strings.ToLower(rand.Text())
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident+" TEMPLATE "+pgx.Identifier{template}.Sanitize()); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { dropDatabase(t, serverURL, ident) })

	dbURL, err := withDatabase(serverURL, name)
	if err != nil {
		t.Fatalf("build test database url: %v", err)
	}

	pool, err := postgres.Open(ctx, dbURL, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// templates holds the template databases this process has already checked.
var templates sync.Map

// migratedTemplate returns the name of a database with every migration in
// migrations applied, creating it on first use. Its name is a hash of the
// migration files, so editing or adding a migration builds a new template.
//
// Test binaries run in parallel, so creation holds an advisory lock, and the
// template is migrated under a temporary name and renamed when complete: a
// run that dies midway never leaves a half-migrated template behind.
func migratedTemplate(ctx context.Context, admin *pgx.Conn, serverURL string, migrations fs.FS) (string, error) {
	sum, err := hashFiles(migrations)
	if err != nil {
		return "", fmt.Errorf("read migrations: %w", err)
	}
	name := "pgtest_template_" + hex.EncodeToString(sum[:6])
	if _, ok := templates.Load(name); ok {
		return name, nil
	}

	lockKey := int64(binary.BigEndian.Uint64(sum[:8])) //nolint:gosec // G115: any 64 bits make a lock key
	if _, err := admin.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return "", fmt.Errorf("lock: %w", err)
	}
	defer func() { _, _ = admin.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", lockKey) }()

	var exists bool
	if err := admin.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return "", fmt.Errorf("look up %s: %w", name, err)
	}
	if !exists {
		if err := buildTemplate(ctx, admin, serverURL, name, migrations); err != nil {
			return "", err
		}
	}

	templates.Store(name, true)
	return name, nil
}

func buildTemplate(ctx context.Context, admin *pgx.Conn, serverURL, name string, migrations fs.FS) error {
	building := name + "_" + strings.ToLower(rand.Text())[:8]
	ident := pgx.Identifier{building}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		return fmt.Errorf("create %s: %w", building, err)
	}

	buildingURL, err := withDatabase(serverURL, building)
	if err == nil {
		err = postgres.Migrate(ctx, buildingURL, migrations, slog.New(slog.DiscardHandler))
	}
	if err == nil {
		_, err = admin.Exec(ctx, "ALTER DATABASE "+ident+" RENAME TO "+pgx.Identifier{name}.Sanitize())
	}
	if err != nil {
		_, _ = admin.Exec(context.WithoutCancel(ctx), "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)")
		return fmt.Errorf("build %s: %w", name, err)
	}
	return nil
}

// hashFiles hashes every file name and content in fsys, in walk order.
func hashFiles(fsys fs.FS) ([sha256.Size]byte, error) {
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", path, len(content))
		_, _ = h.Write(content)
		return nil
	})

	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum, err
}

// withDatabase returns serverURL pointed at the database name.
func withDatabase(serverURL, name string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		// The parse error would quote the URL, and the URL carries the password.
		return "", fmt.Errorf("%s is not a valid URL", EnvURL)
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
