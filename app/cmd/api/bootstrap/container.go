package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/imlargo/ratelimit"
	ratelimitprom "github.com/imlargo/ratelimit/metrics/prometheus"
	"github.com/imlargo/sse"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/health"
	"github.com/imlargo/medusa/metrics"
	"github.com/imlargo/medusa/postgres"
	"github.com/imlargo/medusa/storage"

	"github.com/imlargo/medusa/app/internal/account"
	"github.com/imlargo/medusa/app/internal/config"
	"github.com/imlargo/medusa/app/internal/database"
	"github.com/imlargo/medusa/app/internal/events"
	"github.com/imlargo/medusa/app/internal/store"
)

const (
	// healthCheckTimeout bounds how long the readiness endpoint waits for all
	// dependency checks combined before reporting the app as unhealthy.
	healthCheckTimeout = 5 * time.Second

	// eventRetention is how long a published event stays replayable, which is
	// the window a reconnecting client can resume from without losing events.
	eventRetention = 5 * time.Minute
)

// Container holds every dependency of the application.
//
// Optional components are nil when the matching Options flag is off or the
// configuration for them is absent; always check before use.
type Container struct {
	Config *config.Config
	Logger *slog.Logger

	// Infrastructure, always present.
	DB    *pgxpool.Pool
	Store *store.Store
	JWT   *auth.JWT[int64]

	// Infrastructure, optional.
	RedisClient *redis.Client       // nil unless Redis is configured
	Storage     storage.FileStorage // nil unless object storage is configured
	Metrics     *metrics.HTTP       // nil unless metrics are enabled
	RateLimiter *ratelimit.Limiter  // nil unless rate limiting is enabled

	// Events publishes server-sent events. Always present: it is in-memory and
	// costs nothing until something subscribes.
	Events *sse.Broker

	// EventsLifecycle tracks open SSE sessions so they can be drained together
	// on shutdown. Without it they would be drained one connection at a time by
	// the HTTP server's timeout, which is to say not at all.
	EventsLifecycle *sse.Lifecycle

	HealthService *health.Service

	Services *Services
	Handlers *Handlers

	// MetricsRegistry is what the /metrics endpoint scrapes. nil when metrics
	// are disabled.
	MetricsRegistry *prometheus.Registry

	// closers releases resources in reverse order of acquisition.
	closers []func() error
}

// Services holds the application services, one per feature that has one.
type Services struct {
	Account *account.Service
}

// Handlers holds the HTTP handlers, one per feature.
type Handlers struct {
	Health  *health.Handler
	Account *account.Handler
	Events  *events.Handler
}

// Options selects which optional components to initialize. A component is only
// built when its flag is on and the corresponding configuration is present.
type Options struct {
	WithRedis   bool
	WithStorage bool
	WithMetrics bool
}

// DefaultOptions enables every optional component.
func DefaultOptions() Options {
	return Options{
		WithRedis:   true,
		WithStorage: true,
		WithMetrics: true,
	}
}

// MinimalOptions keeps only the database and JWT.
func MinimalOptions() Options {
	return Options{}
}

// newLogger is the process logger: JSON on stderr, info and above.
func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, nil))
}

// NewContainer wires all dependencies. On failure it closes whatever was
// already opened, so the caller never has to clean up after an error.
//
// ctx bounds the connectivity checks performed while dialing dependencies.
func NewContainer(ctx context.Context, cfg *config.Config, opts Options) (*Container, error) {
	c := &Container{
		Config: cfg,
		Logger: newLogger(),
	}

	if err := c.initInfrastructure(ctx, opts); err != nil {
		// The caller only gets the initialization error, so surface any
		// cleanup failure through the logger instead of swallowing it.
		if closeErr := c.Close(); closeErr != nil {
			c.Logger.Error("failed to release partially initialized resources", slog.Any("error", closeErr))
		}
		return nil, err
	}

	c.HealthService = c.buildHealthService()
	c.Services = c.buildServices()
	c.Handlers = c.buildHandlers()

	return c, nil
}

