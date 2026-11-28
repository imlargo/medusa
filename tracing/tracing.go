// Package tracing is Medusa's tracing battery: it follows one piece of work
// through a request, the queries it runs, the jobs it enqueues and the
// services it calls, as an OpenTelemetry trace.
//
// Set it up once per process, and close it on the way out so the last spans
// are sent:
//
//	shutdown, err := tracing.Setup(ctx, tracing.Config{ServiceName: "api"})
//	defer shutdown(context.Background())
//
// Then connect each layer, one line each:
//
//	router.Use(tracing.Middleware("api"))                        // a span per request
//	httpclient.New(httpclient.WithTransport(tracing.Transport(nil))) // a span per call, and the trace sent along
//	slog.New(tracing.LogHandler(handler))                        // trace_id on every log line
//
// Queries and jobs are connected through hooks, so this module imports
// neither the database nor the queue: see [QueryObserver], [Inject],
// [StartJob] and [EndJob].
//
// Spans are exported over OTLP/HTTP to the endpoint in the standard
// OTEL_EXPORTER_OTLP_ENDPOINT variable: a collector, or any vendor that
// speaks OTLP. With no endpoint configured, Setup traces nothing and costs
// nothing.
package tracing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// tracerName identifies the spans this package creates itself.
const tracerName = "github.com/imlargo/medusa/tracing"

// Environment variables of the OTLP exporter that mean "there is somewhere
// to send spans".
const (
	envEndpoint       = "OTEL_EXPORTER_OTLP_ENDPOINT"
	envTracesEndpoint = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
)

// Config is how a process traces.
type Config struct {
	// ServiceName names the process in every trace: "api", "worker".
	ServiceName string

	// Version is the release the process runs, shown on its spans.
	Version string

	// SampleRatio is the share of traces kept, from 0 to 1, for work this
	// process starts. A request that arrives already traced follows the
	// decision of whoever started the trace. Zero means 1: keep everything,
	// which is right until the volume says otherwise.
	SampleRatio float64

	// Exporter replaces the OTLP exporter, mostly for tests.
	Exporter sdktrace.SpanExporter
}

// Setup starts tracing for the process and returns the function that stops
// it, flushing the spans not yet sent. Call that function before exiting.
//
// Without an Exporter in cfg and without an OTLP endpoint in the
// environment, nothing is traced: the returned function does nothing, and
// every helper of this package is cheap.
func Setup(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	// The trace travels between services in the W3C traceparent header,
	// whether or not this process exports spans.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	exporter := cfg.Exporter
	if exporter == nil {
		if os.Getenv(envEndpoint) == "" && os.Getenv(envTracesEndpoint) == "" {
			return func(context.Context) error { return nil }, nil
		}
		exporter, err = otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("tracing: build the OTLP exporter: %w", err)
		}
	}

	attributes := []attribute.KeyValue{attribute.String("service.name", cfg.ServiceName)}
	if cfg.Version != "" {
		attributes = append(attributes, attribute.String("service.version", cfg.Version))
	}
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(attributes...))
	if err != nil {
		return nil, fmt.Errorf("tracing: describe the service: %w", err)
	}

	ratio := cfg.SampleRatio
	if ratio <= 0 {
		ratio = 1
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)
	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}

// Middleware starts a span for every request, named after its route, and
// continues the trace of a caller that sent one. Put it first, before the
// middleware whose work should be inside the span.
func Middleware(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}

// Transport traces outbound HTTP calls made through base (the default
// transport when nil): a span per call, and the trace sent to the service
// called, so its spans join this one. Give it to httpclient.WithTransport.
func Transport(base http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(base)
}

// QueryObserver traces database statements. It has the shape of
// postgres.WithQueryObserver: called when a statement starts, it returns the
// context to run it with and a function to call when it ends.
//
//	postgres.Open(ctx, url, logger, postgres.WithQueryObserver(tracing.QueryObserver))
//
// The statement's text is recorded, never its arguments: those are data.
func QueryObserver(ctx context.Context, statement string) (context.Context, func(err error)) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "postgres.query",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.statement", statement)),
	)
	return ctx, func(err error) { end(span, err) }
}

// Inject writes the trace of ctx into carrier, a job's metadata for example,
// so that work done later, in another process, continues the same trace.
func Inject(ctx context.Context, carrier map[string]string) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(carrier))
}

// StartJob starts the span of a job's run. carrier is what [Inject] wrote
// when the job was enqueued: the span continues that trace, so a request and
// the jobs it caused read as one story. End it with [EndJob].
//
//	queue.WithJobContext(func(ctx context.Context, job *queue.Job) context.Context {
//	    return tracing.StartJob(ctx, job.Kind, job.Queue, job.Metadata)
//	})
func StartJob(ctx context.Context, kind, queue string, carrier map[string]string) context.Context {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(carrier))
	ctx, _ = otel.Tracer(tracerName).Start(ctx, "job "+kind,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.String("messaging.destination.name", queue), attribute.String("job.kind", kind)),
	)
	return ctx
}

// EndJob ends the span [StartJob] started, with the run's error if it had
// one. ctx is the context StartJob returned, or one derived from it.
//
//	queue.WithOnFinish(func(ctx context.Context, job *queue.Job, _ time.Duration, err error) {
//	    tracing.EndJob(ctx, err)
//	})
func EndJob(ctx context.Context, err error) {
	end(trace.SpanFromContext(ctx), err)
}

func end(span trace.Span, err error) {
	// A caller that went away is not a failure of the work.
	if err != nil && !errors.Is(err, context.Canceled) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// LogHandler wraps h so every record logged with a traced context carries
// trace_id and span_id: from a log line to its trace, and back. Wrap it
// around medusa.LogHandler, or inside it; the order does not matter.
func LogHandler(h slog.Handler) slog.Handler {
	return logHandler{h}
}

type logHandler struct{ slog.Handler }

func (h logHandler) Handle(ctx context.Context, r slog.Record) error {
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		r.AddAttrs(slog.String("trace_id", span.TraceID().String()), slog.String("span_id", span.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return logHandler{h.Handler.WithAttrs(attrs)}
}

func (h logHandler) WithGroup(name string) slog.Handler {
	return logHandler{h.Handler.WithGroup(name)}
}
