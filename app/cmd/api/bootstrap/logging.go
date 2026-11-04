package bootstrap

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// accessLogMiddleware logs one structured entry per request through the
// application logger, replacing gin.Default()'s plain-text writer so access
// logs end up in the same JSON stream as everything else.
//
// Server errors (5xx) are logged at error level, client errors (4xx) at warn
// and everything else at info, which keeps log-based alerting straightforward.
func accessLogMiddleware(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		status := c.Writer.Status()
		path := c.FullPath()
		if path == "" {
			// No route matched: log what the client actually asked for.
			path = c.Request.URL.Path
		}

		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", time.Since(start)),
			slog.String("ip", c.ClientIP()),
		}

		if err := c.Errors.ByType(gin.ErrorTypePrivate).String(); err != "" {
			attrs = append(attrs, slog.String("error", err))
		}

		ctx := c.Request.Context()
		switch {
		case status >= 500:
			log.LogAttrs(ctx, slog.LevelError, "request failed", attrs...)
		case status >= 400:
			log.LogAttrs(ctx, slog.LevelWarn, "request rejected", attrs...)
		default:
			log.LogAttrs(ctx, slog.LevelInfo, "request completed", attrs...)
		}
	}
}
