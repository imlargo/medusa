package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
	"github.com/imlargo/medusa/auth"
)

// serve runs one request through handlers and returns the response.
func serve(header http.Header, handlers ...gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/x", handlers...)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header = header
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func bearer(token string) http.Header {
	return http.Header{"Authorization": {"Bearer " + token}}
}

// The point of the principal: code that only has a context.Context, as a
// service does, sees who is calling.
func TestThePrincipalReachesCodeThatOnlyHasAContext(t *testing.T) {
	tokens := auth.MustNewJWT[int64](secret)
	token, err := tokens.Sign(7, time.Now().Add(time.Hour), auth.WithScopes("orders:read", "orders:write"), auth.WithTenant("acme"))
	if err != nil {
		t.Fatalf("Sign() = %v", err)
	}

	var seen auth.Principal
	service := func(ctx context.Context) { seen, _ = auth.PrincipalFrom(ctx) }

	rec := serve(bearer(token), auth.Middleware(tokens), func(c *gin.Context) {
		service(c.Request.Context())
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	want := auth.Principal{Kind: auth.KindUser, Subject: "7", Tenant: "acme", Scopes: []string{"orders:read", "orders:write"}, Method: auth.MethodJWT}
	if seen.Kind != want.Kind || seen.Subject != want.Subject || seen.Tenant != want.Tenant || seen.Method != want.Method || strings.Join(seen.Scopes, " ") != strings.Join(want.Scopes, " ") {
		t.Errorf("principal = %+v, want %+v", seen, want)
	}
}

// A handler's own ctx satisfies context.Context without carrying the
// request's values; passing it must still find the principal.
func TestPrincipalFromAcceptsAHandlersContext(t *testing.T) {
	tokens := auth.MustNewJWT[string](secret)
	token, _ := tokens.Sign("u1", time.Now().Add(time.Hour))

	var fromGin, fromMedusa bool
	serve(bearer(token), auth.Middleware(tokens), func(c *gin.Context) {
		_, fromGin = auth.PrincipalFrom(c)
		_, fromMedusa = auth.SubjectFrom[string](medusa.NewContext(c))
	})

	if !fromGin || !fromMedusa {
		t.Errorf("found from *gin.Context = %v, from *medusa.Context = %v, want both", fromGin, fromMedusa)
	}
}

func apiKeys(t *testing.T) auth.Authenticator {
	t.Helper()
	known := auth.HashAPIKey("key-of-the-engine")

	return auth.APIKeys(func(_ context.Context, keyHash string) (auth.Principal, error) {
		if keyHash != known {
			return auth.Principal{}, auth.ErrInvalidAPIKey
		}
		return auth.Principal{Kind: auth.KindService, Subject: "engine", Scopes: []string{"renders:callback"}}, nil
	})
}

// Users and services call the same routes, each with its own credential.
func TestMiddlewareTriesItsAuthenticatorsInOrder(t *testing.T) {
	tokens := auth.MustNewJWT[int64](secret)
	token, _ := tokens.Sign(7, time.Now().Add(time.Hour))
	middleware := auth.Middleware(tokens, apiKeys(t))

	who := func(c *gin.Context) {
		p, _ := auth.PrincipalFrom(c.Request.Context())
		c.String(http.StatusOK, string(p.Kind)+":"+p.Subject+":"+p.Method)
	}

	tests := []struct {
		name     string
		header   http.Header
		status   int
		contains string
	}{
		{"a user's token", bearer(token), http.StatusOK, "user:7:jwt"},
		{"a service's key", http.Header{"X-Api-Key": {"key-of-the-engine"}}, http.StatusOK, "service:engine:api_key"},
		{"a wrong key ends the request", http.Header{"X-Api-Key": {"guess"}}, http.StatusUnauthorized, auth.ErrInvalidAPIKey.Code()},
		{"a wrong token is not retried as a key", http.Header{"Authorization": {"Bearer nope"}, "X-Api-Key": {"key-of-the-engine"}}, http.StatusUnauthorized, auth.ErrInvalidToken.Code()},
		{"no credentials", http.Header{}, http.StatusUnauthorized, medusa.ErrMissingCredentials.Code()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(tt.header, middleware, who)
			if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.contains) {
				t.Errorf("response = %d %s, want %d containing %q", rec.Code, rec.Body, tt.status, tt.contains)
			}
		})
	}
}

func TestRequireGuardsByScope(t *testing.T) {
	tokens := auth.MustNewJWT[int64](secret)
	reader, _ := tokens.Sign(7, time.Now().Add(time.Hour), auth.WithScopes("orders:read"))
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }

	if rec := serve(bearer(reader), auth.Middleware(tokens), auth.Require("orders:read"), ok); rec.Code != http.StatusOK {
		t.Errorf("with the scope = %d, want 200", rec.Code)
	}

	rec := serve(bearer(reader), auth.Middleware(tokens), auth.Require("orders:read", "orders:write"), ok)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), auth.ErrInsufficientScope.Code()) {
		t.Errorf("without one scope = %d %s, want 403 %s", rec.Code, rec.Body, auth.ErrInsufficientScope.Code())
	}

	if rec := serve(http.Header{}, auth.Require("orders:read"), ok); rec.Code != http.StatusUnauthorized {
		t.Errorf("without Middleware = %d, want 401", rec.Code)
	}
}

