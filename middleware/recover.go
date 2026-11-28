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

// PanicError is the error a recovered panic becomes: the cause of the
// server.internal error the request answers with. Error reporting reads the
// stack from it.
type PanicError struct {
	// Value is what was passed to panic.
	Value any

	// Stack is the stack of the goroutine that panicked.
	Stack []byte
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

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

			stack := debug.Stack()
			logger.LogAttrs(c.Request.Context(), slog.LevelError, "panic recovered",
				slog.Any("panic", v),
				slog.String("stack", string(stack)),
			)

			if c.Writer.Written() {
				// The response has started; all that is left is to stop.
				c.Abort()
				return
			}
			medusa.AbortWithError(c, apperr.ErrInternal.WithCause(&PanicError{Value: v, Stack: stack}))
		}()

		c.Next()
	}
}
