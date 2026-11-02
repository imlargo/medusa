// Package httperr is the error model behind every failed HTTP response: a
// status, a stable machine-readable code, a client-safe message, optional
// details and an internal cause that never reaches the client.
//
// It depends only on the standard library, so code that classifies failures
// does not have to import the HTTP framework that renders them. The medusa
// package renders an *Error for Gin:
//
//	return nil, httperr.NotFound("user")
package httperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Code is a machine-readable error code clients can branch on. Codes are part
// of the API contract and must stay stable across versions.
type Code string

const (
	// CodeBadRequest: the request was malformed or carried invalid parameters (400).
	CodeBadRequest Code = "BAD_REQUEST"

	// CodeValidation: the body parsed but failed validation (400). Details
	// carries a field-to-message map.
	CodeValidation Code = "VALIDATION_FAILED"

	// CodeUnauthorized: authentication is required, missing or invalid (401).
	CodeUnauthorized Code = "UNAUTHORIZED"

	// CodeForbidden: authenticated but not allowed to touch this resource (403).
	CodeForbidden Code = "FORBIDDEN"

	// CodeNotFound: the requested resource does not exist (404).
	CodeNotFound Code = "NOT_FOUND"

	// CodeConflict: the request conflicts with the resource's current state (409).
	CodeConflict Code = "CONFLICT"

	// CodePayloadTooLarge: the request body exceeded the configured cap (413).
	CodePayloadTooLarge Code = "PAYLOAD_TOO_LARGE"

	// CodeTooManyRequests: the client exceeded its rate limit (429).
	CodeTooManyRequests Code = "TOO_MANY_REQUESTS"

	// CodeInternal: an unexpected server-side failure (500). The underlying
	// cause is logged, never sent to the client.
	CodeInternal Code = "INTERNAL_SERVER_ERROR"

	// CodeServiceUnavailable: a dependency is down or the app is draining (503).
	CodeServiceUnavailable Code = "SERVICE_UNAVAILABLE"
)

// Error carries everything needed to render a failed response. Use the
// constructors below rather than building one by hand.
//
// Internal holds the underlying cause. It is never serialized: it goes to the
// access log, so a 500 can be diagnosed without leaking internals.
type Error struct {
	Code     Code
	Message  string
	Status   int
	Details  any
	Internal error
}

// Error includes the internal cause, so this string is safe for logs but not
// for clients; clients get Message.
func (e *Error) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Internal)
	}
	return e.Message
}

// Unwrap exposes the internal cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.Internal }

// WithDetails attaches details and returns the same error for chaining.
func (e *Error) WithDetails(details any) *Error {
	e.Details = details
	return e
}

// BadRequest reports a malformed request (400).
func BadRequest(message string) *Error {
	return &Error{Code: CodeBadRequest, Message: message, Status: http.StatusBadRequest}
}

// Validation reports a body that parsed but failed validation (400). details
// is typically a field-to-message map.
func Validation(message string, details any) *Error {
	return &Error{Code: CodeValidation, Message: message, Status: http.StatusBadRequest, Details: details}
}

// Unauthorized reports missing or invalid authentication (401).
func Unauthorized(message string) *Error {
	return &Error{Code: CodeUnauthorized, Message: message, Status: http.StatusUnauthorized}
}

// Forbidden reports an authenticated caller without permission (403).
func Forbidden(message string) *Error {
	return &Error{Code: CodeForbidden, Message: message, Status: http.StatusForbidden}
}

// NotFound reports a missing resource (404). resource names what was looked
// up, for example "user".
func NotFound(resource string) *Error {
	return &Error{Code: CodeNotFound, Message: resource + " not found", Status: http.StatusNotFound}
}

// Conflict reports a clash with the resource's current state (409).
func Conflict(message string) *Error {
	return &Error{Code: CodeConflict, Message: message, Status: http.StatusConflict}
}

// PayloadTooLarge reports a request body over the limit (413).
func PayloadTooLarge(maxBytes int64) *Error {
	return &Error{
		Code:    CodePayloadTooLarge,
		Message: "request body is too large",
		Status:  http.StatusRequestEntityTooLarge,
		Details: map[string]int64{"max_bytes": maxBytes},
	}
}

// TooManyRequests reports an exhausted rate limit (429), telling the client
// how many seconds to wait.
func TooManyRequests(retryAfterSeconds int) *Error {
	return &Error{
		Code:    CodeTooManyRequests,
		Message: "rate limit exceeded",
		Status:  http.StatusTooManyRequests,
		Details: map[string]int{"retry_after_seconds": retryAfterSeconds},
	}
}

// Internal reports an unexpected failure (500). The cause is logged; the
// client only ever sees a generic message.
func Internal(cause error) *Error {
	return InternalWithMessage("an internal error occurred", cause)
}

// InternalWithMessage is Internal with a client-safe message of your own.
// Never pass raw error text: it is the most common way internals leak.
func InternalWithMessage(message string, cause error) *Error {
	return &Error{
		Code:     CodeInternal,
		Message:  message,
		Status:   http.StatusInternalServerError,
		Internal: cause,
	}
}

// ServiceUnavailable reports a dependency being down or the app draining (503).
func ServiceUnavailable(message string) *Error {
	return &Error{Code: CodeServiceUnavailable, Message: message, Status: http.StatusServiceUnavailable}
}

// From converts any error into an *Error.
//
// An *Error anywhere in the chain is returned as is, so a wrapped error keeps
// its status and code. Anything else becomes a 500 with the original error as
// the internal cause: an error nobody classified is not something to describe
// to a client.
func From(err error) *Error {
	if err == nil {
		return nil
	}

	if appErr, ok := errors.AsType[*Error](err); ok {
		return appErr
	}

	return Internal(err)
}

// Wrap adds a client-safe message while preserving the classification. A
// wrapped *Error keeps its status, code and details; anything else becomes a
// 500.
func Wrap(err error, message string) *Error {
	if appErr, ok := errors.AsType[*Error](err); ok {
		return &Error{
			Code:     appErr.Code,
			Message:  message,
			Status:   appErr.Status,
			Details:  appErr.Details,
			Internal: err,
		}
	}

	return InternalWithMessage(message, err)
}
