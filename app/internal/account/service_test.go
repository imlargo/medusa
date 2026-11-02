package account_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/postgres/pgtest"

	"github.com/imlargo/medusa/app/internal/account"
	"github.com/imlargo/medusa/app/internal/database/migrations"
	"github.com/imlargo/medusa/app/internal/database/sqlc"
	"github.com/imlargo/medusa/app/internal/user"
)

func newService(t *testing.T) (*account.Service, *auth.JWT[int64]) {
	t.Helper()
	tokens := auth.MustNewJWT[int64](strings.Repeat("s", 32))
	users := user.NewRepository(sqlc.New(pgtest.New(t, migrations.FS)))
	return account.NewService(users, tokens, time.Minute, time.Hour), tokens
}

func TestRegisterThenLogin(t *testing.T) {
	t.Parallel()
	svc, tokens := newService(t)
	ctx := t.Context()

	registered, err := svc.Register(ctx, "Ada@Example.com", "correct horse")
	if err != nil {
		t.Fatalf("Register() = %v", err)
	}

	claims, err := tokens.Parse(registered.Tokens.AccessToken)
	if err != nil || claims.Subject != registered.User.ID {
		t.Errorf("access token = %v (subject %v), want one for user %d", err, claims, registered.User.ID)
	}

	loggedIn, err := svc.Login(ctx, "ada@example.com", "correct horse")
	if err != nil {
		t.Fatalf("Login() = %v", err)
	}
	if loggedIn.User.ID != registered.User.ID {
		t.Errorf("Login() user = %d, want %d", loggedIn.User.ID, registered.User.ID)
	}
}

// An unknown email and a wrong password must be indistinguishable.
func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	ctx := t.Context()

	if _, err := svc.Register(ctx, "ada@example.com", "correct horse"); err != nil {
		t.Fatalf("Register() = %v", err)
	}

	for name, creds := range map[string][2]string{
		"wrong password": {"ada@example.com", "battery staple"},
		"unknown email":  {"nobody@example.com", "correct horse"},
	} {
		if _, err := svc.Login(ctx, creds[0], creds[1]); err != account.ErrInvalidCredentials {
			t.Errorf("Login(%s) = %v, want exactly ErrInvalidCredentials", name, err)
		}
	}
}

func TestRegisterTwiceIsEmailTaken(t *testing.T) {
	t.Parallel()
	svc, _ := newService(t)
	ctx := t.Context()

	if _, err := svc.Register(ctx, "ada@example.com", "correct horse"); err != nil {
		t.Fatalf("Register() = %v", err)
	}
	if _, err := svc.Register(ctx, " ADA@example.com", "other"); !errors.Is(err, user.ErrEmailTaken) {
		t.Errorf("second Register() = %v, want user.ErrEmailTaken", err)
	}
}
