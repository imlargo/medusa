package middleware

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS allows browser calls from origins, with credentials, the standard
// methods and the common request headers. Wildcard origins such as
// "https://*.example.com" are supported.
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
			"X-Forwarded-For",
			"X-Real-IP",
			"X-Forwarded-Host",
			"X-Forwarded-Proto",
		},
		ExposeHeaders: []string{
			"Content-Length",
			"Content-Type",
		},
	})
}
