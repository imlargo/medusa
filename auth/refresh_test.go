package auth_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/imlargo/medusa/auth"
)

// memoryStore is a RefreshStore in a map, with Consume atomic as the
// contract requires.
type memoryStore struct {
	mu     sync.Mutex
	tokens map[string]auth.RefreshToken
	used   map[string]bool
}

func newMemoryStore() *memoryStore {
	return &memoryStore{tokens: map[string]auth.RefreshToken{}, used: map[string]bool{}}
}

func (s *memoryStore) Create(_ context.Context, token auth.RefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token.Hash] = token
	return nil
}

func (s *memoryStore) Consume(_ context.Context, hash string) (auth.RefreshToken, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, ok := s.tokens[hash]
	if !ok {
		return auth.RefreshToken{}, false, auth.ErrRefreshTokenNotFound
	}
	reused := s.used[hash]
	s.used[hash] = true
	return token, reused, nil
}

func (s *memoryStore) RevokeFamily(_ context.Context, family string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, token := range s.tokens {
		if token.Family == family {
			delete(s.tokens, hash)
		}
	}
	return nil
}

func TestRotateTradesATokenForTheNextOne(t *testing.T) {
	store := newMemoryStore()
	refresher := auth.NewRefresher(store, time.Hour)
	ctx := t.Context()

	first, err := refresher.Issue(ctx, "7")
	if err != nil {
		t.Fatalf("Issue() = %v", err)
	}
	if _, stored := store.tokens[first]; stored {
		t.Error("the store holds the token's value, want only its hash")
	}

	subject, second, err := refresher.Rotate(ctx, first)
	if err != nil || subject != "7" || second == "" || second == first {
		t.Fatalf("Rotate() = %q, %q, %v, want subject 7 and a new token", subject, second, err)
	}

	if subject, _, err := refresher.Rotate(ctx, second); err != nil || subject != "7" {
		t.Errorf("Rotate() of the new token = %q, %v, want it to work", subject, err)
	}
}

// A token presented twice was copied. Nothing tells the thief from the
// owner, so the session ends for both.
func TestAReusedTokenRevokesItsWholeSession(t *testing.T) {
	refresher := auth.NewRefresher(newMemoryStore(), time.Hour)
	ctx := t.Context()

	stolen, _ := refresher.Issue(ctx, "7")
	_, current, err := refresher.Rotate(ctx, stolen)
	if err != nil {
		t.Fatalf("Rotate() = %v", err)
	}

	if _, _, err := refresher.Rotate(ctx, stolen); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatalf("Rotate() of a used token = %v, want ErrInvalidRefreshToken", err)
	}
	if _, _, err := refresher.Rotate(ctx, current); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Errorf("Rotate() of the session's current token after a reuse = %v, want the session revoked", err)
	}
}

func TestSessionsAreIndependent(t *testing.T) {
	refresher := auth.NewRefresher(newMemoryStore(), time.Hour)
	ctx := t.Context()

	laptop, _ := refresher.Issue(ctx, "7")
	phone, _ := refresher.Issue(ctx, "7")

	if err := refresher.Revoke(ctx, laptop); err != nil {
		t.Fatalf("Revoke() = %v", err)
	}
	if _, _, err := refresher.Rotate(ctx, laptop); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Errorf("Rotate() after signing out = %v, want ErrInvalidRefreshToken", err)
	}
	if _, _, err := refresher.Rotate(ctx, phone); err != nil {
		t.Errorf("Rotate() on another device = %v, want it untouched", err)
	}
}

func TestRotateRejectsWhatItCannotTrade(t *testing.T) {
	ctx := t.Context()

	if _, _, err := auth.NewRefresher(newMemoryStore(), time.Hour).Rotate(ctx, "never-issued"); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Errorf("Rotate() of an unknown token = %v, want ErrInvalidRefreshToken", err)
	}

	expiring := auth.NewRefresher(newMemoryStore(), -time.Minute)
	token, _ := expiring.Issue(ctx, "7")
	if _, _, err := expiring.Rotate(ctx, token); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Errorf("Rotate() of an expired token = %v, want ErrInvalidRefreshToken", err)
	}
}

// Signing out twice, or with a token that never existed, still leaves the
// client signed out.
func TestRevokeOfAnUnknownTokenIsNotAnError(t *testing.T) {
	if err := auth.NewRefresher(newMemoryStore(), time.Hour).Revoke(t.Context(), "never-issued"); err != nil {
		t.Errorf("Revoke() = %v, want nil", err)
	}
}
