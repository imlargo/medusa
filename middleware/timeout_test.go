package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/middleware"
)

// Work that waits on the request's context gives up at the deadline, and the
// client gets a 503 server.timeout instead of waiting for the write timeout.
func TestTimeoutEndsSlowWorkWithA503(t *testing.T) {
	router := gin.New()
	router.Use(middleware.Timeout(20 * time.Millisecond))
	router.GET("/x", func(c *gin.Context) {
		<-c.Request.Context().Done()
		medusa.WriteError(c, c.Request.Context().Err())
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestTimeoutLeavesEventStreamsOpen(t *testing.T) {
	var deadlineSet bool
	router := gin.New()
	router.Use(middleware.Timeout(time.Millisecond))
	router.GET("/events", func(c *gin.Context) {
		_, deadlineSet = c.Request.Context().Deadline()
	})

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	router.ServeHTTP(httptest.NewRecorder(), req)

	if deadlineSet {
		t.Error("an event stream got a deadline, want it exempt")
	}
}

// The mapping the middleware relies on.
func TestADeadlineIsServerTimeout(t *testing.T) {
	if !errors.Is(apperr.From(context.DeadlineExceeded), apperr.ErrTimeout) {
		t.Errorf("From(DeadlineExceeded) = %v, want ErrTimeout", apperr.From(context.DeadlineExceeded))
	}
}
