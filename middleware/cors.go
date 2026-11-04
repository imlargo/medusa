package middleware

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS allows browser calls from origins, with credentials, the standard
// methods and the request headers Medusa reads. Wildcard origins such as
// "https://*.example.com" are supported.
//
// Forwarding headers (X-Forwarded-For...) are not allowed: they are set by
// proxies, and a page that could send them would be choosing the client IP
// the rate limiter and the logs see.
func CORS(origins []string) gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:  origins,
		AllowWildcard: true,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowCredentials: true,
		AllowHeaders: []string{
			"Origin",
			"Accept",
			"Authorization",
			"Content-Type",
			requestIDHeader,
			apiKeyHeader,
		},
		// Without these a browser client cannot read the request ID to report
		// a problem, or the headers that tell it when to retry.
		ExposeHeaders: []string{
			"Content-Length",
			"Content-Type",
			requestIDHeader,
			"Retry-After",
			"RateLimit",
			"RateLimit-Policy",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
		},
	})
}
