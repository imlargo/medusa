package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// unmatchedRoute labels requests no route matched. Labeling them with the raw
// URL path would let any client mint a new time series per request.
const unmatchedRoute = "unmatched"

// MetricsRecorder receives one observation per request. metrics.HTTP
// implements it.
type MetricsRecorder interface {
	RecordHTTPRequest(method, path, status string)
	RecordHTTPDuration(method, path, status string, duration time.Duration)
}

// Metrics records a count and a duration for every request, labeled by
// method, route template and status. Server-sent event streams and OPTIONS
// preflights are skipped.
func Metrics(recorder MetricsRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Accept") == "text/event-stream" || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		start := time.Now()

		c.Next()

		duration := time.Since(start)
		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())

		path := c.FullPath()
		if path == "" {
			path = unmatchedRoute
		}

		recorder.RecordHTTPRequest(method, path, status)
		recorder.RecordHTTPDuration(method, path, status, duration)
	}
}
