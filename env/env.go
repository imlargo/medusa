// Package env is Medusa's configuration battery: it reads typed values from
// environment variables, with an optional .env file for development, and
// collects every problem so a misconfigured deployment fails once with the
// complete list instead of one variable at a time.
//
//	if err := env.LoadDotEnv(); err != nil {
//	    return nil, err
//	}
//	e := env.New()
//	cfg := Config{
//	    DatabaseURL: e.Required("DATABASE_URL"),
//	    Port:        e.Int("PORT", 8080),
//	    Timeout:     e.Duration("REQUEST_TIMEOUT", 30*time.Second),
//	}
//	if err := errors.Join(e.Err(), cfg.Validate()); err != nil {
//	    return nil, err
//	}
//
// Values are trimmed, and a variable set to an empty string counts as unset.
// A malformed value is reported and its fallback used, so reading continues.
package env

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// LoadDotEnv loads the given files (".env" when none) into the environment.
// Variables already set win, so a real environment always overrides the
// file. A missing file is the normal case in a container; a malformed one is
// an error.
func LoadDotEnv(paths ...string) error {
	if err := godotenv.Load(paths...); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	return nil
}

// Env reads typed values and remembers every problem it finds. Use one per
// load; it is not safe for concurrent use.
type Env struct {
	errs []error
}

// New reads from the process environment.
func New() *Env {
	return &Env{}
}

// Err returns every problem found so far, joined, or nil.
func (e *Env) Err() error {
	return errors.Join(e.errs...)
}

// String returns key's value, or fallback when it is unset.
func (e *Env) String(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// Required returns key's value and records a problem when it is unset.
func (e *Env) Required(key string) string {
	value := e.String(key, "")
	if value == "" {
		e.errs = append(e.errs, fmt.Errorf("%s is required", key))
	}
	return value
}

// Int parses key as a base-10 integer.
func (e *Env) Int(key string, fallback int) int {
	return parse(e, key, fallback, strconv.Atoi, "a valid integer")
}

// Bool parses key as a boolean: 1, t, true, 0, f, false, in any case.
func (e *Env) Bool(key string, fallback bool) bool {
	return parse(e, key, fallback, strconv.ParseBool, "a valid boolean")
}

// Duration parses key as a Go duration such as 15m, 168h or 1h30m.
func (e *Env) Duration(key string, fallback time.Duration) time.Duration {
	return parse(e, key, fallback, time.ParseDuration, `a valid duration, want a value like "15m" or "168h"`)
}

// List splits key on commas, trimming every entry and dropping empty ones.
// It is nil when key is unset.
func (e *Env) List(key string) []string {
	raw := e.String(key, "")
	if raw == "" {
		return nil
	}

	var values []string
	for entry := range strings.SplitSeq(raw, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			values = append(values, entry)
		}
	}
	return values
}

func parse[T any](e *Env, key string, fallback T, parseFn func(string) (T, error), want string) T {
	raw := e.String(key, "")
	if raw == "" {
		return fallback
	}

	value, err := parseFn(raw)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %q is not %s", key, raw, want))
		return fallback
	}
	return value
}
