package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
)

// ReportErrors calls report for every error behind a 5xx response: an error
// nobody classified, a declared internal error, a dependency that failed, a
// recovered panic (a *[PanicError], with its stack). It is the hook for an
// error tracker such as Sentry; Medusa ships none.
//
// report gets the request's context, which carries the request ID
// (medusa.RequestIDFrom) and the caller when there is one, and the request
// itself through c. The error is the application error: its code, its
// message and its cause. Client errors (4xx) are not reported: they are the
// caller's, and belong in metrics, not in an error tracker.
//
// Place it before Recover, so it sees panics too:
//
//	router.Use(middleware.ReportErrors(report), middleware.Recover(logger))
func ReportErrors(report func(c *medusa.Context, err error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Status() < http.StatusInternalServerError {
			return
		}
		ctx := medusa.NewContext(c)
		for _, recorded := range c.Errors {
			report(ctx, recorded.Err)
		}
	}
}
