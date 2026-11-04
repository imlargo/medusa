package medusa

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequestIDKey is the Gin context key holding the current request ID. The
// request-ID middleware writes it and every response reads it, so both go
// through this one constant.
const RequestIDKey = "medusa_request_id"

// Context is the request context handed to every Medusa handler. It embeds
// *gin.Context, so every Gin method remains available.
type Context struct {
	*gin.Context
}

// NewContext wraps a *gin.Context. The Handle adapters call it for you; it is
// exported for middleware and for handlers registered directly with Gin.
func NewContext(c *gin.Context) *Context {
	return &Context{Context: c}
}

// Ctx returns the request's context.Context, for passing to services.
func (c *Context) Ctx() context.Context {
	return c.Request.Context()
}

// RequestID returns the request ID set by the request-ID middleware, or an
// empty string when the middleware is not installed.
func (c *Context) RequestID() string {
	return requestID(c.Context)
}

// BearerToken extracts the token from an "Authorization: Bearer <token>"
// header. It returns ErrMissingCredentials or ErrMalformedCredentials when
// the header is missing, malformed or carries an empty token.
func (c *Context) BearerToken() (string, error) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", ErrMissingCredentials
	}

	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "bearer") {
		return "", ErrMalformedCredentials
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrMalformedCredentials
	}
	return token, nil
}

func requestID(c *gin.Context) string {
	id, _ := c.Get(RequestIDKey)
	value, _ := id.(string)
	return value
}
