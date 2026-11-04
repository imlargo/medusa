package user_test

import (
	"testing"

	"github.com/imlargo/medusa/app/internal/user"
)

func TestNormalizeEmail(t *testing.T) {
	tests := map[string]string{
		"ada@example.com":      "ada@example.com",
		"Ada@Example.COM":      "ada@example.com",
		"  ada@example.com \n": "ada@example.com",
	}
	for in, want := range tests {
		if got := user.NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
