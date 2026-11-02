package medusa

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa/httperr"
)

// render runs WriteError against a throwaway context and decodes the result.
func render(t *testing.T, err error, setup func(*gin.Context)) (int, ErrorResponse) {
	t.Helper()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if setup != nil {
		setup(c)
	}

	WriteError(c, err)

	var body ErrorResponse
	if rec.Body.Len() > 0 {
		if decodeErr := json.Unmarshal(rec.Body.Bytes(), &body); decodeErr != nil {
			t.Fatalf("response body is not an ErrorResponse: %v (%q)", decodeErr, rec.Body.String())
		}
	}
	return rec.Code, body
}

func TestWriteErrorDerivesStatusFromTheError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   httperr.Code
	}{
		{"not found", httperr.NotFound("user"), http.StatusNotFound, httperr.CodeNotFound},
		{"too many requests", httperr.TooManyRequests(30), http.StatusTooManyRequests, httperr.CodeTooManyRequests},
		{"unclassified", errors.New("boom"), http.StatusInternalServerError, httperr.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := render(t, tt.err, nil)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
			if body.Status != tt.wantStatus {
				t.Errorf("body status = %d, want %d", body.Status, tt.wantStatus)
			}
		})
	}
}

// The internal cause is for the log, never for the client.
func TestWriteErrorNeverLeaksTheInternalCause(t *testing.T) {
	secret := "connection to 10.0.0.5 refused: password authentication failed"

	status, body := render(t, httperr.Internal(errors.New(secret)), nil)

	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", status, http.StatusInternalServerError)
	}

	serialized, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	if body := string(serialized); strings.Contains(body, secret) {
		t.Errorf("response leaked the internal cause: %s", body)
	}
}

// The cause still has to reach the access log, which reads Gin's error list.
func TestWriteErrorRecordsTheCauseForLogging(t *testing.T) {
	cause := errors.New("boom")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	WriteError(c, httperr.Internal(cause))

	if len(c.Errors) == 0 {
		t.Fatal("c.Errors is empty, want the internal cause recorded")
	}
	if got := c.Errors.Last().Err; !errors.Is(got, cause) {
		t.Errorf("recorded error = %v, want %v", got, cause)
	}
}

func TestWriteErrorIncludesTheRequestID(t *testing.T) {
	const id = "req-123"

	_, body := render(t, httperr.NotFound("user"), func(c *gin.Context) {
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

	AbortWithError(c, httperr.Unauthorized("nope"))

	if !c.IsAborted() {
		t.Error("IsAborted() = false, want the chain stopped")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
