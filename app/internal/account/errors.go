package account

import "github.com/imlargo/medusa/apperr"

var (
	// ErrInvalidCredentials: the email and password do not match an account.
	// It is the same for an unknown email and a wrong password, so the login
	// endpoint cannot be used to find out which emails have accounts.
	ErrInvalidCredentials = apperr.Unauthenticated("account.invalid_credentials", "invalid email or password")

	// ErrPasswordHashFailed: the password could not be hashed.
	ErrPasswordHashFailed = apperr.Internal("account.password_hash_failed", "could not process the password")

	// ErrTokenIssueFailed: the session tokens could not be signed.
	ErrTokenIssueFailed = apperr.Internal("account.token_issue_failed", "could not issue the session tokens")
)
