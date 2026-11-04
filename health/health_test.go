package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
		{Name: "redis", Status: StatusUnhealthy},
	}
	if len(report.Checks) != len(want) {
		t.Fatalf("checks = %v, want %v", report.Checks, want)
	}
	for i := range want {
		got := report.Checks[i]
		if got.Name != want[i].Name || got.Status != want[i].Status {
			t.Errorf("checks[%d] = %+v, want %+v", i, got, want[i])
		}
	}
	if err := report.Checks[1].Err; err == nil || err.Error() != "connection refused" {
		t.Errorf("checks[1].Err = %v, want the check's error kept for the caller", err)
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
	svc.Register("database", func(context.Context) error { return errors.New("dial tcp db.internal:5432: refused") })

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
	// /ready is public: a driver error names internal hosts.
	if strings.Contains(rec.Body.String(), "db.internal") {
		t.Errorf("body = %s, want the check's error kept out of it", rec.Body)
	}
}

// A draining instance must leave rotation even though its dependencies are
// fine, and Drain must hold shutdown back for the delay.
func TestDrainTakesTheInstanceOutOfRotation(t *testing.T) {
	svc := New(time.Second)
	svc.Register("database", func(context.Context) error { return nil })

	start := time.Now()
	if err := svc.Drain(t.Context(), 50*time.Millisecond); err != nil {
		t.Fatalf("Drain() = %v", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("Drain returned after %s, want it to wait the delay", elapsed)
	}

	report := svc.Check(t.Context())
	if report.Status != StatusUnhealthy || !report.Draining {
		t.Errorf("Check() = %+v, want unhealthy and draining", report)
	}
}

func TestDrainStopsWaitingWhenTheContextEnds(t *testing.T) {
	svc := New(time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	start := time.Now()
	_ = svc.Drain(ctx, time.Minute)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Drain waited %s with a canceled context, want it to return", elapsed)
	}
}
