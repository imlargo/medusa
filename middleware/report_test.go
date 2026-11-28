package middleware_test

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/middleware"
)

var errNotFound = apperr.NotFound("order.not_found", "order not found")

func TestReportErrorsReportsWhatAnswers5xxAndNothingElse(t *testing.T) {
	type report struct {
		err       error
		requestID string
	}
	var reports []report

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.ReportErrors(func(c *medusa.Context, err error) {
			reports = append(reports, report{err: err, requestID: medusa.RequestIDFrom(c.Ctx())})
		}),
		middleware.Recover(slog.New(slog.DiscardHandler)),
	)
	boom := errors.New("connection reset")
	router.GET("/unclassified", medusa.Handler(func(*medusa.Context) error { return boom }))
	router.GET("/panic", func(*gin.Context) { panic("nil map") })
	router.GET("/not-found", medusa.Handler(func(*medusa.Context) error { return errNotFound.WithCause(boom) }))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	get := func(path string) {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	get("/ok")
	get("/not-found")
	if len(reports) != 0 {
		t.Fatalf("reports after a 200 and a 404 = %d, want none: a client's mistake is not an incident", len(reports))
	}

	get("/unclassified")
	if len(reports) != 1 || !errors.Is(reports[0].err, boom) || !errors.Is(reports[0].err, apperr.ErrInternal) {
		t.Fatalf("reports after an unclassified error = %+v, want server.internal caused by it", reports)
	}
	if reports[0].requestID == "" {
		t.Error("the report has no request ID, want the one of its request")
	}

	get("/panic")
	if len(reports) != 2 {
		t.Fatalf("reports after a panic = %d, want 2", len(reports))
	}
	panicErr, ok := errors.AsType[*middleware.PanicError](reports[1].err)
	if !ok || panicErr.Value != "nil map" || len(panicErr.Stack) == 0 {
		t.Errorf("panic report = %v, want a PanicError with the value and the stack", reports[1].err)
	}
}
