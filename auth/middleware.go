package auth

import (
	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
)

// ErrInvalidToken: the bearer token is malformed, forged, expired or from
// another issuer. The reason goes to the log, never to the client.
var ErrInvalidToken = apperr.Unauthenticated("auth.invalid_token", "invalid or expired token")

// subjectKey holds the authenticated subject in the Gin context.
const subjectKey = "medusa_auth_subject"

// Middleware requires an "Authorization: Bearer <token>" header with a token
// j accepts, and makes its subject available through SubjectFrom. Anything
// else is answered with a 401 before the handler runs.
func Middleware[S Subject](j *JWT[S]) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := medusa.NewContext(c).BearerToken()
		if err != nil {
			medusa.AbortWithError(c, err)
			return
		}

		claims, err := j.Parse(token)
		if err != nil {
			medusa.AbortWithError(c, ErrInvalidToken.WithCause(err))
			return
		}

		c.Set(subjectKey, claims.Subject)
		c.Next()
	}
}

// SubjectFrom returns the subject Middleware authenticated, and false when
// the request did not pass through Middleware or S is not the type its JWT
// was built with.
func SubjectFrom[S Subject](c *medusa.Context) (S, bool) {
	value, ok := c.Get(subjectKey)
	if !ok {
		var zero S
		return zero, false
	}

	subject, ok := value.(S)
	return subject, ok
}
