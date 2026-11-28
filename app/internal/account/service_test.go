package account_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/auth/password"
	"github.com/imlargo/medusa/postgres/pgtest"
	"github.com/imlargo/medusa/postgres/queue"

	"github.com/imlargo/medusa/app/internal/account"
	"github.com/imlargo/medusa/app/internal/database/migrations"
	"github.com/imlargo/medusa/app/internal/session"
	"github.com/imlargo/medusa/app/internal/store"
	"github.com/imlargo/medusa/app/internal/user"
)

func newService(t *testing.T) (*account.Service, *auth.JWT[int64]) {
	t.Helper()
	tokens := auth.MustNewJWT[int64](strings.Repeat("s", 32))
	pool := pgtest.New(t, migrations.FS)
	if err := queue.Install(t.Context(), pool); err != nil {
		t.Fatalf("install the job queue: %v", err)
	}
	return account.NewService(store.New(pool), user.NewRepository(pool), session.NewRepository(pool), tokens, time.Minute, time.Hour, 0), tokens
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
		if _, err := svc.Login(ctx, creds[0], creds[1]); !errors.Is(err, account.ErrInvalidCredentials) {
			t.Errorf("Login(%s) = %v, want ErrInvalidCredentials", name, err)
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
	if _, err := svc.Register(ctx, " ADA@example.com", "another passphrase"); !errors.Is(err, user.ErrEmailTaken) {
		t.Errorf("second Register() = %v, want user.ErrEmailTaken", err)
	}
}

// The policy is checked before anything is stored: a password it rejects is
// the client's input to fix, with the rule it broke.
func TestRegisterAppliesThePasswordPolicy(t *testing.T) {
	t.Parallel()
	svc := account.NewService(store.New(nil), user.NewRepository(nil), session.NewRepository(nil), auth.MustNewJWT[int64](strings.Repeat("s", 32)), time.Minute, time.Hour, 0)

	if _, err := svc.Register(t.Context(), "ada@example.com", "too short"); !errors.Is(err, password.ErrTooShort) {
		t.Errorf("Register() with 9 characters = %v, want password.ErrTooShort", err)
	}
	if _, err := svc.Register(t.Context(), "ada@example.com", strings.Repeat("a", 129)); !errors.Is(err, password.ErrTooLong) {
		t.Errorf("Register() with 129 characters = %v, want password.ErrTooLong", err)
	}
}

// An account from before argon2id has a bcrypt hash. It still signs in, and
// signing in is when its hash is replaced.
func TestLoginUpgradesAnOldHash(t *testing.T) {
	t.Parallel()
	pool := pgtest.New(t, migrations.FS)
	if err := queue.Install(t.Context(), pool); err != nil {
		t.Fatalf("install the job queue: %v", err)
	}
	users := user.NewRepository(pool)
	svc := account.NewService(store.New(pool), users, session.NewRepository(pool), auth.MustNewJWT[int64](strings.Repeat("s", 32)), time.Minute, time.Hour, 0)
	ctx := t.Context()

	legacy, err := bcrypt.GenerateFromPassword([]byte("an old password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	created, err := users.Create(ctx, "old@example.com", string(legacy))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Login(ctx, "old@example.com", "an old password"); err != nil {
		t.Fatalf("Login() with a bcrypt hash = %v, want it to sign in", err)
	}

	stored, err := users.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored.Password, "$argon2id$") {
		t.Errorf("stored hash = %q, want it replaced with argon2id", stored.Password[:10])
	}
	if _, err := svc.Login(ctx, "old@example.com", "an old password"); err != nil {
		t.Errorf("Login() after the upgrade = %v, want it to sign in", err)
	}
}
