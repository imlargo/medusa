package medusa

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Context keys
const (
	ContextUserIDKey    = "medusa_user_id"
	ContextRequestIDKey = "medusa_request_id"
	ContextStartTimeKey = "medusa_start_time"
)

// Helpers type-safe
func GetUserID(c *gin.Context) (uint, bool) {
	id, exists := c.Get(ContextUserIDKey)
	if !exists {
		return 0, false
	}
	return id.(uint), true
}

func SetUserID(c *gin.Context, id uint) {
	c.Set(ContextUserIDKey, id)
}

// Context wraps gin.Context with additional helpers.
// It provides a cleaner API for common operations while
// maintaining full access to the underlying Gin context.
type Context struct {
	*gin.Context
}

// NewContext creates a new Medusa context from a Gin context.
func NewContext(c *gin.Context) *Context {
	ctx := &Context{Context: c}
	return ctx
}

// Ctx returns the standard library context.
// Use this when calling services that need context.Context.
func (c *Context) Ctx() context.Context {
	return c.Request.Context()
}

// RequestID returns the request ID if set.
func (c *Context) RequestID() string {
	if id, exists := c.Get(ContextRequestIDKey); exists {
		return id.(string)
	}
	return ""
}

// SetRequestID sets the request ID (typically called by middleware).
func (c *Context) SetRequestID(id string) {
	c.Set(ContextRequestIDKey, id)
	c.Header("X-Request-ID", id)
}

// StartTime returns when the request started.
func (c *Context) StartTime() time.Time {
	if t, exists := c.Get(ContextStartTimeKey); exists {
		return t.(time.Time)
	}
	return time.Time{}
}

// Elapsed returns the time elapsed since request start.
func (c *Context) Elapsed() time.Duration {
	start := c.StartTime()
	if start.IsZero() {
		return 0
	}
	return time.Since(start)
}

// SetValue sets a value in the context.
func (c *Context) SetValue(key string, value interface{}) {
	c.Set(key, value)
}

// GetValue gets a value from the context.
func (c *Context) GetValue(key string) (interface{}, bool) {
	return c.Get(key)
}

// MustGetValue gets a value or panics if not found.
func (c *Context) MustGetValue(key string) interface{} {
	val, exists := c.Get(key)
	if !exists {
		panic("medusa: required context value not found: " + key)
	}
	return val
}

// UserID returns the authenticated user's ID.
// Returns 0 and false if user is not authenticated.
func (c *Context) UserID() (uint, bool) {
	id, exists := c.Get(ContextUserIDKey)
	if !exists {
		return 0, false
	}

	switch v := id.(type) {
	case uint:
		return v, true
	case int:
		return uint(v), true
	case int64:
		return uint(v), true
	case float64:
		return uint(v), true
	default:
		return 0, false
	}
}

// SetUserID sets the authenticated user ID (called by auth middleware).
func (c *Context) SetUserID(id uint) {
	c.Set(ContextUserIDKey, id)
}

// IsAuthenticated returns true if user is authenticated.
func (c *Context) IsAuthenticated() bool {
	_, exists := c.UserID()
	return exists
}
