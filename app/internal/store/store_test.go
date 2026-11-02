package store_test

import (
	"errors"
	"testing"

	"github.com/imlargo/medusa/postgres/pgtest"

	"github.com/imlargo/medusa/app/internal/database/migrations"
	"github.com/imlargo/medusa/app/internal/repositories"
	"github.com/imlargo/medusa/app/internal/store"
)

func TestWithTxCommitsWhenFnSucceeds(t *testing.T) {
	t.Parallel()
	s := store.New(pgtest.New(t, migrations.FS))
	ctx := t.Context()

	var id int64
	err := s.WithTx(ctx, func(tx *store.Store) error {
		user, err := tx.Users.Create(ctx, "commit@example.com", "hash")
		id = user.ID
		return err
	})
	if err != nil {
		t.Fatalf("WithTx() = %v, want nil", err)
	}

	if _, err := s.Users.Get(ctx, id); err != nil {
		t.Errorf("Get() after commit = %v, want the user persisted", err)
	}
}

func TestWithTxRollsBackWhenFnFails(t *testing.T) {
	t.Parallel()
	s := store.New(pgtest.New(t, migrations.FS))
	ctx := t.Context()
	boom := errors.New("boom")

	err := s.WithTx(ctx, func(tx *store.Store) error {
		if _, err := tx.Users.Create(ctx, "rollback@example.com", "hash"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("WithTx() = %v, want it to wrap %v", err, boom)
	}

	if _, err := s.Users.GetByEmail(ctx, "rollback@example.com"); !errors.Is(err, repositories.ErrNotFound) {
		t.Errorf("GetByEmail() after rollback = %v, want ErrNotFound", err)
	}
}

// A nested WithTx is a savepoint: its failure undoes only its own writes.
func TestNestedWithTxRollsBackOnlyTheSavepoint(t *testing.T) {
	t.Parallel()
	s := store.New(pgtest.New(t, migrations.FS))
	ctx := t.Context()

	err := s.WithTx(ctx, func(tx *store.Store) error {
		if _, err := tx.Users.Create(ctx, "outer@example.com", "hash"); err != nil {
			return err
		}

		nestedErr := tx.WithTx(ctx, func(nested *store.Store) error {
			if _, err := nested.Users.Create(ctx, "inner@example.com", "hash"); err != nil {
				return err
			}
			return errors.New("undo the inner write")
		})
		if nestedErr == nil {
			t.Error("nested WithTx() = nil, want its error")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx() = %v, want nil", err)
	}

	if _, err := s.Users.GetByEmail(ctx, "outer@example.com"); err != nil {
		t.Errorf("outer write = %v, want it committed", err)
	}
	if _, err := s.Users.GetByEmail(ctx, "inner@example.com"); !errors.Is(err, repositories.ErrNotFound) {
		t.Errorf("inner write = %v, want it rolled back", err)
	}
}
