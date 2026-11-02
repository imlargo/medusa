package httperr_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/httperr"
)

func TestStatus(t *testing.T) {
	tests := []struct {
		kind apperr.Kind
		want int
	}{
		{apperr.KindInternal, http.StatusInternalServerError},
		{apperr.KindInvalid, http.StatusBadRequest},
		{apperr.KindNotFound, http.StatusNotFound},
		{apperr.KindConflict, http.StatusConflict},
		{apperr.KindFailedPrecondition, http.StatusUnprocessableEntity},
		{apperr.KindUnauthenticated, http.StatusUnauthorized},
		{apperr.KindPermissionDenied, http.StatusForbidden},
		{apperr.KindTooLarge, http.StatusRequestEntityTooLarge},
		{apperr.KindResourceExhausted, http.StatusTooManyRequests},
		{apperr.KindUnavailable, http.StatusServiceUnavailable},
		{apperr.KindCanceled, httperr.StatusClientClosedRequest},
		{apperr.Kind(200), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.kind.String(), func(t *testing.T) {
			if got := httperr.Status(tt.kind); got != tt.want {
				t.Errorf("Status(%s) = %d, want %d", tt.kind, got, tt.want)
			}
		})
	}
}

func TestNewProblem(t *testing.T) {
	err := apperr.Invalid("user.invalid_input", "some fields are invalid").
		WithFields(apperr.FieldError{Field: "email", Message: "invalid email format"})

	p := httperr.NewProblem(err)

	if p.Type != "about:blank" || p.Title != "Bad Request" || p.Status != http.StatusBadRequest {
		t.Errorf("type/title/status = %q/%q/%d, want about:blank/Bad Request/400", p.Type, p.Title, p.Status)
	}
	if p.Code != "user.invalid_input" || p.Detail != "some fields are invalid" {
		t.Errorf("code/detail = %q/%q", p.Code, p.Detail)
	}
	if len(p.Errors) != 1 || p.Errors[0].Field != "email" {
		t.Errorf("errors = %+v, want the email field", p.Errors)
	}
}

// The cause of an unclassified error is for the log, never for the client.
func TestNewProblemHidesTheCause(t *testing.T) {
	secret := "password authentication failed for 10.0.0.5"

	body, err := json.Marshal(httperr.NewProblem(errors.New(secret)))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), secret) {
		t.Errorf("problem leaked the cause: %s", body)
	}
	if !strings.Contains(string(body), `"code":"server.internal"`) {
		t.Errorf("problem = %s, want code server.internal", body)
	}
}

func TestProblemSendsMetadataAsMembers(t *testing.T) {
	err := apperr.New(apperr.KindResourceExhausted, "request.rate_limited", "rate limit exceeded").
		WithMeta("retry_after_seconds", 30).
		WithMeta("status", "ignored: collides with a member")

	body, marshalErr := json.Marshal(httperr.NewProblem(err))
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["retry_after_seconds"] != float64(30) {
		t.Errorf("retry_after_seconds = %v, want 30", decoded["retry_after_seconds"])
	}
	if decoded["status"] != float64(http.StatusTooManyRequests) {
		t.Errorf("status = %v, want the member, not the colliding metadata", decoded["status"])
	}
}

func TestWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	p := httperr.NewProblem(apperr.NotFound("user.not_found", "user not found"))
	p.RequestID = "req-1"

	if err := httperr.Write(rec, p); err != nil {
		t.Fatalf("Write() = %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != httperr.ContentType {
		t.Errorf("Content-Type = %q, want %q", got, httperr.ContentType)
	}
	if !strings.Contains(rec.Body.String(), `"request_id":"req-1"`) {
		t.Errorf("body = %s, want the request id", rec.Body)
	}
}
