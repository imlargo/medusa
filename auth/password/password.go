// Package password hashes passwords and decides which ones are acceptable.
//
//	passwords := password.New()
//
//	hash, err := passwords.Hash(ctx, plain)     // refuses a password the policy rejects
//	ok, rehash, err := passwords.Verify(plain, hash)
//	if ok && rehash {
//	    // store passwords.Hash(ctx, plain) in place of the old hash
//	}
//
// # Hashing
//
// Hashes are argon2id with the parameters OWASP recommends (19 MiB of memory,
// 2 passes, 1 thread), written in the PHC string format, which carries its
// own parameters and salt. Verify also accepts bcrypt hashes, so an
// application that used bcrypt keeps signing its users in, and reports that
// the hash should be replaced: each user moves to argon2id at their next
// sign-in.
//
// # Policy
//
// The policy is NIST's (SP 800-63B): length is what matters, so a password
// is at least [MinLength] characters and at most [MaxLength], and there are
// no rules about which characters it contains. A password known from a data
// breach is refused when the application connects a check for it with
// [WithBreachCheck].
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	"github.com/imlargo/medusa/apperr"
)

// The policy's limits, in characters.
const (
	// MinLength is the shortest password accepted.
	MinLength = 12

	// MaxLength is the longest. It is high enough for any passphrase and
	// bounds the work of hashing what a client sends.
	MaxLength = 128
)

// Errors of the policy. Their codes are part of an API's contract.
var (
	// ErrTooShort: the password is under MinLength characters.
	ErrTooShort = apperr.Invalid("password.too_short", "the password must be at least 12 characters long")

	// ErrTooLong: the password is over MaxLength characters.
	ErrTooLong = apperr.Invalid("password.too_long", "the password must be at most 128 characters long")

	// ErrBreached: the password is known from a data breach.
	ErrBreached = apperr.Invalid("password.breached", "this password has appeared in a data breach: choose another one")
)

// argon2id parameters: OWASP's recommended minimum configuration.
const (
	argonMemory  = 19 * 1024 // KiB
	argonTime    = 2
	argonThreads = 1
	saltLength   = 16
	keyLength    = 32
)

// BreachCheck reports whether a password is known from a data breach, for
// example against the Pwned Passwords range API or a local list.
type BreachCheck func(ctx context.Context, password string) (bool, error)

// Hasher hashes and verifies passwords under the policy. It is safe for
// concurrent use.
type Hasher struct {
	breached BreachCheck
	dummy    string
}

// Option configures a Hasher.
type Option func(*Hasher)

// WithBreachCheck refuses passwords the check reports as breached. If the
// check itself fails, the password is accepted: an outage of a breach list
// must not stop people from registering.
func WithBreachCheck(check BreachCheck) Option {
	return func(h *Hasher) { h.breached = check }
}

// New builds a hasher.
func New(opts ...Option) *Hasher {
	h := &Hasher{}
	for _, opt := range opts {
		opt(h)
	}
	// What Verify compares against when there is no hash, so that signing
	// in as an unknown user costs what a wrong password costs.
	h.dummy = encode([]byte("medusa-dummy-salt"), derive("medusa-dummy-password", []byte("medusa-dummy-salt")))
	return h
}

// Check applies the policy to password and returns the rule it breaks:
// [ErrTooShort], [ErrTooLong] or [ErrBreached]. Use it to validate a
// password before it is time to hash it.
func (h *Hasher) Check(ctx context.Context, password string) error {
	length := utf8.RuneCountInString(password)
	switch {
	case length < MinLength:
		return ErrTooShort.WithMeta("min_length", MinLength)
	case length > MaxLength:
		return ErrTooLong.WithMeta("max_length", MaxLength)
	}

	if h.breached != nil {
		if breached, err := h.breached(ctx, password); err == nil && breached {
			return ErrBreached
		}
	}
	return nil
}

// Hash checks password against the policy and returns its argon2id hash, to
// store. A password the policy rejects returns the policy's error.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	if err := h.Check(ctx, password); err != nil {
		return "", err
	}

	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: generate salt: %w", err)
	}
	return encode(salt, derive(password, salt)), nil
}

// Verify reports whether password matches the stored hash. rehash is true
// when it matches and the hash should be replaced with a new one from Hash:
// it is bcrypt, or argon2id with weaker parameters than today's.
//
// With an empty hash, for a user that does not exist, it does the work of a
// comparison and reports false, so the time a sign-in takes does not reveal
// which accounts exist. A hash in a format it cannot read is an error.
func (h *Hasher) Verify(password, hash string) (ok, rehash bool, err error) {
	switch {
	case hash == "":
		_, _, _ = h.Verify(password, h.dummy)
		return false, false, nil

	case strings.HasPrefix(hash, "$argon2id$"):
		params, salt, want, err := decode(hash)
		if err != nil {
			return false, false, err
		}
		got := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(want))) //nolint:gosec // G115: the length of a decoded key
		if subtle.ConstantTimeCompare(got, want) != 1 {
			return false, false, nil
		}
		weaker := params.memory < argonMemory || params.time < argonTime || len(want) < keyLength
		return true, weaker, nil

	case strings.HasPrefix(hash, "$2a$"), strings.HasPrefix(hash, "$2b$"), strings.HasPrefix(hash, "$2y$"):
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
			if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || errors.Is(err, bcrypt.ErrPasswordTooLong) {
				return false, false, nil
			}
			return false, false, fmt.Errorf("password: read bcrypt hash: %w", err)
		}
		return true, true, nil
	}

	return false, false, errors.New("password: the stored hash is in a format this package does not read")
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func derive(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, keyLength)
}

// encode writes a hash as a PHC string:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<key>, both in unpadded base64.
func encode(salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func decode(hash string) (argonParams, []byte, []byte, error) {
	malformed := errors.New("password: the stored argon2id hash is malformed")

	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, key
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		return argonParams{}, nil, nil, malformed
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argonParams{}, nil, nil, malformed
	}

	var params argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.time, &params.threads); err != nil {
		return argonParams{}, nil, nil, malformed
	}
	// A stored hash is trusted, but not to the point of letting a corrupted
	// row ask for gigabytes.
	if params.memory == 0 || params.memory > 1<<20 || params.time == 0 || params.time > 16 || params.threads == 0 {
		return argonParams{}, nil, nil, malformed
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return argonParams{}, nil, nil, malformed
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return argonParams{}, nil, nil, malformed
	}
	return params, salt, key, nil
}
