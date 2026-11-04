package medusa

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/httperr"
)

var errUserNotFound = apperr.NotFound("user.not_found", "user not found")

// render runs WriteError against a throwaway context and decodes the result.
func render(t *testing.T, err error, setup func(*gin.Context)) (*httptest.ResponseRecorder, httperr.Problem) {
	t.Helper()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if setup != nil {
		setup(c)
	}

	WriteError(c, err)

	var body httperr.Problem
	if rec.Body.Len() > 0 {
		if decodeErr := json.Unmarshal(rec.Body.Bytes(), &body); decodeErr != nil {
			t.Fatalf("response body is not a problem: %v (%q)", decodeErr, rec.Body.String())
		}
	}
	return rec, body
}

func TestWriteErrorRendersAProblem(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"declared error", errUserNotFound, http.StatusNotFound, "user.not_found"},
		{"wrapped declared error", fmt.Errorf("get user 7: %w", errUserNotFound), http.StatusNotFound, "user.not_found"},
		{"rate limited", apperr.New(apperr.KindResourceExhausted, "request.rate_limited", "slow down"), http.StatusTooManyRequests, "request.rate_limited"},
		{"unclassified", errors.New("boom"), http.StatusInternalServerError, "server.internal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := render(t, tt.err, nil)

			if rec.Code != tt.wantStatus || body.Status != tt.wantStatus {
				t.Errorf("status = %d (body %d), want %d", rec.Code, body.Status, tt.wantStatus)
			}
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Content-Type"); got != httperr.ContentType {
				t.Errorf("Content-Type = %q, want %q", got, httperr.ContentType)
			}
		})
	}
}

// The cause is for the log, never for the client.
func TestWriteErrorNeverLeaksTheCause(t *testing.T) {
	secret := "connection to 10.0.0.5 refused: password authentication failed"

	for name, err := range map[string]error{
		"unclassified":        errors.New(secret),
		"declared with cause": errUserNotFound.WithCause(errors.New(secret)),
		"wrapped declared":    fmt.Errorf("%s: %w", secret, errUserNotFound),
	} {
		t.Run(name, func(t *testing.T) {
			rec, _ := render(t, err, nil)
			if strings.Contains(rec.Body.String(), secret) {
				t.Errorf("response leaked the cause: %s", rec.Body)
			}
		})
	}
}

// The cause still has to reach the access log, which reads Gin's error list.
func TestWriteErrorRecordsTheCauseForLogging(t *testing.T) {
	cause := errors.New("boom")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	WriteError(c, errUserNotFound.WithCause(cause))

	if len(c.Errors) == 0 {
		t.Fatal("c.Errors is empty, want the cause recorded")
	}
	if got := c.Errors.Last().Err; !errors.Is(got, cause) {
		t.Errorf("recorded error = %v, want %v", got, cause)
	}
}

func TestWriteErrorIncludesTheRequestID(t *testing.T) {
	const id = "req-123"

	_, body := render(t, errUserNotFound, func(c *gin.Context) {
		c.Set(RequestIDKey, id)
	})

	if body.RequestID != id {
		t.Errorf("request_id = %q, want %q", body.RequestID, id)
	}
}

func TestWriteErrorOnNilWritesNothing(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	WriteError(c, nil)

	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
}

func TestAbortWithErrorStopsTheChain(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	AbortWithError(c, ErrMissingCredentials)

	if !c.IsAborted() {
		t.Error("IsAborted() = false, want the chain stopped")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// A list with no rows must still be a list: null breaks clients that iterate it.
func TestNilSlicesAreSentAsEmptyLists(t *testing.T) {
	type item struct{ ID int }

	tests := []struct {
		name  string
		write func(c *gin.Context)
	}{
		{"WriteSuccess", func(c *gin.Context) { WriteSuccess(c, http.StatusOK, MessageOK, []item(nil)) }},
		{"Paged", func(c *gin.Context) { Paged(NewContext(c), []item(nil), Pagination{}, 0) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			tt.write(c)

			var body map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if got := string(body["data"]); got != "[]" {
				t.Errorf("data = %s, want []", got)
			}
		})
	}
}
