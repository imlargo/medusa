package middleware

import (
	"strings"
	"uuid"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
)

// requestIDHeader carries the request ID in both directions.
const requestIDHeader = "X-Request-ID"

// RequestID gives every request an ID and echoes it in the response.
// Handlers read it with medusa.Context.RequestID, and code that only has the
// request's context.Context with medusa.RequestIDFrom, which is also how
// medusa.LogHandler adds it to every log line.
//
// A caller's X-Request-ID is reused only when it is a valid UUID, rewritten in
// canonical form. Anything else is replaced: the ID is written to logs and
// response headers, and reflecting arbitrary client text there would let a
// caller forge log lines or inject header content.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := uuid.New().String()
		if parsed, err := uuid.Parse(strings.TrimSpace(c.GetHeader(requestIDHeader))); err == nil {
			id = parsed.String()
		}

		c.Set(medusa.RequestIDKey, id)
		c.Request = c.Request.WithContext(medusa.WithRequestID(c.Request.Context(), id))
		c.Header(requestIDHeader, id)
		c.Next()
	}
}
