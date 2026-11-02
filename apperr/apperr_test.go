package apperr_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/imlargo/medusa/apperr"
)

var errEmailTaken = apperr.Conflict("user.email_taken", "a user with this email already exists")

func TestNewRejectsMalformedCodes(t *testing.T) {
	for _, code := range []string{"", "user", "User.taken", "user.", ".taken", "user.email-taken", "user..taken", "1user.taken"} {
		t.Run(code, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("New(%q) did not panic", code)
				}
			}()
			apperr.New(apperr.KindInvalid, code, "message")
		})
	}
}

func TestNewAcceptsDomainRuleCodes(t *testing.T) {
	for _, code := range []string{"user.email_taken", "billing.invoice.already_paid", "auth2.invalid_token"} {
		if got := apperr.New(apperr.KindInvalid, code, "m").Code(); got != code {
			t.Errorf("Code() = %q, want %q", got, code)
		}
	}
}

// A copy made by a With method is still the declared error.
func TestIsMatchesByCode(t *testing.T) {
	cause := errors.New("duplicate key")
	extended := errEmailTaken.WithCause(cause).WithMeta("k", 1).WithFields(apperr.FieldError{Field: "email"})
	wrapped := fmt.Errorf("register: %w", extended)

	if !errors.Is(wrapped, errEmailTaken) {
		t.Error("errors.Is(wrapped copy, declared) = false, want true")
	}
	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is(wrapped copy, cause) = false, want the cause reachable")
	}
	other := apperr.Conflict("user.handle_taken", "taken")
	if errors.Is(wrapped, other) {
		t.Error("errors.Is matched an error with another code")
	}
}

func TestWithMethodsNeverChangeTheDeclaredError(t *testing.T) {
	_ = errEmailTaken.WithCause(errors.New("x")).WithMeta("k", 1).WithFields(apperr.FieldError{Field: "email"})

	if errEmailTaken.Cause() != nil || errEmailTaken.Meta() != nil || errEmailTaken.Fields() != nil {
		t.Errorf("declared error was modified: cause=%v meta=%v fields=%v",
			errEmailTaken.Cause(), errEmailTaken.Meta(), errEmailTaken.Fields())
	}

	first := errEmailTaken.WithMeta("a", 1)
	second := first.WithMeta("b", 2)
	if _, ok := first.Meta()["b"]; ok {
		t.Error("WithMeta on a copy changed the copy it came from")
	}
	if len(second.Meta()) != 2 {
		t.Errorf("Meta() = %v, want both keys", second.Meta())
	}
}

func TestFrom(t *testing.T) {
	plain := errors.New("connection refused")
	wrappedDeclared := fmt.Errorf("get user 7: %w", errEmailTaken)

	tests := []struct {
		name      string
		err       error
		wantCode  string
		wantKind  apperr.Kind
		wantCause error
	}{
		{name: "declared error", err: errEmailTaken, wantCode: "user.email_taken", wantKind: apperr.KindConflict},
		{name: "wrapped declared error keeps the wrapping as cause", err: wrappedDeclared, wantCode: "user.email_taken", wantKind: apperr.KindConflict, wantCause: wrappedDeclared},
		{name: "canceled", err: fmt.Errorf("query: %w", context.Canceled), wantCode: "server.canceled", wantKind: apperr.KindCanceled},
		{name: "deadline", err: context.DeadlineExceeded, wantCode: "server.timeout", wantKind: apperr.KindUnavailable},
		{name: "unclassified", err: plain, wantCode: "server.internal", wantKind: apperr.KindInternal, wantCause: plain},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apperr.From(tt.err)
			if got.Code() != tt.wantCode || got.Kind() != tt.wantKind {
				t.Errorf("From() = %s/%s, want %s/%s", got.Code(), got.Kind(), tt.wantCode, tt.wantKind)
			}
			if tt.wantCause != nil && got.Cause() != tt.wantCause {
				t.Errorf("Cause() = %v, want %v", got.Cause(), tt.wantCause)
			}
		})
	}

	if apperr.From(nil) != nil {
		t.Error("From(nil) != nil")
	}
}

func TestErrorStringIsForLogs(t *testing.T) {
	got := errEmailTaken.WithCause(errors.New("23505")).Error()
	want := "user.email_taken: a user with this email already exists: 23505"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
