// Package middleware holds Gin middleware for Medusa applications: request
// IDs, CORS, body limits, metrics, rate limiting and API key checks.
//
// Every middleware that rejects a request renders the error through
// medusa.AbortWithError, so failures share the response format of handlers.
package middleware

import (
	"crypto/subtle"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
)

// ErrInvalidAPIKey: the request carries an API key, but not the right one.
var ErrInvalidAPIKey = apperr.Unauthenticated("auth.invalid_api_key", "invalid API key")

// apiKeyHeader carries the key APIKey checks.
const apiKeyHeader = "X-API-Key"

// APIKey rejects requests whose X-API-Key header does not match key.
func APIKey(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := c.GetHeader(apiKeyHeader)
		if got == "" {
			medusa.AbortWithError(c, medusa.ErrMissingCredentials)
			return
		}

		if !keysMatch(got, key) {
			medusa.AbortWithError(c, ErrInvalidAPIKey)
			return
		}

		c.Next()
	}
}

// BearerAPIKey rejects requests whose "Authorization: Bearer <key>" header
// does not carry key.
func BearerAPIKey(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		got, err := medusa.NewContext(c).BearerToken()
		if err != nil {
			medusa.AbortWithError(c, err)
			return
		}

		if !keysMatch(got, key) {
			medusa.AbortWithError(c, ErrInvalidAPIKey)
			return
		}

		c.Next()
	}
}

// keysMatch compares in constant time, so response timing reveals nothing
// about how much of a guessed key was right.
func keysMatch(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
