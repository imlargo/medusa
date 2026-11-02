package middleware

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/imlargo/medusa/pkg/medusa/tools"
)

// NewCorsMiddleware creates a CORS middleware with the specified host and allowed origins.
// It configures standard HTTP methods, credentials support, and common headers.
// The host is automatically added to the list of allowed origins.
// Wildcard origins are supported for flexible cross-origin access.
func NewCorsMiddleware(host string, origins []string) gin.HandlerFunc {

	if origins == nil {
		origins = []string{}
	}

	allowedOrigins := append(origins, tools.ToCompleteURL(host))

	config := cors.Config{
		AllowOrigins:  allowedOrigins,
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
			"X-Forwarded-For",
			"X-Real-IP",
			"X-Forwarded-Host",
			"X-Forwarded-Proto",
		},
		ExposeHeaders: []string{
			"Content-Length",
			"Content-Type",
		},
	}

	return cors.New(config)
}
