package middleware

import "github.com/gin-gonic/gin"

// SecureHeaders sets the response headers OWASP recommends for every API
// response: browsers must not guess a content type (nosniff), and no page
// may frame a response (frame-ancestors). They are cheap and only matter
// when a response ends up rendered by a browser.
func SecureHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Writer.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Content-Security-Policy", "frame-ancestors 'none'")
		c.Next()
	}
}