// initInfrastructure opens the external resources the app depends on.
func (c *Container) initInfrastructure(ctx context.Context, opts Options) error {
	cfg := c.Config

	db, err := postgres.Open(ctx, cfg.Database.URL, c.Logger,
		// Statement logging only outside production: it writes user data.
		postgres.WithQueryLogging(!cfg.Environment.IsProduction()),
	)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	c.DB = db
	c.onClose(func() error {
		db.Close()
		return nil
	})

	c.Store = store.New(db)

	jwtAuth, err := auth.NewJWT[int64](cfg.Auth.JwtSecret, auth.WithIssuer(cfg.Auth.JwtIssuer))
	if err != nil {
		return fmt.Errorf("initialize jwt: %w", err)
	}
	c.JWT = jwtAuth

	c.Events = sse.NewBroker("events", sse.NewMemoryLog(sse.Retention{For: eventRetention}))
	c.EventsLifecycle = sse.NewLifecycle()

	if opts.WithRedis && cfg.Redis.Enabled() {
		redisClient, err := database.NewRedisClient(ctx, cfg.Redis.URL)
		if err != nil {
			return fmt.Errorf("connect to redis: %w", err)
		}
		c.RedisClient = redisClient
		c.onClose(redisClient.Close)
		c.Logger.Info("redis initialized")
	}

	if opts.WithStorage && cfg.Storage.Enabled() {
		fileStorage, err := storage.NewFileStorage(ctx, cfg.Storage.Provider, cfg.Storage.Config)
		if err != nil {
			return fmt.Errorf("initialize storage: %w", err)
		}
		c.Storage = fileStorage
		c.Logger.Info("storage initialized",
			slog.String("provider", string(cfg.Storage.Provider)),
			slog.String("bucket", cfg.Storage.BucketName),
		)
	}

	if opts.WithMetrics {
		// A dedicated registry rather than the global default: two containers in
		// one process (which is every test that builds one) would otherwise
		// fail on duplicate collector registration.
		registry := prometheus.NewRegistry()
		httpMetrics, err := metrics.NewHTTP(registry)
		if err != nil {
			return fmt.Errorf("initialize metrics: %w", err)
		}
		c.Metrics = httpMetrics
		c.MetricsRegistry = registry
	}

	if cfg.RateLimiter.Enabled {
		rateLimiter, err := buildRateLimiter(cfg.RateLimiter, c.MetricsRegistry)
		if err != nil {
			return fmt.Errorf("initialize rate limiter: %w", err)
		}
		c.RateLimiter = rateLimiter
		c.onClose(rateLimiter.Close)
		c.Logger.Info("rate limiter enabled",
			slog.Int("requests", cfg.RateLimiter.RequestsPerTimeFrame),
			slog.Duration("per", cfg.RateLimiter.TimeFrame),
			slog.Int("auth_requests_per_minute", cfg.RateLimiter.AuthRequestsPerMinute),
			slog.String("keyed_by", rateLimiterKeyDescription(cfg.RateLimiter)),
			slog.Any("rules", rateLimiter.Rules()),
		)
	}

	return nil
}

// buildRateLimiter assembles the rule table.
//
// Two rules, because they answer different questions. The general one caps how
// much of the API any one caller may use. The auth one caps guesses at
// credentials, which is a volume attack and needs a much tighter bound than a
// browser loading a page does. Both apply to a request that matches both, and
// the tighter one governs.
//
// Everything the limiter needs to reject a request is decided here, at startup:
// a bad selector, an incoherent quota or a malformed proxy range fails now,
// with an error naming the problem, rather than on the first request.
func buildRateLimiter(cfg config.RateLimiterConfig, registry *prometheus.Registry) (*ratelimit.Limiter, error) {
	key := ratelimit.ByPeer()
	if len(cfg.TrustedProxies) > 0 {
		key = ratelimit.ByIP(cfg.TrustedProxies...)
	}

	rlCfg := ratelimit.Config{
		Rules: []ratelimit.Rule{
			{
				// Credential endpoints, by address. Not by account: an attacker
				// guessing passwords supplies a different account every time,
				// and would get a fresh counter for each.
				Name:     "auth",
				Selector: "POST /v1/auth/",
				Quota:    ratelimit.PerMinute(cfg.AuthRequestsPerMinute),
				Key:      key,
			},
			{
				Name:  "general",
				Quota: ratelimit.Per(cfg.RequestsPerTimeFrame, cfg.TimeFrame),
				Key:   key,
			},
		},
	}

	// Metrics matter more here than anywhere else in the application: this is
	// the one component whose job is to refuse requests, so "is it refusing the
	// wrong ones" has to be answerable. Compare denied against shadow_denied
	// before tightening a limit, and watch store_saturated, which is the only
	// way this component can refuse a caller for a reason unrelated to its
	// quota.
	if registry != nil {
		exporter, err := ratelimitprom.New(registry, ratelimitprom.Options{Namespace: "medusa"})
		if err != nil {
			return nil, fmt.Errorf("rate limiter metrics: %w", err)
		}
		rlCfg.Metrics = exporter.Metrics()
	}

	return ratelimit.NewWith(rlCfg)
}

