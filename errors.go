package medusa

import "github.com/imlargo/medusa/apperr"

// Errors the request helpers report. Their codes are part of every Medusa
// API's contract.
var (
	// ErrMalformedBody: the body is not JSON of the expected shape.
	ErrMalformedBody = apperr.Invalid("request.malformed_body", "the request body could not be parsed")

	// ErrValidation: the input parsed but some fields are invalid; the error
	// lists them by the names the client sent.
	ErrValidation = apperr.Invalid("request.validation_failed", "some fields are invalid")

	// ErrBodyTooLarge: the body is over the limit set by middleware.BodyLimit.
	// The limit is in the max_bytes metadata.
	ErrBodyTooLarge = apperr.New(apperr.KindTooLarge, "request.body_too_large", "the request body is too large")

	// ErrInvalidParameter: a path or query parameter is missing or does not
	// parse. The error names it when the parameter is known.
	ErrInvalidParameter = apperr.Invalid("request.invalid_parameter", "a path or query parameter is invalid")

	// ErrMissingCredentials: the Authorization header is missing.
	ErrMissingCredentials = apperr.Unauthenticated("auth.missing_credentials", "the authorization header is missing")

	// ErrMalformedCredentials: the Authorization header is not
	// "Bearer <token>".
	ErrMalformedCredentials = apperr.Unauthenticated("auth.malformed_credentials", "the authorization header must be 'Bearer <token>'")
)
