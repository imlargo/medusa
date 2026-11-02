package bootstrap

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/middleware"

	apidocs "github.com/imlargo/medusa/app/api/docs"
	"github.com/imlargo/medusa/app/internal/hosturl"
)

// apiGroups are the two authorization scopes every feature registers routes
// into. Both already carry the shared middleware stack.
type apiGroups struct {
	// public needs no credentials.
	public *gin.RouterGroup
	// protected requires a valid JWT access token.
	protected *gin.RouterGroup
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

	router.Use(gin.Recovery(), accessLogMiddleware(c.Logger))

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
// end-to-end latency of a request.
func registerRoutes(router *gin.Engine, c *Container) {
	router.Use(
		middleware.RequestID(),
		middleware.CORS(allowedOrigins(c)),
		middleware.BodyLimit(c.Config.Runtime.MaxRequestBody),
	)

	if c.Metrics != nil {
		router.Use(middleware.Metrics(c.Metrics))

		// Scraped from this container's own registry, not the global default.
		// It exposes internal request paths and latencies, so restrict it at the
		// ingress or network layer.
		router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(c.MetricsRegistry, promhttp.HandlerOpts{})))
	}

	// Off by default in production: publishing the whole API surface, including
	// endpoints not yet meant to be public, should be a decision.
	if c.Config.Runtime.DocsEnabled {
		registerDocs(router, apidocs.SwaggerInfo, c.Config.Server.Host, c.Config.Server.Port)
	}

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
		public:    v1.Group(""),
		protected: v1.Group("", auth.Middleware(c.JWT)),
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
	public := g.public.Group("/auth")
	public.POST("/login", medusa.Handle(c.Handlers.Account.Login))
	public.POST("/register", medusa.HandleCreate(c.Handlers.Account.Register))

	protected := g.protected.Group("/auth")
	protected.GET("/user", medusa.HandleGet(c.Handlers.Account.Me))
}

func registerEventRoutes(g apiGroups, c *Container) {
	// The stream authenticates itself: the sse Authorizer reads the token off
	// the raw request so it can both identify the caller and scope their topics,
	// which the gin middleware cannot do. Hence public here, not unauthenticated.
	g.public.GET("/events", c.Handlers.Events.Stream())

	g.protected.POST("/events/publish", medusa.HandleCreate(c.Handlers.Events.Publish))
}

// Template for adding a new feature:
//
//	func registerProductRoutes(g apiGroups, c *Container) {
//	    products := g.protected.Group("/products")
//	    products.GET("", medusa.HandleGet(c.Handlers.Product.List))
//	    products.GET("/:id", medusa.HandleGet(c.Handlers.Product.Get))
//	    products.POST("", medusa.HandleCreate(c.Handlers.Product.Create))
//	    products.PUT("/:id", medusa.HandleUpdate(c.Handlers.Product.Update))
//	    products.DELETE("/:id", medusa.HandleDelete(c.Handlers.Product.Delete))
//	}
