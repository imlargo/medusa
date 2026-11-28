// Package account is how people get into the API: registration, password
// login and the signed-in user's own account.
package account

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/imlargo/medusa/auth"

	"github.com/imlargo/medusa/app/internal/session"
	"github.com/imlargo/medusa/app/internal/store"
	"github.com/imlargo/medusa/app/internal/user"
)

// Session is what a successful registration or login returns.
type Session struct {
	User   user.User `json:"user"`
	Tokens Tokens    `json:"tokens"`
}

// Tokens are the signed credentials of a session.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// maxPasswordBytes is the most bcrypt hashes; it rejects anything longer.
const maxPasswordBytes = 72

// dummyHash is compared against when the email is unknown, so a login takes
// as long for an unknown email as for a wrong password and its timing does
// not reveal which emails have accounts.
var dummyHash = sync.OnceValue(func() []byte {
	hash, _ := bcrypt.GenerateFromPassword([]byte("medusa-dummy-password"), bcrypt.DefaultCost)
	return hash
})

// Service registers and signs in accounts.
type Service struct {
	store     *store.Store
	users     *user.Repository
	sessions  *session.Repository
	tokens    *auth.JWT[int64]
	refresher *auth.Refresher
	accessTTL time.Duration
}

// NewService signs access tokens with tokens, valid for accessTTL, and
// issues refresh tokens valid for refreshTTL. A refresh token is an opaque
// value kept in the store, not a signed token: it cannot be used as an
// access token, and it can be revoked.
//
// reuseGrace is how long after its first use a refresh token is still
// accepted, for clients that refresh from several requests at once. Zero
// treats every second use as a stolen token.
func NewService(store *store.Store, users *user.Repository, sessions *session.Repository, tokens *auth.JWT[int64], accessTTL, refreshTTL, reuseGrace time.Duration) *Service {
	return &Service{
		store:     store,
		users:     users,
		sessions:  sessions,
		tokens:    tokens,
		refresher: auth.NewRefresher(sessions, refreshTTL, auth.WithReuseGrace(reuseGrace)),
		accessTTL: accessTTL,
	}
}

// Register creates an account and signs it in. A taken email is
// user.ErrEmailTaken, a password over 72 bytes ErrPasswordTooLong.
func (s *Service) Register(ctx context.Context, email, password string) (*Session, error) {
	if len(password) > maxPasswordBytes {
		return nil, ErrPasswordTooLong
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, ErrPasswordHashFailed.WithCause(err)
	}

	// The unique email constraint decides, so two concurrent registrations of
	// the same email cannot both succeed. The event is part of the same
	// transaction: whoever reacts to a new account hears of every one that
	// exists and of none that does not.
	var created *user.User
	err = s.store.WithTx(ctx, func(ctx context.Context) error {
		var txErr error
		if created, txErr = s.users.Create(ctx, email, string(hash)); txErr != nil {
			return txErr
		}
		return s.store.Publish(ctx, EventRegistered, Registered{UserID: created.ID})
	})
	if err != nil {
		return nil, err
	}

	return s.sessionFor(ctx, created)
}

// Login signs in the account with email and password, or answers
// ErrInvalidCredentials.
func (s *Service) Login(ctx context.Context, email, password string) (*Session, error) {
	found, err := s.users.GetByEmail(ctx, email)
	if errors.Is(err, user.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(found.Password), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.sessionFor(ctx, found)
}

// Refresh trades a refresh token for new tokens. The refresh token is
// replaced and stops working. One that is unknown, expired or already used
// is auth.ErrInvalidRefreshToken, and a used one also ends its session
// everywhere: it was copied.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*Tokens, error) {
	subject, next, err := s.refresher.Rotate(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	userID, err := strconv.ParseInt(subject, 10, 64)
	if err != nil {
		return nil, ErrTokenIssueFailed.WithCause(err)
	}
	access, err := s.tokens.Sign(userID, time.Now().Add(s.accessTTL))
	if err != nil {
		return nil, ErrTokenIssueFailed.WithCause(err)
	}

	return &Tokens{AccessToken: access, RefreshToken: next}, nil
}

// Logout ends the session refreshToken belongs to. Its access token keeps
// working until it expires, which is why access tokens are short-lived.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return s.refresher.Revoke(ctx, refreshToken)
}

// PurgeExpiredSessions deletes refresh tokens whose time has passed.
func (s *Service) PurgeExpiredSessions(ctx context.Context) error {
	_, err := s.sessions.DeleteExpired(ctx)
	return err
}

// Me returns the signed-in account. A deleted account is user.ErrNotFound.
func (s *Service) Me(ctx context.Context, userID int64) (*user.User, error) {
	return s.users.Get(ctx, userID)
}

func (s *Service) sessionFor(ctx context.Context, u *user.User) (*Session, error) {
	access, err := s.tokens.Sign(u.ID, time.Now().Add(s.accessTTL))
	if err != nil {
		return nil, ErrTokenIssueFailed.WithCause(err)
	}

	refresh, err := s.refresher.Issue(ctx, strconv.FormatInt(u.ID, 10))
	if err != nil {
		return nil, ErrTokenIssueFailed.WithCause(err)
	}

	return &Session{
		User:   *u,
		Tokens: Tokens{AccessToken: access, RefreshToken: refresh},
	}, nil
}
