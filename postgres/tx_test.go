package postgres_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/imlargo/medusa/postgres"
	"github.com/imlargo/medusa/postgres/pgtest"
)

var notes = fstest.MapFS{"00001_notes.sql": {Data: []byte("-- +goose Up\nCREATE TABLE notes (body text PRIMARY KEY);\n")}}

// repository stands for a feature's storage: it has only the pool, and joins
// whatever transaction its context carries.
type repository struct{ pool postgres.Conn }

func (r repository) add(ctx context.Context, body string) error {
	_, err := postgres.ConnFrom(ctx, r.pool).Exec(ctx, "INSERT INTO notes (body) VALUES ($1)", body)
	return err
}

func (r repository) count(ctx context.Context) (n int, err error) {
	err = postgres.ConnFrom(ctx, r.pool).QueryRow(ctx, "SELECT count(*) FROM notes").Scan(&n)
	return n, err
}

// Two repositories that know nothing of each other share one transaction,
// because it travels in the context.
func TestTransactJoinsEveryRepositoryThatGetsItsContext(t *testing.T) {
	t.Parallel()
	pool := pgtest.New(t, notes)
	ctx := t.Context()
	a, b := repository{pool}, repository{pool}

	failed := errors.New("the second write is refused")
	err := postgres.Transact(ctx, pool, func(ctx context.Context) error {
		if err := a.add(ctx, "from a"); err != nil {
			return err
		}
		if n, _ := b.count(ctx); n != 1 {
			t.Errorf("b sees %d notes inside the transaction, want a's 1", n)
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("Transact() = %v, want the error fn returned", err)
	}
	if n, _ := a.count(ctx); n != 0 {
		t.Errorf("notes after a rollback = %d, want 0", n)
	}

	if err := postgres.Transact(ctx, pool, func(ctx context.Context) error {
		if err := a.add(ctx, "from a"); err != nil {
			return err
		}
		return b.add(ctx, "from b")
	}); err != nil {
		t.Fatalf("Transact() = %v", err)
	}
	if n, _ := a.count(ctx); n != 2 {
		t.Errorf("notes after a commit = %d, want 2", n)
	}
}

// A function that asks for atomicity inside a caller's transaction gets a
// savepoint: its failure undoes its own writes, not the caller's.
func TestTransactInsideATransactionIsASavepoint(t *testing.T) {
	t.Parallel()
	pool := pgtest.New(t, notes)
	ctx := t.Context()
	repo := repository{pool}

	err := postgres.Transact(ctx, pool, func(ctx context.Context) error {
		if err := repo.add(ctx, "outer"); err != nil {
			return err
		}

		inner := postgres.Transact(ctx, pool, func(ctx context.Context) error {
			if err := repo.add(ctx, "inner"); err != nil {
				return err
			}
			return errors.New("the inner step fails")
		})
		if inner == nil {
			t.Error("the inner Transact() = nil, want its error")
		}
		return nil // the caller carries on without the inner step
	})
	if err != nil {
		t.Fatalf("Transact() = %v", err)
	}

	if n, _ := repo.count(ctx); n != 1 {
		t.Errorf("notes = %d, want only the outer one", n)
	}
}

// A panic must not leave a transaction open, or commit half of it.
func TestTransactRollsBackOnAPanic(t *testing.T) {
	t.Parallel()
	pool := pgtest.New(t, notes)
	ctx := t.Context()
	repo := repository{pool}

	func() {
		defer func() { _ = recover() }()
		_ = postgres.Transact(ctx, pool, func(ctx context.Context) error {
			_ = repo.add(ctx, "before the panic")
			panic("boom")
		})
	}()

	if n, _ := repo.count(ctx); n != 0 {
		t.Errorf("notes after a panic = %d, want 0", n)
	}
}
