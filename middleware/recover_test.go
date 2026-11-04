package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/httperr"
	"github.com/imlargo/medusa/middleware"
)

func TestRecoverAnswersAPanicWithAnInternalProblem(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	var seenStatus int
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Next()
		seenStatus = c.Writer.Status()
	})
	router.Use(middleware.Recover(logger))
	router.GET("/x", func(*gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Header().Get("Content-Type"); got != httperr.ContentType {
		t.Errorf("Content-Type = %q, want %q", got, httperr.ContentType)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body["code"] != apperr.ErrInternal.Code() {
		t.Errorf("code = %v, want %q", body["code"], apperr.ErrInternal.Code())
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("body = %s, want the panic value kept out of the response", rec.Body)
	}
	if seenStatus != http.StatusInternalServerError {
		t.Errorf("outer middleware saw %d, want %d", seenStatus, http.StatusInternalServerError)
	}
	if !strings.Contains(logs.String(), "boom") {
		t.Errorf("log = %s, want the panic value", logs.String())
	}
}

func TestRecoverLetsErrAbortHandlerThrough(t *testing.T) {
	router := gin.New()
	router.Use(middleware.Recover(slog.New(slog.DiscardHandler)))
	router.GET("/x", func(*gin.Context) { panic(http.ErrAbortHandler) })

	defer func() {
		if v := recover(); v != http.ErrAbortHandler { //nolint:errorlint // identity, as net/http checks it
			t.Errorf("recovered %v, want http.ErrAbortHandler to reach net/http", v)
		}
	}()
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
}
