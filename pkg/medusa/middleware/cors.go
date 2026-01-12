package middleware

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/imlargo/medusa/pkg/medusa/tools"
)

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
