package password_test

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	"github.com/imlargo/medusa/auth/password"
)

func TestHashThenVerify(t *testing.T) {
	passwords := password.New()

	hash, err := passwords.Hash(t.Context(), "correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() = %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash = %q, want an argon2id PHC string with OWASP's parameters", hash)
	}

	if ok, rehash, err := passwords.Verify("correct horse battery staple", hash); err != nil || !ok || rehash {
		t.Errorf("Verify() of the right password = %v, rehash %v, %v, want true, false", ok, rehash, err)
	}
	if ok, _, err := passwords.Verify("correct horse battery stapler", hash); err != nil || ok {
		t.Errorf("Verify() of a wrong password = %v, %v, want false", ok, err)
	}

	again, _ := passwords.Hash(t.Context(), "correct horse battery staple")
	if again == hash {
		t.Error("two hashes of one password are equal, want a salt per hash")
	}
}

// The policy is length, and nothing about which characters.
func TestThePolicy(t *testing.T) {
	passwords := password.New()

	tests := []struct {
		name     string
		password string
		want     error
	}{
		{"eleven characters", "elevenchars", password.ErrTooShort},
		{"twelve characters, all lowercase", "twelve chars", nil},
		{"a passphrase with spaces", "four words are enough", nil},
		{"twelve characters that are not ASCII", "contraseñaañ", nil},
		{"128 characters", strings.Repeat("a", 128), nil},
		{"129 characters", strings.Repeat("a", 129), password.ErrTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := passwords.Check(t.Context(), tt.password)
			if tt.want == nil && err != nil {
				t.Errorf("Check() = %v, want it accepted", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("Check() = %v, want %v", err, tt.want)
			}
		})
	}

	if _, err := passwords.Hash(t.Context(), "short"); !errors.Is(err, password.ErrTooShort) {
		t.Errorf("Hash() of a short password = %v, want ErrTooShort: nothing weaker than the policy is ever stored", err)
	}
}

func TestABreachedPasswordIsRefusedAndAFailingCheckIsNot(t *testing.T) {
	breached := password.New(password.WithBreachCheck(func(_ context.Context, plain string) (bool, error) {
		return plain == "password1234", nil
	}))
	if err := breached.Check(t.Context(), "password1234"); !errors.Is(err, password.ErrBreached) {
		t.Errorf("Check() of a breached password = %v, want ErrBreached", err)
	}
	if err := breached.Check(t.Context(), "a fresh passphrase"); err != nil {
		t.Errorf("Check() of a password that is not breached = %v, want nil", err)
	}

	// The breach list being down must not close registration.
	down := password.New(password.WithBreachCheck(func(context.Context, string) (bool, error) {
		return false, errors.New("the breach list is unreachable")
	}))
	if err := down.Check(t.Context(), "password1234"); err != nil {
		t.Errorf("Check() while the breach list is down = %v, want the password accepted", err)
	}
}

// An application that used bcrypt keeps signing its users in, and is told to
// replace each hash as it is used.
func TestVerifyReadsBcryptAndAsksForARehash(t *testing.T) {
	legacy, err := bcrypt.GenerateFromPassword([]byte("an old password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	passwords := password.New()

	if ok, rehash, err := passwords.Verify("an old password", string(legacy)); err != nil || !ok || !rehash {
		t.Errorf("Verify() of a bcrypt hash = %v, rehash %v, %v, want true, true", ok, rehash, err)
	}
	if ok, rehash, err := passwords.Verify("a wrong password", string(legacy)); err != nil || ok || rehash {
		t.Errorf("Verify() of a wrong password against bcrypt = %v, rehash %v, %v, want false, false", ok, rehash, err)
	}
}

// Parameters get stronger over the years. A hash made with weaker ones still
// verifies, and is replaced the next time its owner signs in.
func TestVerifyAsksForARehashOfWeakerParameters(t *testing.T) {
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte("an old password"), salt, 1, 8*1024, 1, 32)
	weak := "$argon2id$v=19$m=8192,t=1,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)

	passwords := password.New()
	if ok, rehash, err := passwords.Verify("an old password", weak); err != nil || !ok || !rehash {
		t.Errorf("Verify() of a hash with weaker parameters = %v, rehash %v, %v, want true, true", ok, rehash, err)
	}

	strong, _ := passwords.Hash(t.Context(), "an old password")
	if _, rehash, _ := passwords.Verify("an old password", strong); rehash {
		t.Error("a hash with today's parameters asks for a rehash")
	}
}

func TestVerifyWithNoHashIsFalseAndAnUnknownFormatIsAnError(t *testing.T) {
	passwords := password.New()

	if ok, rehash, err := passwords.Verify("whatever it is", ""); err != nil || ok || rehash {
		t.Errorf("Verify() with no hash = %v, %v, %v, want false with no error", ok, rehash, err)
	}
	for _, hash := range []string{"plaintext", "$argon2id$v=19$m=19456,t=2,p=1$onlysalt", "$argon2id$v=19$m=99999999,t=2,p=1$c2FsdA$a2V5"} {
		if _, _, err := passwords.Verify("whatever it is", hash); err == nil {
			t.Errorf("Verify() against %q = nil error, want one: a hash that cannot be read is not a wrong password", hash)
		}
	}
}
