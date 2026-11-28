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

	got, err := repo.Consume(ctx, "h1")
	if err != nil || got.Reused {
		t.Fatalf("first Consume() = reused %v, %v, want false, nil", got.Reused, err)
	}
	if got.Family != "f1" || got.Subject != subject || !got.ExpiresAt.Equal(expires) {
		t.Errorf("token = %+v, want what was stored", got)
	}

	again, err := repo.Consume(ctx, "h1")
	if err != nil || !again.Reused {
		t.Errorf("second Consume() = reused %v, %v, want true, nil", again.Reused, err)
	}
	if again.UsedAgo < 0 || again.UsedAgo > time.Minute {
		t.Errorf("UsedAgo = %s, want the moments since the first use", again.UsedAgo)
	}

	if _, err := repo.Consume(ctx, "unknown"); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
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
		if _, err := repo.Consume(ctx, hash); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
			t.Errorf("Consume(%s) after revoking its family = %v, want ErrRefreshTokenNotFound", hash, err)
		}
	}
	if _, err := repo.Consume(ctx, "b1"); err != nil {
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
	if _, err := repo.Consume(ctx, "live"); err != nil {
		t.Errorf("the live token is gone: %v", err)
	}
}

// The first use of a token retires the siblings it was issued with, so only
// one line of a session continues after a burst.
func TestConsumeRetiresTheTokensSiblings(t *testing.T) {
	t.Parallel()
	repo, subject := newRepository(t)
	ctx := t.Context()

	create := func(hash, parent string) {
		t.Helper()
		if err := repo.Create(ctx, auth.RefreshToken{Hash: hash, Family: "f", Parent: parent, Subject: subject, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	create("root", "")
	create("kept", "root")
	create("sibling", "root")
	create("cousin", "other")
	create("first", "") // no parent: nobody's sibling

	if got, err := repo.Consume(ctx, "kept"); err != nil || got.Reused {
		t.Fatalf("Consume(kept) = reused %v, %v, want a first use", got.Reused, err)
	}

	if got, err := repo.Consume(ctx, "sibling"); err != nil || !got.Reused {
		t.Errorf("Consume(sibling) = reused %v, %v, want it retired with its sibling", got.Reused, err)
	}
	for _, hash := range []string{"cousin", "root", "first"} {
		if got, err := repo.Consume(ctx, hash); err != nil || got.Reused {
			t.Errorf("Consume(%s) = reused %v, %v, want it untouched", hash, got.Reused, err)
		}
	}
}
