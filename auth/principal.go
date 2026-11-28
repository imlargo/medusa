package auth

import (
	"context"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"

	"github.com/imlargo/medusa"
)

// Kind says what a principal is.
type Kind string

const (
	// KindUser is a person, signed in.
	KindUser Kind = "user"

	// KindService is another program acting as itself: a worker, a partner's
	// integration, a service of the same system.
	KindService Kind = "service"
)

// Principal is who a request, or a job, acts for. It travels in the
// context.Context, so services and repositories see the caller without each
// function taking it as an argument.
//
// It holds what authentication established and nothing an application has to
// look up again to trust: load roles, plans and per-resource permissions
// where they are checked, or add them per request with [Enrich].
type Principal struct {
	Kind Kind

	// Subject identifies the principal: a user ID, a client ID.
	Subject string

	// Tenant is the workspace or organization the principal acts in, when
	// the application has one and it was chosen for this request. Empty
	// otherwise.
	Tenant string

	// Scopes are what the principal may do, as the application names it
	// ("orders:read"). Checked by [Require] and [Principal.Has].
	Scopes []string

	// Method is how it authenticated: [MethodJWT], [MethodAPIKey], or a name
	// of the application's own.
	Method string
}

// How a principal authenticated.
const (
	MethodJWT    = "jwt"
	MethodAPIKey = "api_key"
)

// Has reports whether the principal has scope.
func (p Principal) Has(scope string) bool {
	return slices.Contains(p.Scopes, scope)
}

type principalKey struct{}

// WithPrincipal returns ctx carrying p. [Middleware] does it for requests;
// call it yourself where there is no request: a job, a command, a test.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal ctx carries, and false when it carries
// none: the request is anonymous, or the code runs outside a request.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := requestContext(ctx).Value(principalKey{}).(Principal)
	return p, ok
}

// requestContext returns the request's context when ctx is a handler's
// *medusa.Context or *gin.Context. Both satisfy context.Context but do not
// see the values of the request's own context, so a handler that passed its
// ctx straight in would otherwise find no principal.
func requestContext(ctx context.Context) context.Context {
	var request *http.Request
	switch c := ctx.(type) {
	case *medusa.Context:
		request = c.Request
	case *gin.Context:
		request = c.Request
	}
	if request == nil {
		return ctx
	}
	return request.Context()
}

// SubjectFrom returns the principal's subject as an S, for an application
// whose IDs are int64 or string. It is false when ctx carries no principal
// or the subject is not an S.
func SubjectFrom[S Subject](ctx context.Context) (S, bool) {
	p, ok := PrincipalFrom(ctx)
	if !ok {
		var zero S
		return zero, false
	}

	subject, err := parseSubject[S](p.Subject)
	return subject, err == nil
}

// Keys of a principal written as metadata.
const (
	metaKind    = "principal_kind"
	metaSubject = "principal_subject"
	metaTenant  = "principal_tenant"
)

// Metadata returns who the principal is as plain strings, to store with work
// that runs later: a job carries it, and its handler runs as the same
// subject and tenant. Scopes are left out on purpose. They say what a
// request was allowed to do when it was made; work that runs later decides
// again with the facts of that moment.
func (p Principal) Metadata() map[string]string {
	meta := map[string]string{metaKind: string(p.Kind), metaSubject: p.Subject}
	if p.Tenant != "" {
		meta[metaTenant] = p.Tenant
	}
	return meta
}

// PrincipalFromMetadata reads what [Principal.Metadata] wrote. It is false
// when meta holds no principal.
func PrincipalFromMetadata(meta map[string]string) (Principal, bool) {
	subject := meta[metaSubject]
	if subject == "" {
		return Principal{}, false
	}
	return Principal{Kind: Kind(meta[metaKind]), Subject: subject, Tenant: meta[metaTenant]}, true
}
