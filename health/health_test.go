package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestCheckReportsEveryResultInRegistrationOrder(t *testing.T) {
	svc := New(time.Second)
	svc.Register("database", func(context.Context) error { return nil })
	svc.Register("redis", func(context.Context) error { return errors.New("connection refused") })

	report := svc.Check(t.Context())

	if report.Status != StatusUnhealthy {
		t.Errorf("status = %q, want %q", report.Status, StatusUnhealthy)
	}

	want := []Result{
		{Name: "database", Status: StatusHealthy},
		{Name: "redis", Status: StatusUnhealthy, Message: "connection refused"},
	}
	if len(report.Checks) != len(want) {
		t.Fatalf("checks = %v, want %v", report.Checks, want)
	}
	for i := range want {
		if report.Checks[i] != want[i] {
			t.Errorf("checks[%d] = %+v, want %+v", i, report.Checks[i], want[i])
		}
	}
}

func TestCheckBoundsTheRoundWithTheTimeout(t *testing.T) {
	svc := New(20 * time.Millisecond)
	svc.Register("slow", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	if report := svc.Check(t.Context()); report.Status != StatusUnhealthy {
		t.Errorf("status = %q, want %q for a check past the timeout", report.Status, StatusUnhealthy)
	}
}

func TestReadyReplies503WhenACheckFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := New(time.Second)
	svc.Register("database", func(context.Context) error { return errors.New("down") })

	router := gin.New()
	router.GET("/ready", NewHandler(svc).Ready)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After is missing, want the prober told when to retry")
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body["status"] != StatusUnhealthy {
		t.Errorf("status = %v, want %q", body["status"], StatusUnhealthy)
	}
}
