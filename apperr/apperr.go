// Package apperr is the application error model: what went wrong, in terms
// any caller understands, independent of the transport that reports it.
//
// Every error a client may see is declared once, next to the rule it
// enforces, with a Kind and a stable code:
//
//	var ErrEmailTaken = apperr.Conflict("user.email_taken", "a user with this email already exists")
//
// Services return it, optionally with the underlying cause for the log:
//
//	return nil, ErrEmailTaken.WithCause(err)
//
// Callers branch on it with errors.Is, which matches by code. The HTTP edge
// (package httperr) turns the Kind into a status and the error into an
// RFC 9457 problem; a job or a queue consumer can read the same error without
// knowing anything about HTTP.
//
// An error that is not an *Error is a bug nobody classified: From turns it
// into an internal error whose cause is logged and never shown.
package apperr

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
)

// Kind is the category of a failure. It decides how each transport reports
// it (the HTTP status, for example), so pick it by what the caller can do
// about the error, not by where it happened.
type Kind uint8

const (
	// KindInternal is a failure the caller cannot fix: a bug or a broken
	// dependency. It is the zero value, so an unclassified error is internal.
	KindInternal Kind = iota

	// KindInvalid: the input breaks a rule, whatever the current state. The
	// caller must change the request.
	KindInvalid

	// KindNotFound: the thing the caller referred to does not exist, or the
	// caller may not know it exists.
	KindNotFound

	// KindConflict: the request clashes with existing state, such as a
	// duplicate or a concurrent change. Retrying the same request fails again
	// until that state changes.
	KindConflict

	// KindFailedPrecondition: the request is valid, but a business rule
	// forbids it in the current state (an order already shipped, an account
	// not verified yet).
	KindFailedPrecondition

	// KindUnauthenticated: the caller's identity is missing or invalid.
	KindUnauthenticated

	// KindPermissionDenied: the caller is known but not allowed to do this.
	KindPermissionDenied

	// KindTooLarge: the input is over a size limit.
	KindTooLarge

	// KindResourceExhausted: a quota or rate limit ran out. Retrying later
	// may succeed.
	KindResourceExhausted

	// KindUnavailable: the operation could not finish now (a dependency is
	// down, a deadline expired). Retrying later may succeed.
	KindUnavailable

	// KindCanceled: the caller gave up before the operation finished.
	KindCanceled
)

var kindNames = [...]string{
	KindInternal:           "internal",
	KindInvalid:            "invalid",
	KindNotFound:           "not_found",
	KindConflict:           "conflict",
	KindFailedPrecondition: "failed_precondition",
	KindUnauthenticated:    "unauthenticated",
	KindPermissionDenied:   "permission_denied",
	KindTooLarge:           "too_large",
	KindResourceExhausted:  "resource_exhausted",
	KindUnavailable:        "unavailable",
	KindCanceled:           "canceled",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", k)
}

// codePattern is the shape of every code: lowercase dot-separated segments,
// the first naming the domain that owns the rule (user.email_taken).
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// FieldError points at one invalid input field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error is an application error. Its code and message are part of the API
// contract: the code is stable and machine-readable, the message is safe to
// show to the caller. The cause is for the log only.
//
// An *Error is immutable: the With methods return a copy, so a package-level
// error can be returned from any goroutine and extended safely.
type Error struct {
	kind    Kind
	code    string
	message string
	fields  []FieldError
	meta    map[string]any
	cause   error
}

// New declares an error. code must be dot-separated lowercase segments,
// starting with the owning domain (user.email_taken); New panics otherwise,
// so a malformed code fails when its package loads. message must be safe to
// show to a client: never build it from an error or from user input.
func New(kind Kind, code, message string) *Error {
	if !codePattern.MatchString(code) {
		panic(fmt.Sprintf("apperr: code %q must look like domain.rule (lowercase, dot-separated)", code))
	}
	return &Error{kind: kind, code: code, message: message}
}

// Invalid declares a KindInvalid error.
func Invalid(code, message string) *Error { return New(KindInvalid, code, message) }

// NotFound declares a KindNotFound error.
func NotFound(code, message string) *Error { return New(KindNotFound, code, message) }

