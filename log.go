package medusa

import (
	"context"
	"log/slog"
)

type requestIDContextKey struct{}

// WithRequestID returns ctx carrying the request ID, for code that only has a
// context.Context: services, repositories, and the logs they write.
// middleware.RequestID calls it for every request.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, id)
}

// RequestIDFrom returns the request ID ctx carries, or "" outside a request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

// LogHandler wraps h so that every record logged with a request's context
// (logger.InfoContext(ctx, ...)) carries a request_id attribute, without each
// call adding it. Build the process logger with it once:
//
//	logger := slog.New(medusa.LogHandler(slog.NewJSONHandler(os.Stderr, nil)))
func LogHandler(h slog.Handler) slog.Handler {
	return logHandler{h}
}

type logHandler struct{ slog.Handler }

func (h logHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDFrom(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return logHandler{h.Handler.WithAttrs(attrs)}
}

func (h logHandler) WithGroup(name string) slog.Handler {
	return logHandler{h.Handler.WithGroup(name)}
}
