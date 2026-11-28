package bootstrap

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/imlargo/medusa/api"
	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/auth/password"
	"github.com/imlargo/medusa/middleware"

	"github.com/imlargo/medusa/app/internal/account"
	"github.com/imlargo/medusa/app/internal/events"
	"github.com/imlargo/medusa/app/internal/hosturl"
	"github.com/imlargo/medusa/app/internal/user"
)

// apiVersion is the version of the contract in the OpenAPI document.
const apiVersion = "1.0.0"

// apiGroups are the two authorization scopes every feature registers routes
// into. Both already carry the shared middleware stack, and every route
// registered on them is in the OpenAPI document.
type apiGroups struct {
	// public needs no credentials.
	public *api.Group
	// protected requires a valid JWT access token, and says so in the
	// document.
	protected *api.Group
}

// newRouter builds the Gin engine: router mode, global middleware and routes.
func newRouter(c *Container) *gin.Engine {
	gin.SetMode(ginMode(c))

	router := gin.New()

	// Gin trusts every proxy by default, which makes gin.ClientIP return the
	// leftmost X-Forwarded-For entry: a value the caller chose. Anything keyed
	// on it - access logs, metrics labels, an audit trail - is then attacker
	// controlled. Declaring the proxies fixes that; declaring none makes
	// ClientIP report the address that actually opened the connection, which is
	// the right answer when nothing sits in front of this server.
	//
	// The rate limiter does not rely on this: it derives its own address from
	// the same configuration and refuses to start if asked to read a forwarding
	// header without being told which hops to believe. This is for everything
	// else.
	if err := router.SetTrustedProxies(c.Config.RateLimiter.TrustedProxies); err != nil {
		// Unreachable: the CIDR blocks are validated when the configuration is
		// loaded. Log rather than ignore, so a future change cannot make this
		// silent.
		c.Logger.Error("invalid trusted proxy configuration", slog.Any("error", err))
	}

	router.Use(accessLogMiddleware(c.Logger))

	registerRoutes(router, c)

	return router
}

// ginMode maps the deployment environment onto Gin's own mode. Release mode
// disables Gin's debug output and startup warnings.
func ginMode(c *Container) string {
	if c.Config.Environment.IsProduction() {
		return gin.ReleaseMode
	}
	return gin.DebugMode
}

// registerRoutes installs the global middleware and every route of the API.
//
// Middleware order matters: the request ID comes first so everything downstream
// can log it, CORS runs before any middleware that may abort so preflight
// responses keep their headers, and metrics wrap the rest to measure the real
// end-to-end latency of a request. Recover comes last, so a panic still
// reaches the access log and the metrics as a 500.
func registerRoutes(router *gin.Engine, c *Container) {
	router.Use(
		middleware.RequestID(),
		middleware.SecureHeaders(),
		middleware.CORS(allowedOrigins(c)),
		middleware.BodyLimit(c.Config.Runtime.MaxRequestBody),
	)
	if c.Metrics != nil {
		router.Use(middleware.Metrics(c.Metrics))
	}
	router.Use(
		middleware.Recover(c.Logger),
		middleware.Timeout(c.Config.Runtime.RequestTimeout),
	)

	if c.Metrics != nil {
		// Scraped from this container's own registry, not the global default.
		// It exposes internal request paths and latencies, so restrict it at the
		// ingress or network layer.
		router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(c.MetricsRegistry, promhttp.HandlerOpts{})))
	}

	// The contract: every typed route below is in its OpenAPI document. The
	// document and its page are served only when docs are enabled, which is
	// off by default in production: publishing the whole API surface,
	// including routes not yet meant to be public, should be a decision.
	contract := api.New(router, "Medusa example API", apiVersion,
		api.WithDescription("Accounts, sessions and a per-user event stream, built on Medusa."),
		api.WithBearerAuth(),
		api.WithDocs(c.Config.Runtime.DocsEnabled),
	)

	// Probes sit outside the API group on purpose: no auth and no rate limit, so
	// orchestrators keep getting an answer even while the app sheds load.
	router.GET("/health", c.Handlers.Health.Live)
	router.GET("/ready", c.Handlers.Health.Ready)

	v1 := router.Group("/v1")

	// Rate limiting covers public endpoints too: unauthenticated login and
	// register calls are precisely what needs throttling.
	if c.RateLimiter != nil {
		v1.Use(middleware.RateLimit(c.RateLimiter))
	}

	groups := apiGroups{
		public: contract.Group(v1),
		// The middleware that guards the group and the scheme the document
		// shows for it are one declaration.
		protected: contract.Group(v1, api.Secured(api.BearerAuth, auth.Middleware(c.JWT))),
	}

	registerAccountRoutes(groups, c)
	registerEventRoutes(groups, c)
}

