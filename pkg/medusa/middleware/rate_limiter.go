package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/imlargo/medusa/pkg/medusa/core/ratelimiter"
	"github.com/imlargo/medusa/pkg/medusa/core/responses"
)

// NewRateLimiterMiddleware creates a middleware that enforces rate limiting per client IP.
// It uses the provided rate limiter to check if a request should be allowed.
// If the rate limit is exceeded, it returns a 429 Too Many Requests response
// with information about when to retry.
func NewRateLimiterMiddleware(rl ratelimiter.RateLimiter) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ip := ctx.ClientIP()
		allow, retryAfter := rl.Allow(ip)
		if !allow {
			message := "Rate limit exceeded. Try again in " + fmt.Sprintf("%.2f", retryAfter)
			responses.ErrorTooManyRequests(ctx, message)
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}
