package medusa

import "github.com/gin-gonic/gin"

// Context keys used throughout the Medusa framework
const (
	ContextKeyUserID    = "user_id"
	ContextKeyRequestID = "request_id"
	ContextKeyTraceID   = "trace_id"
)

// Helpers type-safe
func GetUserID(c *gin.Context) (uint, bool) {
	id, exists := c.Get(ContextKeyUserID)
	if !exists {
		return 0, false
	}
	return id.(uint), true
}

func MustGetUserID(c *gin.Context) uint {
	id, ok := GetUserID(c)
	if !ok {
		panic("user_id not found in context")
	}
	return id
}
