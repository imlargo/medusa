package session_test

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/postgres/pgtest"

	"github.com/imlargo/medusa/app/internal/database/migrations"
	"github.com/imlargo/medusa/app/internal/database/sqlc"
	"github.com/imlargo/medusa/app/internal/session"
	"github.com/imlargo/medusa/app/internal/user"
)

func newRepository(t *testing.T) (*session.Repository, string) {
	t.Helper()
	queries := sqlc.New(pgtest.New(t, migrations.FS))

	owner, err := user.NewRepository(queries).Create(t.Context(), "ada@example.com", "hash")
	if err != nil {
		t.Fatalf("create the token's user: %v", err)
	}
	return session.NewRepository(queries), strconv.FormatInt(owner.ID, 10)
}

// The contract auth.Refresher relies on: the second use of a token is told
// apart from the first, and the token is still returned so its family can be
// revoked.
func TestConsumeReportsTheSecondUse(t *testing.T) {
	t.Parallel()
	repo, subject := newRepository(t)
	ctx := t.Context()

	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	token := auth.RefreshToken{Hash: "h1", Family: "f1", Subject: subject, ExpiresAt: expires}
	if err := repo.Create(ctx, token); err != nil {
		t.Fatalf("Create() = %v", err)
	}

	got, reused, err := repo.Consume(ctx, "h1")
	if err != nil || reused {
		t.Fatalf("first Consume() = reused %v, %v, want false, nil", reused, err)
	}
	if got.Family != "f1" || got.Subject != subject || !got.ExpiresAt.Equal(expires) {
		t.Errorf("token = %+v, want what was stored", got)
	}

	if _, reused, err := repo.Consume(ctx, "h1"); err != nil || !reused {
		t.Errorf("second Consume() = reused %v, %v, want true, nil", reused, err)
	}

	if _, _, err := repo.Consume(ctx, "unknown"); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Errorf("Consume() of an unknown hash = %v, want ErrRefreshTokenNotFound", err)
	}
}

func TestRevokeFamilyRemovesOnlyThatFamily(t *testing.T) {
	t.Parallel()
	repo, subject := newRepository(t)
	ctx := t.Context()

	for hash, family := range map[string]string{"a1": "a", "a2": "a", "b1": "b"} {
		if err := repo.Create(ctx, auth.RefreshToken{Hash: hash, Family: family, Subject: subject, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}

	if err := repo.RevokeFamily(ctx, "a"); err != nil {
		t.Fatalf("RevokeFamily() = %v", err)
	}
	for _, hash := range []string{"a1", "a2"} {
		if _, _, err := repo.Consume(ctx, hash); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
			t.Errorf("Consume(%s) after revoking its family = %v, want ErrRefreshTokenNotFound", hash, err)
		}
	}
	if _, _, err := repo.Consume(ctx, "b1"); err != nil {
		t.Errorf("Consume() of another family = %v, want nil", err)
	}
}

func TestDeleteExpiredKeepsLiveTokens(t *testing.T) {
	t.Parallel()
	repo, subject := newRepository(t)
	ctx := t.Context()

	_ = repo.Create(ctx, auth.RefreshToken{Hash: "old", Family: "f", Subject: subject, ExpiresAt: time.Now().Add(-time.Hour)})
	_ = repo.Create(ctx, auth.RefreshToken{Hash: "live", Family: "g", Subject: subject, ExpiresAt: time.Now().Add(time.Hour)})

	if deleted, err := repo.DeleteExpired(ctx); err != nil || deleted != 1 {
		t.Errorf("DeleteExpired() = %d, %v, want 1", deleted, err)
	}
	if _, _, err := repo.Consume(ctx, "live"); err != nil {
		t.Errorf("the live token is gone: %v", err)
	}
}