var errNotAMember = apperr.PermissionDenied("workspace.not_a_member", "you are not a member of this workspace")

// The tenant is chosen per request and checked now, not read from a token
// signed when the membership may have been different.
func TestEnrichSetsWhatOnlyTheApplicationKnows(t *testing.T) {
	tokens := auth.MustNewJWT[int64](secret)
	token, _ := tokens.Sign(7, time.Now().Add(time.Hour))

	memberships := map[string][]string{"acme": {"orders:read"}}
	enrich := auth.Enrich(func(c *medusa.Context, p auth.Principal) (auth.Principal, error) {
		workspace := c.GetHeader("X-Workspace")
		scopes, member := memberships[workspace]
		if !member {
			return p, errNotAMember
		}
		p.Tenant, p.Scopes = workspace, scopes
		return p, nil
	})
	show := func(c *gin.Context) {
		p, _ := auth.PrincipalFrom(c.Request.Context())
		c.String(http.StatusOK, p.Tenant)
	}

	header := bearer(token)
	header.Set("X-Workspace", "acme")
	if rec := serve(header, auth.Middleware(tokens), enrich, auth.Require("orders:read"), show); rec.Code != http.StatusOK || rec.Body.String() != "acme" {
		t.Errorf("a member = %d %s, want 200 acme", rec.Code, rec.Body)
	}

	header.Set("X-Workspace", "globex")
	if rec := serve(header, auth.Middleware(tokens), enrich, show); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), errNotAMember.Code()) {
		t.Errorf("not a member = %d %s, want 403 %s", rec.Code, rec.Body, errNotAMember.Code())
	}
}

// A job runs as whoever caused it, and decides again what it may do.
func TestMetadataCarriesWhoButNotWhatWasAllowed(t *testing.T) {
	original := auth.Principal{Kind: auth.KindUser, Subject: "7", Tenant: "acme", Scopes: []string{"orders:write"}, Method: auth.MethodJWT}

	restored, ok := auth.PrincipalFromMetadata(original.Metadata())
	if !ok || restored.Kind != auth.KindUser || restored.Subject != "7" || restored.Tenant != "acme" {
		t.Errorf("restored = %+v, %v, want the same kind, subject and tenant", restored, ok)
	}
	if len(restored.Scopes) != 0 {
		t.Errorf("restored scopes = %v, want none", restored.Scopes)
	}

	if _, ok := auth.PrincipalFromMetadata(map[string]string{"other": "x"}); ok {
		t.Error("PrincipalFromMetadata() of metadata without a principal = true, want false")
	}
}