// rateLimiterKeyDescription is for the startup log. Which address a limiter
// counts by is the thing most worth seeing in a log line, because getting it
// wrong is silent in one direction and obvious in the other.
func rateLimiterKeyDescription(cfg config.RateLimiterConfig) string {
	if len(cfg.TrustedProxies) == 0 {
		return "connection address (no trusted proxies declared, forwarding headers ignored)"
	}
	return fmt.Sprintf("client address behind trusted proxies %v", cfg.TrustedProxies)
}

// buildHealthService registers a readiness check per external dependency.
//
// Object storage is deliberately left out: the client validates connectivity at
// initialization and handles errors per request, so a periodic check would only
// add latency to the readiness probe.
func (c *Container) buildHealthService() *health.Service {
	svc := health.New(healthCheckTimeout)
	svc.Register("database", c.DB.Ping)

	if c.RedisClient != nil {
		svc.Register("redis", func(ctx context.Context) error {
			return c.RedisClient.Ping(ctx).Err()
		})
	}

	return svc
}

func (c *Container) buildServices() *Services {
	return &Services{
		Account: account.NewService(c.Store.Users, c.JWT, c.Config.Auth.TokenExpiration, c.Config.Auth.RefreshExpiration),
	}
}

func (c *Container) buildHandlers() *Handlers {
	return &Handlers{
		Health:  health.NewHandler(c.HealthService),
		Account: account.NewHandler(c.Services.Account),
		Events:  events.NewHandler(c.Events, c.EventsLifecycle, c.JWT),
	}
}

// DrainEventStreams ends every open SSE session.
//
// It has to run before the HTTP server shuts down, which is why it is wired as
// an onStop hook rather than a Close closer: an SSE response never completes on
// its own, and http.Server.Shutdown waits for in-flight requests. Without this,
// every deploy with a connected client would stall for the server's full
// shutdown timeout and then cut the streams anyway.
func (c *Container) DrainEventStreams(ctx context.Context) error {
	if c.EventsLifecycle == nil {
		return nil
	}

	open := c.EventsLifecycle.NodeSessionCount()
	if open == 0 {
		return nil
	}

	timeout := c.Config.Runtime.DrainTimeout()
	c.Logger.Info("draining event streams", slog.Int("open", open), slog.Duration("timeout", timeout))

	drainCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := c.EventsLifecycle.Shutdown(drainCtx); err != nil {
		return fmt.Errorf("drain event streams: %w", err)
	}

	return nil
}

// onClose registers a cleanup function to run on Close.
func (c *Container) onClose(fn func() error) {
	c.closers = append(c.closers, fn)
}

// Close releases every resource the container holds, in reverse order of
// acquisition, and returns all failures joined together. It is idempotent.
func (c *Container) Close() error {
	var errs []error

	for i := len(c.closers) - 1; i >= 0; i-- {
		errs = append(errs, c.closers[i]())
	}
	c.closers = nil

	return errors.Join(errs...)
}

// HasStorage reports whether object storage is available.
func (c *Container) HasStorage() bool { return c.Storage != nil }