// Conflict declares a KindConflict error.
func Conflict(code, message string) *Error { return New(KindConflict, code, message) }

// FailedPrecondition declares a KindFailedPrecondition error.
func FailedPrecondition(code, message string) *Error {
	return New(KindFailedPrecondition, code, message)
}

// Unauthenticated declares a KindUnauthenticated error.
func Unauthenticated(code, message string) *Error { return New(KindUnauthenticated, code, message) }

// PermissionDenied declares a KindPermissionDenied error.
func PermissionDenied(code, message string) *Error {
	return New(KindPermissionDenied, code, message)
}

// Internal declares a KindInternal error with a message of its own, for a
// failure worth naming ("could not issue the token"). Unnamed failures need
// no declaration: From turns them into ErrInternal.
func Internal(code, message string) *Error { return New(KindInternal, code, message) }

// Errors Medusa reports on its own, when nothing more specific applies.
var (
	// ErrInternal stands in for any error nobody classified.
	ErrInternal = Internal("server.internal", "an internal error occurred")

	// ErrCanceled reports that the caller went away (context.Canceled).
	ErrCanceled = New(KindCanceled, "server.canceled", "the request was canceled")

	// ErrTimeout reports that the operation ran out of time
	// (context.DeadlineExceeded).
	ErrTimeout = New(KindUnavailable, "server.timeout", "the request took too long, retry later")
)

// Kind is the error's category.
func (e *Error) Kind() Kind { return e.kind }

// Code is the stable, machine-readable code, such as user.email_taken.
func (e *Error) Code() string { return e.code }

// Message is the client-safe description.
func (e *Error) Message() string { return e.message }

// Fields lists the invalid input fields, if any.
func (e *Error) Fields() []FieldError { return slices.Clone(e.fields) }

// Meta holds structured facts a client can act on, such as
// retry_after_seconds, or nil.
func (e *Error) Meta() map[string]any { return maps.Clone(e.meta) }

// Cause is the underlying error, for the log. It is never shown to a client.
func (e *Error) Cause() error { return e.cause }

// Error is for logs: it includes the cause, so never send it to a client.
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.code, e.message, e.cause)
	}
	return e.code + ": " + e.message
}

// Unwrap exposes the cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.cause }

// Is matches any *Error with the same code, so a copy made by a With method
// still matches the declared error:
//
//	errors.Is(ErrEmailTaken.WithCause(err), ErrEmailTaken) // true
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.code == e.code
}

// WithCause returns a copy that carries cause for the log.
func (e *Error) WithCause(cause error) *Error {
	c := e.clone()
	c.cause = cause
	return c
}

// WithFields returns a copy that also points at the given invalid fields.
func (e *Error) WithFields(fields ...FieldError) *Error {
	c := e.clone()
	c.fields = append(slices.Clone(e.fields), fields...)
	return c
}

// WithMeta returns a copy that also carries key=value for the client. Keys
// are snake_case facts the client can act on (retry_after_seconds), never
// internals.
func (e *Error) WithMeta(key string, value any) *Error {
	c := e.clone()
	c.meta = maps.Clone(e.meta)
	if c.meta == nil {
		c.meta = make(map[string]any, 1)
	}
	c.meta[key] = value
	return c
}

func (e *Error) clone() *Error {
	c := *e
	return &c
}

// From returns the *Error in err's chain. Without one, a canceled context
// becomes ErrCanceled, an expired deadline ErrTimeout, and anything else
// ErrInternal, each carrying err as its cause. From(nil) is nil.
func From(err error) *Error {
	if err == nil {
		return nil
	}

	if appErr, ok := errors.AsType[*Error](err); ok {
		if appErr.cause == nil && appErr != err {
			// err wrapped a declared error: keep the wrapping for the log.
			return appErr.WithCause(err)
		}
		return appErr
	}

	switch {
	case errors.Is(err, context.Canceled):
		return ErrCanceled.WithCause(err)
	case errors.Is(err, context.DeadlineExceeded):
		return ErrTimeout.WithCause(err)
	}

	return ErrInternal.WithCause(err)
}
