package auth

import (
	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
)

// userIDKey holds the authenticated user's ID in the Gin context.
const userIDKey = "auth_user_id"

// UserID returns the authenticated user's ID, and false when the request did
// not pass through Middleware.
func UserID(c *medusa.Context) (int64, bool) {
	id, ok := c.Get(userIDKey)
	if !ok {
		return 0, false
	}

	userID, ok := id.(int64)
	return userID, ok
}

func setUserID(c *gin.Context, id int64) {
	c.Set(userIDKey, id)
}