// allowedOrigins is the configured CORS origins plus the server's own URL,
// which is always allowed.
func allowedOrigins(c *Container) []string {
	origins := make([]string, 0, len(c.Config.CORS.AllowedOrigins)+1)
	origins = append(origins, c.Config.CORS.AllowedOrigins...)
	return append(origins, hosturl.Complete(c.Config.Server.Host))
}

func registerAccountRoutes(g apiGroups, c *Container) {
	h := c.Handlers.Account

	api.Post(g.public, "/auth/register", h.Register,
		api.Summary("Create an account"),
		api.Description("Creates an account and signs it in."),
		api.Status(http.StatusCreated),
		api.Errors(user.ErrEmailTaken, password.ErrTooShort, password.ErrTooLong),
	)
	api.Post(g.public, "/auth/login", h.Login,
		api.Summary("Sign in"),
		api.Errors(account.ErrInvalidCredentials),
	)
	// Public on purpose: the refresh token is the credential of both.
	api.Post(g.public, "/auth/refresh", h.Refresh,
		api.Summary("Refresh the session"),
		api.Description("Trades a refresh token for a new access token and a new refresh token. The old refresh token stops working; using it again ends the session."),
		api.Errors(auth.ErrInvalidRefreshToken),
	)
	api.Post(g.public, "/auth/logout", h.Logout,
		api.Summary("Sign out"),
		api.Description("Ends the session of a refresh token, on every device that holds a token of it. Signing out twice is not an error."),
	)

	api.Get(g.protected, "/auth/user", h.Me,
		api.Summary("The signed-in user"),
		api.Errors(auth.ErrInvalidToken, user.ErrNotFound),
	)
}

func registerEventRoutes(g apiGroups, c *Container) {
	h := c.Handlers.Events

	// The stream is a plain Gin handler: it never finishes, and it
	// authenticates itself, because the sse Authorizer reads the token off
	// the raw request to both identify the caller and scope their topics.
	// Hence public here, not unauthenticated. Document lists it.
	g.public.Router().GET("/events", h.Stream())
	api.Document(g.public, http.MethodGet, "/events",
		api.Summary("Stream of the signed-in user's events"),
		api.Description("Server-sent events. Send the access token as a Bearer header, or as the access_token query parameter where headers cannot be set."),
	)

	api.Post(g.protected, "/events/publish", h.Publish,
		api.Summary("Publish an event to your own stream"),
		api.Status(http.StatusCreated),
		api.Errors(auth.ErrInvalidToken, events.ErrPublishFailed),
	)
}

// Template for adding a new feature:
//
//	func registerProductRoutes(g apiGroups, c *Container) {
//	    h := c.Handlers.Product
//	    api.Get(g.protected, "/products", h.List)
//	    api.Get(g.protected, "/products/{id}", h.Get, api.Errors(product.ErrNotFound))
//	    api.Post(g.protected, "/products", h.Create, api.Status(http.StatusCreated))
//	    api.Patch(g.protected, "/products/{id}", h.Update, api.Errors(product.ErrNotFound))
//	    api.Delete(g.protected, "/products/{id}", h.Delete, api.Errors(product.ErrNotFound))
//	}
