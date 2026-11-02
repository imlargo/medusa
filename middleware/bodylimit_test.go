package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/middleware"
)

func init() { gin.SetMode(gin.TestMode) }

type payload struct {
	Text string `json:"text" binding:"required"`
}

func post(t *testing.T, limit int64, body string) (int, map[string]any) {
	t.Helper()

	router := gin.New()
	router.Use(middleware.BodyLimit(limit))
	router.POST("/x", medusa.Handle(func(ctx *medusa.Context, in *payload) (*payload, error) {
		return in, nil
	}))

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var decoded map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	return rec.Code, decoded
}

func TestBodyUnderTheLimitIsAccepted(t *testing.T) {
	if status, _ := post(t, 1024, `{"text":"hello"}`); status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
}

// An oversized body is not a validation problem: reporting it as a 400 sends the
// client hunting for a field that is perfectly fine.
func TestOversizedBodyIs413NotAValidationError(t *testing.T) {
	oversized := `{"text":"` + strings.Repeat("a", 2048) + `"}`

	status, body := post(t, 512, oversized)

	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", status, http.StatusRequestEntityTooLarge)
	}
	if body["code"] != medusa.ErrBodyTooLarge.Code() {
		t.Errorf("code = %v, want %q", body["code"], medusa.ErrBodyTooLarge.Code())
	}
	if body["max_bytes"] != float64(512) {
		t.Errorf("max_bytes = %v, want 512", body["max_bytes"])
	}
}
