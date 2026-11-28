package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/apperr"
)

var (
	// ErrInvalidToken: the bearer token is malformed, forged, expired or from
	// another issuer. The reason goes to the log, never to the client.
	ErrInvalidToken = apperr.Unauthenticated("auth.invalid_token", "invalid or expired token")

	// ErrInsufficientScope: the caller is known, and may not do this.
	ErrInsufficientScope = apperr.PermissionDenied("auth.insufficient_scope", "this action is not allowed for these credentials")
)

// ErrNoCredentials is what an [Authenticator] returns when the request does
// not carry the kind of credential it reads, so [Middleware] tries the next
// one. A credential that is present and wrong is a different error, and ends
// the request.
var ErrNoCredentials = errors.New("auth: the request carries no credentials of this kind")

// Authenticator establishes who sent a request from one kind of credential:
// a bearer token, an API key, a session cookie.
type Authenticator interface {
	Authenticate(r *http.Request) (Principal, error)
}

// AuthenticatorFunc is a function used as an Authenticator.
type AuthenticatorFunc func(r *http.Request) (Principal, error)

// Authenticate calls f.
func (f AuthenticatorFunc) Authenticate(r *http.Request) (Principal, error) { return f(r) }

// Middleware requires a request to authenticate with one of authenticators,
// tried in order, and puts the [Principal] in the request's context for
// [PrincipalFrom]. A request with no credentials any of them reads answers
// 401 auth.missing_credentials; one with a wrong credential answers what its
// authenticator returned.
//
//	protected := router.Group("/v1", auth.Middleware(tokens, apiKeys))
func Middleware(authenticators ...Authenticator) gin.HandlerFunc {
	if len(authenticators) == 0 {
		panic("auth: Middleware needs at least one authenticator")
	}

	return func(c *gin.Context) {
		for _, authenticator := range authenticators {
			principal, err := authenticator.Authenticate(c.Request)
			if errors.Is(err, ErrNoCredentials) {
				continue
			}
			if err != nil {
				medusa.AbortWithError(c, err)
				return
			}

			c.Request = c.Request.WithContext(WithPrincipal(c.Request.Context(), principal))
			c.Next()
			return
		}

		medusa.AbortWithError(c, medusa.ErrMissingCredentials)
	}
}

// Enrich completes the principal after [Middleware] with what only the
// application knows, on every request: the tenant the request acts in (from
// a header or the path, checked against the caller's memberships), and
// scopes derived from a role or a plan read now, not when the token was
// signed. An error from fn ends the request; return a declared apperr error
// for a caller who may not act in that tenant.
func Enrich(fn func(c *medusa.Context, p Principal) (Principal, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := PrincipalFrom(c.Request.Context())
		if !ok {
			medusa.AbortWithError(c, medusa.ErrMissingCredentials)
			return
		}

		enriched, err := fn(medusa.NewContext(c), principal)
		if err != nil {
			medusa.AbortWithError(c, err)
			return
		}

		c.Request = c.Request.WithContext(WithPrincipal(c.Request.Context(), enriched))
		c.Next()
	}
}

// Require lets a request through only if its principal has every one of
// scopes. It answers 403 auth.insufficient_scope otherwise, and 401 when the
// route was reached without [Middleware].
func Require(scopes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := PrincipalFrom(c.Request.Context())
		if !ok {
			medusa.AbortWithError(c, medusa.ErrMissingCredentials)
			return
		}

		for _, scope := range scopes {
			if !principal.Has(scope) {
				medusa.AbortWithError(c, ErrInsufficientScope.WithMeta("required_scope", scope))
				return
			}
		}
		c.Next()
	}
}
