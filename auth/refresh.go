package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/imlargo/medusa/apperr"
)

// ErrInvalidRefreshToken: the refresh token is unknown, expired, revoked or
// was already used. The client has to sign in again.
var ErrInvalidRefreshToken = apperr.Unauthenticated("auth.invalid_refresh_token", "invalid or expired refresh token")

// ErrRefreshTokenNotFound is what a [RefreshStore] returns for a hash it
// does not hold.
var ErrRefreshTokenNotFound = errors.New("auth: refresh token not found")

// RefreshToken is a refresh token as it is stored: by the hash of its value,
// never the value.
type RefreshToken struct {
	// Hash identifies the token: see [HashToken].
	Hash string

	// Family is shared by every token descended from one sign-in. Revoking
	// it ends that session on every device that copied it.
	Family string

	// Parent is the hash of the token this one replaced, empty for the first
	// token of a session. Tokens with the same parent are siblings: answers
	// to the same token presented more than once within the reuse grace.
	Parent string

	// Subject is who the token signs in.
	Subject string

	ExpiresAt time.Time
}

// RefreshStore keeps refresh tokens. The application implements it on its
// database: this battery imports none.
type RefreshStore interface {
	// Create stores token.
	Create(ctx context.Context, token RefreshToken) error

	// Consume marks the token with hash as used and returns it, saying
	// whether it had been used before and how long ago. On a first use it
	// also marks as used every unused sibling of the token (same non-empty
	// Parent), so of the tokens handed out for one parent only one line
	// continues.
	//
	// The check and the marks must be one atomic step, or two requests with
	// the same token could both pass as the first. An unknown hash is
	// [ErrRefreshTokenNotFound].
	Consume(ctx context.Context, hash string) (ConsumedToken, error)

	// RevokeFamily removes every token of family.
	RevokeFamily(ctx context.Context, family string) error
}

// ConsumedToken is a refresh token after [RefreshStore.Consume].
type ConsumedToken struct {
	RefreshToken

	// Reused is true when the token had been used before this call.
	Reused bool

	// UsedAgo is how long ago it was first used, when Reused. It is measured
	// by the store's clock, so servers with skewed clocks agree.
	UsedAgo time.Duration
}

// Refresher issues and rotates refresh tokens: long-lived, opaque values a
// client trades for a new access token.
//
// Every use replaces the token with a new one of the same family. A token
// that is presented twice was copied, by an attacker or by a client bug, and
// there is no telling which holder is the real one: the whole family is
// revoked and both have to sign in again. This is the rotation with reuse
// detection of RFC 9700. See [WithReuseGrace] for clients that legitimately
// send one token several times at once.
type Refresher struct {
	store RefreshStore
	ttl   time.Duration
	grace time.Duration
}

// RefresherOption configures a Refresher.
type RefresherOption func(*Refresher)

// WithReuseGrace accepts a token that is presented again within window of
// its first use, instead of revoking its session. It is for clients that
// refresh from several requests at once: a page that fires five requests
// with an expired access token sends the same refresh token five times
// within milliseconds, and without a grace that signs the user out.
//
// Each of those requests gets a token of its own. They are siblings: when
// one of them is rotated, the others are retired, so only one line of the
// session continues, and a retired sibling presented after the window
// revokes the session like any reused token.
//
// The window is a trade against detection. A thief who replays a token
// inside it is not told apart from the client's own burst. Keep it as short
// as the burst it covers; ten seconds is usual. Zero, the default, turns it
// off.
func WithReuseGrace(window time.Duration) RefresherOption {
	return func(r *Refresher) { r.grace = window }
}

// NewRefresher issues tokens that live for ttl, kept in store.
func NewRefresher(store RefreshStore, ttl time.Duration, opts ...RefresherOption) *Refresher {
	r := &Refresher{store: store, ttl: ttl}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Issue starts a session for subject and returns its first refresh token.
func (r *Refresher) Issue(ctx context.Context, subject string) (string, error) {
	return r.create(ctx, subject, rand.Text(), "")
}

// Rotate trades token for the next one of its session. It returns the
// subject, to sign a new access token for, and the new refresh token; the
// old one stops working. Any token that cannot be traded is
// [ErrInvalidRefreshToken], with the reason as its cause.
func (r *Refresher) Rotate(ctx context.Context, token string) (subject, next string, err error) {
	stored, err := r.store.Consume(ctx, HashToken(token))
	if errors.Is(err, ErrRefreshTokenNotFound) {
		return "", "", ErrInvalidRefreshToken.WithCause(err)
	}
	if err != nil {
		return "", "", fmt.Errorf("consume refresh token: %w", err)
	}

	// Inside the grace a second use is the client's own burst: it is served
	// like the first, with a sibling of the token the first one got.
	if stored.Reused && stored.UsedAgo > r.grace {
		if err := r.store.RevokeFamily(ctx, stored.Family); err != nil {
			return "", "", fmt.Errorf("revoke a reused refresh token's session: %w", err)
		}
		return "", "", ErrInvalidRefreshToken.WithCause(errors.New("refresh token reused: its session was revoked"))
	}
	if time.Now().After(stored.ExpiresAt) {
		return "", "", ErrInvalidRefreshToken.WithCause(errors.New("refresh token expired"))
	}

	next, err = r.create(ctx, stored.Subject, stored.Family, stored.Hash)
	if err != nil {
		return "", "", err
	}
	return stored.Subject, next, nil
}

// Revoke ends the session token belongs to: signing out. A token that is
// unknown or already used is not an error; the session is over either way.
func (r *Refresher) Revoke(ctx context.Context, token string) error {
	stored, err := r.store.Consume(ctx, HashToken(token))
	if errors.Is(err, ErrRefreshTokenNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("consume refresh token: %w", err)
	}

	if err := r.store.RevokeFamily(ctx, stored.Family); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (r *Refresher) create(ctx context.Context, subject, family, parent string) (string, error) {
	// 26 characters of base32: 130 bits from crypto/rand.
	token := rand.Text()

	err := r.store.Create(ctx, RefreshToken{
		Hash:      HashToken(token),
		Family:    family,
		Parent:    parent,
		Subject:   subject,
		ExpiresAt: time.Now().Add(r.ttl),
	})
	if err != nil {
		return "", fmt.Errorf("store refresh token: %w", err)
	}
	return token, nil
}

// HashToken is the form a refresh token is stored and looked up in: its
// SHA-256 in hex. The value is long and random, so a fast hash is enough,
// and a leaked table gives no usable token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
