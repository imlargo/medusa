package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/middleware"
)

// requestIDFor sends one request with the given X-Request-ID header (none when
// empty) and returns the ID the handler saw and the one echoed back.
func requestIDFor(t *testing.T, header string) (seen, echoed string) {
	t.Helper()

	router := gin.New()
	router.Use(middleware.RequestID())
	router.GET("/x", func(c *gin.Context) {
		seen = medusa.NewContext(c).RequestID()
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if header != "" {
		req.Header.Set("X-Request-ID", header)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return seen, rec.Header().Get("X-Request-ID")
}

func TestRequestIDReusesAValidUUIDInCanonicalForm(t *testing.T) {
	const given = "F81D4FAE-7DEC-11D0-A765-00A0C91E6BF6"
	const want = "f81d4fae-7dec-11d0-a765-00a0c91e6bf6"

	seen, echoed := requestIDFor(t, given)

	if seen != want || echoed != want {
		t.Errorf("RequestID = %q, header = %q, want both %q", seen, echoed, want)
	}
}

// Arbitrary client text must never reach the logs or the response headers.
func TestRequestIDReplacesAnythingThatIsNotAUUID(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"missing", ""},
		{"free text", "hello"},
		{"log forging", "abc\nlevel=error msg=forged"},
		{"too long", "0123456789abcdef0123456789abcdef0123456789abcdef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen, echoed := requestIDFor(t, tt.header)

			if _, err := uuid.Parse(seen); err != nil {
				t.Fatalf("RequestID = %q, want a generated UUID", seen)
			}
			if seen == tt.header {
				t.Errorf("RequestID = %q, want the client value replaced", seen)
			}
			if echoed != seen {
				t.Errorf("header = %q, want it to match RequestID %q", echoed, seen)
			}
		})
	}
}
