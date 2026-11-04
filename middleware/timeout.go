package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// eventStream is the Accept value of a server-sent events request.
const eventStream = "text/event-stream"

// Timeout puts a deadline of d on every request's context, so database calls
// and outbound requests made with it give up once it passes. A handler that
// returns the resulting context.DeadlineExceeded answers 503 server.timeout
// (see apperr.From). Event streams are exempt: they are meant to stay open,
// and the server's write timeout still bounds each of their writes.
//
// The deadline is cooperative: work that ignores the context keeps running.
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Accept") == eventStream {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
