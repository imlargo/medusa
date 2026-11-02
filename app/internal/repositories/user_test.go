package repositories_test

import (
	"errors"
	"testing"

	"github.com/imlargo/medusa/app/internal/database/dbtest"
	"github.com/imlargo/medusa/app/internal/database/sqlc"
	"github.com/imlargo/medusa/app/internal/repositories"
)

func newUsers(t *testing.T) repositories.UserRepository {
	t.Helper()
	return repositories.NewUserRepository(sqlc.New(dbtest.New(t)))
}

func TestUserRepositoryRoundTrip(t *testing.T) {
	t.Parallel()
	users := newUsers(t)
	ctx := t.Context()

	created, err := users.Create(ctx, "ada@example.com", "hash")
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if created.ID == 0 || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Errorf("Create() = %+v, want the id and timestamps set by the database", created)
	}

	byID, err := users.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() = %v, want nil", err)
	}
	if byID.Email != "ada@example.com" || byID.Password != "hash" {
		t.Errorf("Get() = %+v, want the stored email and hash", byID)
	}

	byEmail, err := users.GetByEmail(ctx, "ada@example.com")
	if err != nil {
		t.Fatalf("GetByEmail() = %v, want nil", err)
	}
	if byEmail.ID != created.ID {
		t.Errorf("GetByEmail().ID = %d, want %d", byEmail.ID, created.ID)
	}

	if err := users.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() = %v, want nil", err)
	}
	if _, err := users.Get(ctx, created.ID); !errors.Is(err, repositories.ErrNotFound) {
		t.Errorf("Get() after Delete = %v, want ErrNotFound", err)
	}
}

// Driver errors never leave the repository: callers branch on sentinels.
func TestUserRepositoryTranslatesDriverErrors(t *testing.T) {
	t.Parallel()
	users := newUsers(t)
	ctx := t.Context()

	if _, err := users.GetByEmail(ctx, "nobody@example.com"); !errors.Is(err, repositories.ErrNotFound) {
		t.Errorf("GetByEmail(missing) = %v, want ErrNotFound", err)
	}

	if _, err := users.Create(ctx, "taken@example.com", "hash"); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := users.Create(ctx, "taken@example.com", "hash"); !errors.Is(err, repositories.ErrDuplicate) {
		t.Errorf("Create(duplicate) = %v, want ErrDuplicate", err)
	}
}
