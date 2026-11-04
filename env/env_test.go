package env_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/imlargo/medusa/env"
)

func TestTypedValuesAndFallbacks(t *testing.T) {
	t.Setenv("ENV_TEST_INT", " 42 ")
	t.Setenv("ENV_TEST_BOOL", "true")
	t.Setenv("ENV_TEST_DURATION", "1h30m")
	t.Setenv("ENV_TEST_LIST", "a, b,,c ")
	t.Setenv("ENV_TEST_EMPTY", "  ")

	e := env.New()

	if got := e.Int("ENV_TEST_INT", 0); got != 42 {
		t.Errorf("Int() = %d, want 42", got)
	}
	if got := e.Bool("ENV_TEST_BOOL", false); !got {
		t.Error("Bool() = false, want true")
	}
	if got := e.Duration("ENV_TEST_DURATION", 0); got != 90*time.Minute {
		t.Errorf("Duration() = %s, want 1h30m", got)
	}
	if got := e.List("ENV_TEST_LIST"); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("List() = %q, want [a b c]", got)
	}
	if got := e.String("ENV_TEST_EMPTY", "fallback"); got != "fallback" {
		t.Errorf("String() = %q, want the fallback for a blank value", got)
	}
	if got := e.List("ENV_TEST_UNSET"); got != nil {
		t.Errorf("List() = %q, want nil when unset", got)
	}
	if err := e.Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}
}

// One load reports every problem, so a deployment is fixed in one pass.
func TestErrReportsEveryProblem(t *testing.T) {
	t.Setenv("ENV_TEST_INT", "forty")
	t.Setenv("ENV_TEST_DURATION", "soon")

	e := env.New()
	if got := e.Int("ENV_TEST_INT", 7); got != 7 {
		t.Errorf("Int() = %d, want the fallback for a malformed value", got)
	}
	e.Duration("ENV_TEST_DURATION", 0)
	e.Required("ENV_TEST_MISSING")

	err := e.Err()
	for _, want := range []string{"ENV_TEST_INT", "ENV_TEST_DURATION", "ENV_TEST_MISSING is required"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Err() = %v, want it to mention %s", err, want)
		}
	}
}

func TestLoadDotEnvNeverOverridesTheEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("ENV_TEST_FILE=from-file\nENV_TEST_SET=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV_TEST_SET", "from-env")
	t.Cleanup(func() { _ = os.Unsetenv("ENV_TEST_FILE") })

	if err := env.LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv() = %v", err)
	}

	e := env.New()
	if got := e.String("ENV_TEST_FILE", ""); got != "from-file" {
		t.Errorf("ENV_TEST_FILE = %q, want it loaded from the file", got)
	}
	if got := e.String("ENV_TEST_SET", ""); got != "from-env" {
		t.Errorf("ENV_TEST_SET = %q, want the environment to win", got)
	}
}

func TestLoadDotEnvWithoutAFileIsFine(t *testing.T) {
	if err := env.LoadDotEnv(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Errorf("LoadDotEnv() = %v, want nil for a missing file", err)
	}
}
