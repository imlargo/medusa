package user_test

import (
	"errors"
	"testing"

	"github.com/imlargo/medusa/postgres/pgtest"

	"github.com/imlargo/medusa/app/internal/database/migrations"
	"github.com/imlargo/medusa/app/internal/database/sqlc"
	"github.com/imlargo/medusa/app/internal/user"
)

func newUsers(t *testing.T) *user.Repository {
	t.Helper()
	return user.NewRepository(sqlc.New(pgtest.New(t, migrations.FS)))
}

func TestRepositoryRoundTrip(t *testing.T) {
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
	if _, err := users.Get(ctx, created.ID); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("Get() after Delete = %v, want ErrNotFound", err)
	}
}

// Driver errors never leave the repository: callers branch on the package's
// errors.
func TestRepositoryTranslatesDriverErrors(t *testing.T) {
	t.Parallel()
	users := newUsers(t)
	ctx := t.Context()

	if _, err := users.GetByEmail(ctx, "nobody@example.com"); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("GetByEmail(missing) = %v, want ErrNotFound", err)
	}

	if _, err := users.Create(ctx, "taken@example.com", "hash"); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := users.Create(ctx, "taken@example.com", "hash"); !errors.Is(err, user.ErrEmailTaken) {
		t.Errorf("Create(duplicate) = %v, want ErrEmailTaken", err)
	}
}

// Emails are stored and looked up normalized, so letter case and spaces never
// create a second account or hide an existing one.
func TestRepositoryNormalizesEmails(t *testing.T) {
	t.Parallel()
	users := newUsers(t)
	ctx := t.Context()

	created, err := users.Create(ctx, "  Grace@Example.com ", "hash")
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if created.Email != "grace@example.com" {
		t.Errorf("stored email = %q, want it normalized", created.Email)
	}

	if _, err := users.GetByEmail(ctx, "GRACE@example.com"); err != nil {
		t.Errorf("GetByEmail(other case) = %v, want the account", err)
	}
	if _, err := users.Create(ctx, "grace@EXAMPLE.com", "hash"); !errors.Is(err, user.ErrEmailTaken) {
		t.Errorf("Create(other case) = %v, want ErrEmailTaken", err)
	}
}
