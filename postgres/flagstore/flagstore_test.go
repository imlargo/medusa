package flagstore_test

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/imlargo/medusa/postgres/flagstore"
	"github.com/imlargo/medusa/postgres/pgtest"
)

var noMigrations = fstest.MapFS{"00001_init.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}

func newStore(t *testing.T) *flagstore.Store {
	t.Helper()
	pool := pgtest.New(t, noMigrations)
	for range 2 { // idempotent: a second deploy changes nothing
		if err := flagstore.Install(t.Context(), pool); err != nil {
			t.Fatalf("Install() = %v", err)
		}
	}
	return flagstore.New(pool)
}

func TestSetLoadAndDelete(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := t.Context()

	if flags, err := store.Load(ctx); err != nil || len(flags) != 0 {
		t.Fatalf("Load() of an empty store = %v, %v, want none", flags, err)
	}

	if err := store.Set(ctx, "new_checkout", flagstore.Flag{Percentage: 10, Allow: []string{"acme"}}); err != nil {
		t.Fatalf("Set() = %v", err)
	}
	if err := store.Set(ctx, "dark_mode", flagstore.Flag{Enabled: true}); err != nil {
		t.Fatalf("Set() = %v", err)
	}
	// Raising a rollout replaces the flag.
	if err := store.Set(ctx, "new_checkout", flagstore.Flag{Percentage: 50, Allow: []string{"acme", "globex"}}); err != nil {
		t.Fatalf("Set() again = %v", err)
	}

	flags, err := store.Load(ctx)
	if err != nil || len(flags) != 2 {
		t.Fatalf("Load() = %v, %v, want two flags", flags, err)
	}
	if got := flags["new_checkout"]; got.Enabled || got.Percentage != 50 || !slices.Equal(got.Allow, []string{"acme", "globex"}) {
		t.Errorf("new_checkout = %+v, want 50%% with two allowed keys", got)
	}
	if got := flags["dark_mode"]; !got.Enabled || got.Percentage != 0 || len(got.Allow) != 0 {
		t.Errorf("dark_mode = %+v, want on for everyone", got)
	}

	if err := store.Delete(ctx, "dark_mode"); err != nil {
		t.Fatalf("Delete() = %v", err)
	}
	if err := store.Delete(ctx, "never_existed"); err != nil {
		t.Errorf("Delete() of a flag that does not exist = %v, want nil", err)
	}
	if flags, _ := store.Load(ctx); len(flags) != 1 {
		t.Errorf("flags after a delete = %v, want one", flags)
	}
}

func TestSetRefusesAPercentageOutsideItsRange(t *testing.T) {
	t.Parallel()
	store := newStore(t)

	for _, percentage := range []int{-1, 101} {
		if err := store.Set(t.Context(), "x", flagstore.Flag{Percentage: percentage}); err == nil {
			t.Errorf("Set() with percentage %d = nil, want an error", percentage)
		}
	}
	if err := store.Set(t.Context(), "", flagstore.Flag{}); err == nil {
		t.Error("Set() without a name = nil, want an error")
	}
}
