package postgres

import (
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"
)

func TestMergeMigrationsGivesOneFlatHistory(t *testing.T) {
	users := fstest.MapFS{
		"migrations/20261101000000_create_users.sql": {Data: []byte("-- users")},
		"migrations/README.md":                       {Data: []byte("not a migration")},
		"repository.go":                              {Data: []byte("package user")},
	}
	// A source can also hold its files at the root.
	billing := fstest.MapFS{"20261102000000_create_invoices.sql": {Data: []byte("-- invoices")}}

	merged, err := MergeMigrations(users, billing)
	if err != nil {
		t.Fatalf("MergeMigrations() = %v", err)
	}

	// What goose does to find them.
	names, err := fs.Glob(merged, "*.sql")
	if err != nil {
		t.Fatalf("Glob() = %v", err)
	}
	slices.Sort(names)
	if want := []string{"20261101000000_create_users.sql", "20261102000000_create_invoices.sql"}; !slices.Equal(names, want) {
		t.Errorf("migrations = %v, want %v", names, want)
	}

	content, err := fs.ReadFile(merged, "20261102000000_create_invoices.sql")
	if err != nil || string(content) != "-- invoices" {
		t.Errorf("ReadFile() = %q, %v, want the file's content", content, err)
	}

	// The standard library's own check of an fs.FS implementation.
	if err := fstest.TestFS(merged, names...); err != nil {
		t.Errorf("the merged file system is not a valid fs.FS: %v", err)
	}
}

func TestMergeMigrationsRefusesTwoFilesWithOneName(t *testing.T) {
	a := fstest.MapFS{"migrations/00001_init.sql": {Data: []byte("-- a")}}
	b := fstest.MapFS{"migrations/00001_init.sql": {Data: []byte("-- b")}}

	if _, err := MergeMigrations(a, b); err == nil {
		t.Error("MergeMigrations() = nil, want an error: one of the two would be lost")
	}
}
