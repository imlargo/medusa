package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa/middleware"
)

// A browser client has to be able to send the request ID and the API key,
// and to read the request ID and the retry headers back.
func TestCORSAllowsAndExposesMedusasHeaders(t *testing.T) {
	router := gin.New()
	router.Use(middleware.CORS([]string{"https://app.example.com"}))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	preflight := httptest.NewRequest(http.MethodOptions, "/x", nil)
	preflight.Header.Set("Origin", "https://app.example.com")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflight.Header.Set("Access-Control-Request-Headers", "x-request-id, x-api-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, preflight)

	allowed := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, h := range []string{"x-request-id", "x-api-key"} {
		if !strings.Contains(allowed, h) {
			t.Errorf("Access-Control-Allow-Headers = %q, want it to include %s", allowed, h)
		}
	}

	get := httptest.NewRequest(http.MethodGet, "/x", nil)
	get.Header.Set("Origin", "https://app.example.com")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, get)

	exposed := strings.ToLower(rec.Header().Get("Access-Control-Expose-Headers"))
	for _, h := range []string{"x-request-id", "retry-after", "ratelimit"} {
		if !strings.Contains(exposed, h) {
			t.Errorf("Access-Control-Expose-Headers = %q, want it to include %s", exposed, h)
		}
	}
}
