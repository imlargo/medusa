package user

import "github.com/imlargo/medusa/apperr"

var (
	// ErrNotFound: no account matches.
	ErrNotFound = apperr.NotFound("user.not_found", "user not found")

	// ErrEmailTaken: another account already uses the email.
	ErrEmailTaken = apperr.Conflict("user.email_taken", "a user with this email already exists")
)
