package tracing_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/imlargo/medusa/tracing"
)

// setup starts tracing into memory and returns what was recorded so far.
// Tracing is process-wide, so these tests do not run in parallel.
func setup(t *testing.T) func() tracetest.SpanStubs {
	t.Helper()
	gin.SetMode(gin.TestMode)

	exporter := tracetest.NewInMemoryExporter()
	// A simple processor exports each span as it ends, so a test reads it
	// without waiting for a batch.
	shutdown, err := tracing.Setup(t.Context(), tracing.Config{ServiceName: "test", Exporter: syncExporter{exporter}})
	if err != nil {
		t.Fatalf("Setup() = %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	return func() tracetest.SpanStubs {
		// Shutdown flushes the batch; spans are then all in the exporter.
		_ = shutdown(context.Background())
		return exporter.GetSpans()
	}
}

// syncExporter keeps the exported spans after the provider shuts down, which
// the in-memory exporter otherwise clears.
type syncExporter struct{ *tracetest.InMemoryExporter }

func (syncExporter) Shutdown(context.Context) error { return nil }

func find(spans tracetest.SpanStubs, name string) *tracetest.SpanStub {
	for i := range spans {
		if spans[i].Name == name {
			return &spans[i]
		}
	}
	return nil
}

// One request: its span, the query it ran and the call it made are one
// trace, and the service it called receives that trace.
func TestARequestItsQueryAndItsCallAreOneTrace(t *testing.T) {
	spans := setup(t)

	var receivedTraceparent string
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		receivedTraceparent = r.Header.Get("traceparent")
	}))
	defer upstream.Close()
	client := &http.Client{Transport: tracing.Transport(nil)}

	router := gin.New()
	router.Use(tracing.Middleware("test"))
	router.GET("/orders/:id", func(c *gin.Context) {
		ctx := c.Request.Context()

		_, done := tracing.QueryObserver(ctx, "SELECT * FROM orders WHERE id = $1")
		done(nil)

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("outbound call: %v", err)
			return
		}
		_ = resp.Body.Close()
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/orders/7", nil))

	recorded := spans()
	request := find(recorded, "GET /orders/:id")
	query := find(recorded, "postgres.query")
	call := find(recorded, "HTTP GET")
	if request == nil || query == nil || call == nil {
		names := make([]string, len(recorded))
		for i, s := range recorded {
			names[i] = s.Name
		}
		t.Fatalf("spans = %v, want the request, the query and the outbound call", names)
	}

	traceID := request.SpanContext.TraceID()
	if query.SpanContext.TraceID() != traceID || call.SpanContext.TraceID() != traceID {
		t.Error("the query and the call are not in the request's trace")
	}
	if query.Parent.SpanID() != request.SpanContext.SpanID() {
		t.Error("the query is not a child of the request's span")
	}
	if !strings.Contains(receivedTraceparent, traceID.String()) {
		t.Errorf("the called service received traceparent %q, want the trace %s", receivedTraceparent, traceID)
	}
	for _, attr := range query.Attributes {
		if attr.Key == "db.statement" && strings.Contains(attr.Value.AsString(), "7") {
			t.Error("the query span carries an argument's value, want the statement only")
		}
	}
}

// A job runs later and elsewhere. What Inject wrote when it was enqueued is
// enough for its span to continue the trace of the request that caused it.
func TestAJobContinuesTheTraceThatEnqueuedIt(t *testing.T) {
	spans := setup(t)

	router := gin.New()
	router.Use(tracing.Middleware("test"))
	metadata := map[string]string{}
	router.POST("/orders", func(c *gin.Context) {
		tracing.Inject(c.Request.Context(), metadata) // stored with the job
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/orders", nil))

	if metadata["traceparent"] == "" {
		t.Fatalf("metadata = %v, want a traceparent to carry", metadata)
	}

	// Later, in the worker, with a context that knows nothing of the request.
	jobCtx := tracing.StartJob(context.Background(), "orders.send_receipt", "mail", metadata)
	tracing.EndJob(jobCtx, errors.New("smtp is down"))

	recorded := spans()
	request, job := find(recorded, "POST /orders"), find(recorded, "job orders.send_receipt")
	if request == nil || job == nil {
		t.Fatalf("spans = %d, want the request and the job", len(recorded))
	}
	if job.SpanContext.TraceID() != request.SpanContext.TraceID() {
		t.Error("the job is not in the trace of the request that enqueued it")
	}
	if job.Parent.SpanID() != request.SpanContext.SpanID() {
		t.Error("the job's span is not a child of the request's span")
	}
	if job.Status.Description != "smtp is down" || len(job.Events) == 0 {
		t.Errorf("job span status = %+v with %d events, want the run's error recorded", job.Status, len(job.Events))
	}
}

// A job with nothing to continue starts a trace of its own.
func TestAJobWithoutACarrierStartsItsOwnTrace(t *testing.T) {
	spans := setup(t)

	tracing.EndJob(tracing.StartJob(context.Background(), "nightly.cleanup", "default", nil), nil)

	job := find(spans(), "job nightly.cleanup")
	if job == nil || !job.SpanContext.TraceID().IsValid() || job.Parent.IsValid() {
		t.Errorf("job span = %+v, want a root span with a trace of its own", job)
	}
}

func TestLogHandlerAddsTheTraceToLogLines(t *testing.T) {
	setup(t)

	var out bytes.Buffer
	logger := slog.New(tracing.LogHandler(slog.NewJSONHandler(&out, nil)))

	ctx := tracing.StartJob(context.Background(), "k", "q", nil)
	logger.InfoContext(ctx, "inside a trace")
	logger.InfoContext(context.Background(), "outside")
	tracing.EndJob(ctx, nil)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	traceID := trace.SpanContextFromContext(ctx).TraceID().String()
	if !strings.Contains(lines[0], `"trace_id":"`+traceID+`"`) || !strings.Contains(lines[0], `"span_id"`) {
		t.Errorf("line inside a trace = %s, want its trace_id and span_id", lines[0])
	}
	if strings.Contains(lines[1], "trace_id") {
		t.Errorf("line outside a trace = %s, want no trace_id", lines[1])
	}
}

// Tracing must cost nothing and break nothing where it is not configured.
func TestWithoutAnEndpointNothingIsTracedAndEverythingStillWorks(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	shutdown, err := tracing.Setup(t.Context(), tracing.Config{ServiceName: "test"})
	if err != nil {
		t.Fatalf("Setup() = %v", err)
	}
	if err := shutdown(t.Context()); err != nil {
		t.Errorf("shutdown() = %v, want nil", err)
	}

	ctx, done := tracing.QueryObserver(t.Context(), "SELECT 1")
	done(nil)
	tracing.Inject(ctx, map[string]string{})
	tracing.EndJob(tracing.StartJob(ctx, "k", "q", nil), nil)
}

var _ sdktrace.SpanExporter = syncExporter{}
