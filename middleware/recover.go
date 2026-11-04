package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
)

// Recover turns a panic in a later handler into a logged apperr.ErrInternal
// problem, so the request still gets a response in the API's error format,
// and middleware placed before it (the access log, metrics) still sees a 500.
// It replaces gin.Recovery, which writes an empty body and logs outside slog.
//
// http.ErrAbortHandler is passed on: it is how a handler asks net/http to
// abort the response without a log line.
func Recover(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler { //nolint:errorlint // net/http compares this sentinel by identity too
				panic(v)
			}

			logger.LogAttrs(c.Request.Context(), slog.LevelError, "panic recovered",
				slog.Any("panic", v),
				slog.String("request_id", c.GetString(medusa.RequestIDKey)),
				slog.String("stack", string(debug.Stack())),
			)

			if c.Writer.Written() {
				// The response has started; all that is left is to stop.
				c.Abort()
				return
			}
			medusa.AbortWithError(c, apperr.ErrInternal.WithCause(fmt.Errorf("panic: %v", v)))
		}()

		c.Next()
	}
}
