package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/auth"
)

func newProtectedRouter(tokens *auth.JWT[int64]) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/me", auth.Middleware(tokens), func(c *gin.Context) {
		id, ok := auth.SubjectFrom[int64](medusa.NewContext(c))
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.String(http.StatusOK, strconv.FormatInt(id, 10))
	})
	return router
}

func TestMiddleware(t *testing.T) {
	tokens := auth.MustNewJWT[int64](secret)
	valid, err := tokens.Sign(7, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Sign() = %v", err)
	}

	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantBody   string
	}{
		{name: "valid token", header: "Bearer " + valid, wantStatus: http.StatusOK, wantBody: "7"},
		{name: "no header", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", header: "Basic " + valid, wantStatus: http.StatusUnauthorized},
		{name: "invalid token", header: "Bearer nope", wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			newProtectedRouter(tokens).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body, tt.wantBody)
			}
		})
	}
}

func TestSubjectFromWithoutMiddleware(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if id, ok := auth.SubjectFrom[int64](medusa.NewContext(c)); ok {
		t.Errorf("SubjectFrom() = %d, true; want false outside the middleware", id)
	}
}
