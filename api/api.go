// Package api is Medusa's API contract battery: one typed input per route
// drives binding, validation, the OpenAPI 3.1 document and the errors a
// route can answer with, so none of them can say something the code does
// not do. It is built on Huma and keeps Gin as the router.
//
//	docs := api.New(router, "Orders API", "1.0.0", api.WithBearerAuth())
//
//	v1 := docs.Group(router.Group("/v1"), api.Tags("orders"),
//	    api.Secured(api.BearerAuth, auth.Middleware(tokens)))
//
//	api.Get(v1, "/orders/{id}", h.Get, api.Errors(order.ErrNotFound))
//	api.Post(v1, "/orders", h.Create, api.Status(http.StatusCreated))
//
// A handler takes a context and its input, and returns the resource:
//
//	type GetInput struct {
//	    ID int64 `path:"id" minimum:"1"`
//	}
//
//	func (h *Handler) Get(ctx context.Context, in *GetInput) (*Order, error) {
//	    return h.orders.Get(ctx, in.ID) // an apperr error becomes its problem
//	}
//
// The input's fields say where each value comes from, with Huma's tags:
// path, query and header on fields, and a field named Body for the JSON
// body. Validation is written as tags too (minimum, maxLength, format,
// enum...) and runs before the handler. A JSON field the body's type does
// not have is refused.
//
// The resource is answered as is, with no envelope: the status code says how
// it went. An error is an RFC 9457 problem, the same one medusa.WriteError
// writes, whether it comes from validation or from the handler.
//
// Routes registered here and routes written with medusa.Handle live on the
// same Gin groups, behind the same middleware.
package api

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
)

// Names of the security schemes New can declare.
const (
	// BearerAuth is "Authorization: Bearer <token>".
	BearerAuth = "bearerAuth"

	// APIKeyAuth is the X-API-Key header.
	APIKeyAuth = "apiKeyAuth"
)

// Paths the documentation is served at when WithDocs is on.
const (
	DocsPath    = "/docs"
	OpenAPIPath = "/openapi"
)

// API is the contract of an application: its routes and the OpenAPI document
// that describes them.
type API struct {
	huma huma.API
}

// Option configures an API.
type Option func(*huma.Config)

// WithDescription sets the document's description.
func WithDescription(description string) Option {
	return func(c *huma.Config) { c.Info.Description = description }
}

// WithDocs serves the OpenAPI document at /openapi.json and /openapi.yaml
// and a page to browse it at /docs. Off by default: publishing every route
// an API has is a decision, usually taken outside production.
func WithDocs(enabled bool) Option {
	return func(c *huma.Config) {
		if enabled {
			c.DocsPath, c.OpenAPIPath = DocsPath, OpenAPIPath
		}
	}
}

// WithBearerAuth declares the [BearerAuth] scheme, for groups secured with
// a bearer token.
func WithBearerAuth() Option {
	return withSecurityScheme(BearerAuth, &huma.SecurityScheme{Type: "http", Scheme: "bearer"})
}

// WithAPIKeyAuth declares the [APIKeyAuth] scheme, for groups secured with
// an API key.
func WithAPIKeyAuth() Option {
	return withSecurityScheme(APIKeyAuth, &huma.SecurityScheme{Type: "apiKey", In: "header", Name: "X-API-Key"})
}

func withSecurityScheme(name string, scheme *huma.SecurityScheme) Option {
	return func(c *huma.Config) {
		if c.Components.SecuritySchemes == nil {
			c.Components.SecuritySchemes = map[string]*huma.SecurityScheme{}
		}
		c.Components.SecuritySchemes[name] = scheme
	}
}

// problemErrors makes Huma build Medusa's problems. It is process-wide,
// because Huma's error constructors are.
var problemErrors sync.Once

// New starts the contract of the application served by engine.
func New(engine *gin.Engine, title, version string, opts ...Option) *API {
	problemErrors.Do(func() {
		huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
			return newProblem(nil, status, msg, errs...)
		}
		huma.NewErrorWithContext = func(ctx huma.Context, status int, msg string, errs ...error) huma.StatusError {
			return newProblem(ctx, status, msg, errs...)
		}
	})

	config := huma.DefaultConfig(title, version)
	// Off unless WithDocs turns them on.
	config.DocsPath, config.OpenAPIPath = "", ""
	// Huma's defaults add a "$schema" member to every response and serve the
	// schemas for it. A response here is the resource and nothing else.
	config.SchemasPath = ""
	config.CreateHooks = nil
	for _, opt := range opts {
		opt(&config)
	}

	return &API{huma: huma.NewAPI(config, &adapter{engine: engine})}
}

