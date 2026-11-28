package store_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/postgres/pgtest"
	"github.com/imlargo/medusa/postgres/queue"

	"github.com/imlargo/medusa/app/internal/database/migrations"
	"github.com/imlargo/medusa/app/internal/session"
	"github.com/imlargo/medusa/app/internal/store"
	"github.com/imlargo/medusa/app/internal/user"
)

func setup(t *testing.T) (*store.Store, *user.Repository, *session.Repository, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.New(t, migrations.FS)
	if err := queue.Install(t.Context(), pool); err != nil {
		t.Fatalf("install the job queue: %v", err)
	}
	return store.New(pool), user.NewRepository(pool), session.NewRepository(pool), pool
}

func countJobs(t *testing.T, pool *pgxpool.Pool) (n int) {
	t.Helper()
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM medusa_jobs").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Two features that do not know each other, and a job, in one transaction:
// the store imports neither feature, the transaction travels in the context.
func TestWithTxSpansFeaturesAndJobs(t *testing.T) {
	t.Parallel()
	s, users, sessions, pool := setup(t)
	ctx := t.Context()

	write := func(ctx context.Context, email string) error {
		created, err := users.Create(ctx, email, "hash")
		if err != nil {
			return err
		}
		token := auth.RefreshToken{Hash: "h-" + email, Family: "f", Subject: strconv.FormatInt(created.ID, 10), ExpiresAt: time.Now().Add(time.Hour)}
		if err := sessions.Create(ctx, token); err != nil {
			return err
		}
		return s.Enqueue(ctx, "account.welcome", map[string]int64{"user_id": created.ID})
	}

	boom := errors.New("boom")
	err := s.WithTx(ctx, func(ctx context.Context) error {
		if err := write(ctx, "rollback@example.com"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("WithTx() = %v, want it to wrap %v", err, boom)
	}
	if _, err := users.GetByEmail(ctx, "rollback@example.com"); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("the user after a rollback = %v, want ErrNotFound", err)
	}
	if _, err := sessions.Consume(ctx, "h-rollback@example.com"); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Errorf("the session after a rollback = %v, want it gone", err)
	}
	if n := countJobs(t, pool); n != 0 {
		t.Errorf("jobs after a rollback = %d, want 0", n)
	}

	if err := s.WithTx(ctx, func(ctx context.Context) error { return write(ctx, "commit@example.com") }); err != nil {
		t.Fatalf("WithTx() = %v, want nil", err)
	}
	if _, err := users.GetByEmail(ctx, "commit@example.com"); err != nil {
		t.Errorf("the user after a commit = %v, want it stored", err)
	}
	if _, err := sessions.Consume(ctx, "h-commit@example.com"); err != nil {
		t.Errorf("the session after a commit = %v, want it stored", err)
	}
	if n := countJobs(t, pool); n != 1 {
		t.Errorf("jobs after a commit = %d, want 1", n)
	}
}

// A nested WithTx is a savepoint: its failure undoes only its own writes.
func TestNestedWithTxRollsBackOnlyTheSavepoint(t *testing.T) {
	t.Parallel()
	s, users, _, _ := setup(t)
	ctx := t.Context()

	err := s.WithTx(ctx, func(ctx context.Context) error {
		if _, err := users.Create(ctx, "outer@example.com", "hash"); err != nil {
			return err
		}

		nestedErr := s.WithTx(ctx, func(ctx context.Context) error {
			if _, err := users.Create(ctx, "inner@example.com", "hash"); err != nil {
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

	if _, err := users.GetByEmail(ctx, "outer@example.com"); err != nil {
		t.Errorf("outer write = %v, want it committed", err)
	}
	if _, err := users.GetByEmail(ctx, "inner@example.com"); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("inner write = %v, want it rolled back", err)
	}
}
