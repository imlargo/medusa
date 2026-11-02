package auth

import (
	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/httperr"
)

// Middleware requires an "Authorization: Bearer <token>" header with a valid
// access token and makes the user's ID available through UserID.
func Middleware(j *JWT) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := medusa.NewContext(c).BearerToken()
		if err != nil {
			medusa.AbortWithError(c, httperr.Unauthorized(err.Error()))
			return
		}

		if token == "" {
			medusa.AbortWithError(c, httperr.Unauthorized("token is empty"))
			return
		}

		claims, err := j.ParseToken(token)
		if err != nil {
			medusa.AbortWithError(c, httperr.Unauthorized("invalid or expired token"))
			return
		}

		setUserID(c, claims.UserID)
		c.Next()
	}
}
