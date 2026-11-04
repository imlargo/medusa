// Package account is how people get into the API: registration, password
// login and the signed-in user's own account.
package account

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/imlargo/medusa/auth"

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
	users      *user.Repository
	tokens     *auth.JWT[int64]
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewService signs sessions with tokens; access and refresh tokens expire
// after accessTTL and refreshTTL.
func NewService(users *user.Repository, tokens *auth.JWT[int64], accessTTL, refreshTTL time.Duration) *Service {
	return &Service{users: users, tokens: tokens, accessTTL: accessTTL, refreshTTL: refreshTTL}
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
	// the same email cannot both succeed.
	created, err := s.users.Create(ctx, email, string(hash))
	if err != nil {
		return nil, err
	}

	return s.sessionFor(created)
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

	return s.sessionFor(found)
}

// Me returns the signed-in account. A deleted account is user.ErrNotFound.
func (s *Service) Me(ctx context.Context, userID int64) (*user.User, error) {
	return s.users.Get(ctx, userID)
}

func (s *Service) sessionFor(u *user.User) (*Session, error) {
	now := time.Now()

	access, err := s.tokens.Sign(u.ID, now.Add(s.accessTTL))
	if err != nil {
		return nil, ErrTokenIssueFailed.WithCause(err)
	}

	refresh, err := s.tokens.Sign(u.ID, now.Add(s.refreshTTL))
	if err != nil {
		return nil, ErrTokenIssueFailed.WithCause(err)
	}

	return &Session{
		User:   *u,
		Tokens: Tokens{AccessToken: access, RefreshToken: refresh},
	}, nil
}
