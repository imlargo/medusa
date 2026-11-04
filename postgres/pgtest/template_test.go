package pgtest

import (
	"testing"
	"testing/fstest"
)

// The template is named by the migrations' content, so changing any
// migration must give a new template instead of reusing a stale schema.
func TestHashFilesChangesWithTheMigrations(t *testing.T) {
	base := fstest.MapFS{"00001_init.sql": {Data: []byte("CREATE TABLE a ();")}}
	edited := fstest.MapFS{"00001_init.sql": {Data: []byte("CREATE TABLE b ();")}}
	added := fstest.MapFS{
		"00001_init.sql": {Data: []byte("CREATE TABLE a ();")},
		"00002_more.sql": {Data: []byte("CREATE TABLE c ();")},
	}

	sum := func(fsys fstest.MapFS) [32]byte {
		s, err := hashFiles(fsys)
		if err != nil {
			t.Fatalf("hashFiles() = %v", err)
		}
		return s
	}

	if sum(base) != sum(base) {
		t.Error("the same migrations hash differently")
	}
	if sum(base) == sum(edited) || sum(base) == sum(added) {
		t.Error("changed migrations hash the same, want a new template")
	}
}
