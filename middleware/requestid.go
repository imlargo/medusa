package middleware

import (
	"uuid"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
)

// requestIDHeader carries the request ID in both directions.
const requestIDHeader = "X-Request-ID"

// RequestID gives every request an ID, reusing the caller's X-Request-ID when
// present, and echoes it in the response. Handlers read it with
// medusa.Context.RequestID.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = uuid.New().String()
		}

		c.Set(medusa.RequestIDKey, id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}
