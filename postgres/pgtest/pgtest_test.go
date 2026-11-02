package pgtest_test

import (
	"testing"
	"testing/fstest"

	"github.com/imlargo/medusa/postgres"
	"github.com/imlargo/medusa/postgres/pgtest"
)

var migrations = fstest.MapFS{
	"00001_create_items.sql": {Data: []byte(`-- +goose Up
CREATE TABLE items (name TEXT PRIMARY KEY);
`)},
}

func TestNewReturnsAMigratedDatabase(t *testing.T) {
	t.Parallel()
	pool := pgtest.New(t, migrations)
	ctx := t.Context()

	if _, err := pool.Exec(ctx, "INSERT INTO items (name) VALUES ('a')"); err != nil {
		t.Fatalf("insert into the migrated table: %v", err)
	}

	_, err := pool.Exec(ctx, "INSERT INTO items (name) VALUES ('a')")
	if !postgres.IsUniqueViolation(err) {
		t.Errorf("duplicate insert = %v, want a unique violation", err)
	}

	var name string
	err = pool.QueryRow(ctx, "SELECT name FROM items WHERE name = 'missing'").Scan(&name)
	if !postgres.IsNoRows(err) {
		t.Errorf("missing row = %v, want no rows", err)
	}
}

// Every call gets its own database, so parallel tests never see each other's rows.
func TestNewIsolatesEachTest(t *testing.T) {
	t.Parallel()
	pool := pgtest.New(t, migrations)

	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM items").Scan(&count); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if count != 0 {
		t.Errorf("items = %d, want 0 in a fresh database", count)
	}
}
