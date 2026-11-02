package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit rejects request bodies larger than maxBytes.
//
// Without a cap the server reads whatever a client sends into memory, so a
// single request can exhaust the process. A size cap is the right tool here
// rather than a read deadline, which would also cut long-lived streams.
//
// Reading past the limit surfaces as *http.MaxBytesError, which the binding
// helpers translate into a 413 rather than a generic 400.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}

		c.Next()
	}
}
