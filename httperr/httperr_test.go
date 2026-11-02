package httperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestConstructorsSetStatusAndCode(t *testing.T) {
	tests := []struct {
		name       string
		err        *Error
		wantStatus int
		wantCode   Code
	}{
		{"bad request", BadRequest("bad"), http.StatusBadRequest, CodeBadRequest},
		{"validation", Validation("invalid", nil), http.StatusBadRequest, CodeValidation},
		{"unauthorized", Unauthorized("nope"), http.StatusUnauthorized, CodeUnauthorized},
		{"forbidden", Forbidden("nope"), http.StatusForbidden, CodeForbidden},
		{"not found", NotFound("user"), http.StatusNotFound, CodeNotFound},
		{"conflict", Conflict("taken"), http.StatusConflict, CodeConflict},
		{"payload too large", PayloadTooLarge(10), http.StatusRequestEntityTooLarge, CodePayloadTooLarge},
		{"too many requests", TooManyRequests(30), http.StatusTooManyRequests, CodeTooManyRequests},
		{"unavailable", ServiceUnavailable("draining"), http.StatusServiceUnavailable, CodeServiceUnavailable},
		{"internal", Internal(errors.New("boom")), http.StatusInternalServerError, CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Status != tt.wantStatus {
				t.Errorf("status = %d, want %d", tt.err.Status, tt.wantStatus)
			}
			if tt.err.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", tt.err.Code, tt.wantCode)
			}
		})
	}
}

// A classified error keeps its classification through wrapping, which is what
// lets callers wrap freely without turning a 404 into a 500.
func TestFromUnwrapsAWrappedError(t *testing.T) {
	original := NotFound("user")
	wrapped := fmt.Errorf("loading profile: %w", original)

	got := From(wrapped)

	if got.Status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", got.Status, http.StatusNotFound)
	}
	if got.Code != CodeNotFound {
		t.Errorf("code = %q, want %q", got.Code, CodeNotFound)
	}
}

func TestFromClassifiesAnUnknownErrorAsInternal(t *testing.T) {
	cause := errors.New("boom")

	got := From(cause)

	if got.Status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", got.Status, http.StatusInternalServerError)
	}
	if !errors.Is(got, cause) {
		t.Error("errors.Is(From(cause), cause) = false, want the cause kept for the log")
	}
}

func TestFromNil(t *testing.T) {
	if got := From(nil); got != nil {
		t.Errorf("From(nil) = %v, want nil", got)
	}
}

func TestWrapKeepsClassificationAndChainsTheCause(t *testing.T) {
	original := Conflict("email already registered")

	wrapped := Wrap(original, "could not create the account")

	if wrapped.Status != http.StatusConflict {
		t.Errorf("status = %d, want %d", wrapped.Status, http.StatusConflict)
	}
	if wrapped.Message != "could not create the account" {
		t.Errorf("message = %q, want the new one", wrapped.Message)
	}
	if !errors.Is(wrapped, original) {
		t.Error("errors.Is(wrapped, original) = false, want the cause to be reachable")
	}
}

func TestWrapClassifiesAnUnknownErrorAsInternal(t *testing.T) {
	wrapped := Wrap(errors.New("boom"), "failed")

	if wrapped.Status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", wrapped.Status, http.StatusInternalServerError)
	}
}

func TestErrorMessageIncludesTheCauseForLogs(t *testing.T) {
	err := InternalWithMessage("could not save", errors.New("disk full"))

	if got, want := err.Error(), "could not save: disk full"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