// OpenAPI returns the document, to add what the options do not cover:
// servers, contact, external docs.
func (a *API) OpenAPI() *huma.OpenAPI {
	return a.huma.OpenAPI()
}

// Huma returns the Huma API underneath, for a feature of it this package
// does not wrap. A route registered through it goes on the engine itself,
// outside any group.
func (a *API) Huma() huma.API {
	return a.huma
}

// Group is a set of routes that share a Gin group, and with it a path
// prefix and middleware.
type Group struct {
	api      *API
	router   *gin.RouterGroup
	tags     []string
	security []string
	hidden   bool
}

// GroupOption configures a Group.
type GroupOption func(*Group)

// Tags lists every route of the group under these tags in the document.
// Use one per audience (web, agent, admin) to split a large API.
func Tags(tags ...string) GroupOption {
	return func(g *Group) { g.tags = append(g.tags, tags...) }
}

// Secured guards every route of the group with middleware, and says so in
// the document with scheme ([BearerAuth], [APIKeyAuth]). The guard and its
// documentation are one declaration, so neither can be forgotten:
//
//	api.Secured(api.BearerAuth, auth.Middleware(tokens))
func Secured(scheme string, middleware ...gin.HandlerFunc) GroupOption {
	return func(g *Group) {
		g.security = append(g.security, scheme)
		g.router = g.router.Group("", middleware...)
	}
}

// Internal leaves every route of the group out of the document. They work
// like any other.
func Internal() GroupOption {
	return func(g *Group) { g.hidden = true }
}

// Group puts routes on router. Its path prefix is part of every route's
// path in the document, and its middleware guards them.
func (a *API) Group(router *gin.RouterGroup, opts ...GroupOption) *Group {
	g := &Group{api: a, router: router}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Router returns the group's Gin group, with the middleware [Secured] added,
// for the routes a typed handler cannot serve: a stream, a websocket. List
// them in the document with [Document].
func (g *Group) Router() *gin.RouterGroup {
	return g.router
}

// route is where an operation is registered: on which Gin group, at which
// path relative to it, behind which handlers of its own.
type route struct {
	router gin.IRoutes
	path   string
	guards []gin.HandlerFunc
}

// routeKey is the key of an operation's route in its metadata.
const routeKey = "medusa.route"

type ginContextKey struct{}

// adapter registers Huma's operations on Gin. Unlike Huma's own Gin adapter
// it puts each operation on the Gin group it was declared for, so one
// document covers routes behind different middleware.
type adapter struct {
	engine *gin.Engine
}

func (a *adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.engine.ServeHTTP(w, r)
}

func (a *adapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	// Routes Huma adds itself, the document and its page, have no route in
	// their metadata and go on the engine.
	target := route{router: a.engine, path: op.Path}
	if declared, ok := op.Metadata[routeKey].(route); ok {
		target = declared
	}

	// {param} is :param in Gin.
	path := strings.NewReplacer("{", ":", "}", "").Replace(target.path)

	handlers := append(target.guards, func(c *gin.Context) {
		// The handler only gets a context.Context; problems still need the
		// Gin context, to record the error for the access log.
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ginContextKey{}, c))
		handler(humagin.NewContext(op, c))
	})
	target.router.Handle(op.Method, path, handlers...)
}

// ginFrom returns the Gin context of the request ctx belongs to, or nil.
func ginFrom(ctx context.Context) *gin.Context {
	c, _ := ctx.Value(ginContextKey{}).(*gin.Context)
	return c
}

// Request returns the HTTP request a handler is serving, for the few things
// an input cannot declare: every header of a signed webhook, the remote
// address. It is nil outside a route of this package.
func Request(ctx context.Context) *http.Request {
	if c := ginFrom(ctx); c != nil {
		return c.Request
	}
	return nil
}
