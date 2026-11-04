// Package user is the user aggregate: the account record, the rules every
// account follows, the errors those rules produce, and its storage.
package user

import (
	"strings"
	"time"
)

// User is an account. Password holds the bcrypt hash and is never serialized.
type User struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Email    string `json:"email"`
	Password string `json:"-"`
}

// NormalizeEmail is the form every email is stored and looked up in: emails
// are case-insensitive and surrounding spaces are never part of one. The
// repository applies it, so no caller can store or search a variant.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
