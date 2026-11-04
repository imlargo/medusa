// Package config resolves the application configuration from the process
// environment.
//
// Load reads every value once at startup, applies defaults for the optional
// ones and reports *all* problems it finds in a single error, so a misconfigured
// deployment fails immediately with a complete list instead of one variable at a
// time. The returned Config is read-only and safe to share across goroutines.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/imlargo/medusa/env"
	"github.com/imlargo/medusa/storage"
)

// Environment identifies the deployment the process is running in.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// IsProduction reports whether the app runs with production settings: release
// mode router, structured access logs and the stricter secret validation.
func (e Environment) IsProduction() bool { return e == EnvProduction }

// IsDevelopment reports whether the app runs with developer conveniences.
func (e Environment) IsDevelopment() bool { return e == EnvDevelopment }

// IsValid reports whether e is one of the known environments.
func (e Environment) IsValid() bool {
	switch e {
	case EnvDevelopment, EnvStaging, EnvProduction:
		return true
	default:
		return false
	}
}

// Config is the fully resolved application configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig

	Environment Environment
	Runtime     RuntimeConfig
	CORS        CORSConfig
	RateLimiter RateLimiterConfig
	Redis       RedisConfig
	Storage     StorageConfig
}

// ServerConfig is where the HTTP server listens.
type ServerConfig struct {
	Host string
	Port int
}

// DatabaseConfig points at the Postgres database.
type DatabaseConfig struct {
	URL string
}

// AuthConfig signs and times the access and refresh tokens.
type AuthConfig struct {
	JwtSecret         string
	JwtIssuer         string
	TokenExpiration   time.Duration
	RefreshExpiration time.Duration
}

// CORSConfig lists the origins allowed to call the API from a browser. The
// server's own host is always allowed and does not need to be repeated here.
type CORSConfig struct {
	AllowedOrigins []string
}

// RuntimeConfig holds settings that shape how the process serves and stops,
// rather than what it connects to.
type RuntimeConfig struct {
	// ShutdownTimeout is the budget for draining in-flight work on shutdown.
	ShutdownTimeout time.Duration

	// DrainDelay is how long /ready answers 503 before the shutdown starts, so
	// the load balancer stops routing here first. Zero, the default, skips the
	// wait; behind Kubernetes, a few probe periods (5s) is typical. It comes on
	// top of ShutdownTimeout.
	DrainDelay time.Duration

	// RequestTimeout is the deadline on every request's context, except event
	// streams.
	RequestTimeout time.Duration

	// MaxRequestBody caps an accepted request body, in bytes.
	MaxRequestBody int64

	// DocsEnabled serves the Swagger UI. It defaults to off in production, where
	// publishing the full API surface is a decision rather than a default.
	DocsEnabled bool
}

// Shutdown runs in two sequential phases — long-lived streams are ended first,
// then the HTTP server drains ordinary requests — so the two budgets below have
// to add up to ShutdownTimeout rather than each being it.

// DrainTimeout is the slice spent ending long-lived streams.
func (c RuntimeConfig) DrainTimeout() time.Duration {
	return c.ShutdownTimeout / 3
}

// ServerShutdownTimeout is what remains for in-flight requests.
func (c RuntimeConfig) ServerShutdownTimeout() time.Duration {
	return c.ShutdownTimeout - c.DrainTimeout()
}

// RateLimiterConfig caps how many requests a single caller may issue.
type RateLimiterConfig struct {
	Enabled              bool
	RequestsPerTimeFrame int
	TimeFrame            time.Duration

	// AuthRequestsPerMinute is the separate, much tighter limit on credential
	// endpoints. Brute force is a volume attack, and the general limit is sized
	// for a browser loading a page rather than for someone guessing passwords.
	AuthRequestsPerMinute int

	// TrustedProxies are the CIDR blocks of the reverse proxies, load balancers
	// or ingress controllers that sit directly in front of this server. Empty
	// means nothing does.
	//
	// This is a security setting, not a convenience. A caller can put anything
	// in X-Forwarded-For, so the only way to derive an address worth rate
	// limiting is to know which hops added to that header and can be believed.
	// Leave it empty and every forwarding header is ignored: the counter is
	// keyed by the address that actually opened the connection, which cannot be
	// forged.
	//
	// Get it wrong in the trusting direction and the rate limiter stops working
	// without saying so: a caller sets a fresh header on every request, gets a
	// fresh counter every time, and has no limit at all. Get it wrong in the
	// other direction, by leaving it empty behind a proxy, and every caller
	// shares one counter, which is loud and obvious.
	//
	// So the empty default is the safe one. Fill it in only for the proxies you
	// actually operate.
	TrustedProxies []string
}

// RedisConfig points at the Redis instance. Redis is optional: an empty URL
// disables it.
type RedisConfig struct {
	URL string
}

// Enabled reports whether Redis is configured.
func (c RedisConfig) Enabled() bool { return c.URL != "" }

// StorageConfig holds the object storage credentials and the provider to talk
// to. Storage is optional: an empty bucket name disables it.
type StorageConfig struct {
	storage.Config

	Provider storage.Provider
}

// Enabled reports whether object storage is configured.
func (c StorageConfig) Enabled() bool { return c.BucketName != "" }

// Load reads the configuration from the environment. A .env file in the working
// directory is loaded first when present; it never overrides variables already
// exported in the environment.
func Load() (*Config, error) {
	if err := env.LoadDotEnv(); err != nil {
		return nil, err
	}

	l := env.New()

	environment := Environment(strings.ToLower(l.String(envAppEnv, string(EnvDevelopment))))

	cfg := &Config{
		Server: ServerConfig{
			Host: l.String(envHost, defaultHost),
			Port: l.Int(envPort, defaultPort),
		},
		Database: DatabaseConfig{
			URL: l.Required(envDatabaseURL),
		},
		Auth: AuthConfig{
			JwtSecret:         l.Required(envJWTSecret),
			JwtIssuer:         l.String(envJWTIssuer, defaultJWTIssuer),
			TokenExpiration:   l.Duration(envJWTTokenExpiration, defaultTokenExpiration),
			RefreshExpiration: l.Duration(envJWTRefreshExpiration, defaultRefreshExpiration),
		},
		Environment: environment,
		Runtime: RuntimeConfig{
			ShutdownTimeout: l.Duration(envShutdownTimeout, defaultShutdownTimeout),
			DrainDelay:      l.Duration(envDrainDelay, 0),
			RequestTimeout:  l.Duration(envRequestTimeout, defaultRequestTimeout),
			MaxRequestBody:  int64(l.Int(envMaxRequestBody, defaultMaxRequestBody)),
			DocsEnabled:     l.Bool(envDocsEnabled, !environment.IsProduction()),
		},
		CORS: CORSConfig{
			AllowedOrigins: l.List(envCORSAllowedOrigins),
		},
		RateLimiter: RateLimiterConfig{
			Enabled:               l.Bool(envRateLimiterEnabled, defaultRateLimiterEnabled),
			RequestsPerTimeFrame:  l.Int(envRateLimiterRequests, defaultRateLimiterRequests),
			TimeFrame:             l.Duration(envRateLimiterTimeFrame, defaultRateLimiterTimeFrame),
			AuthRequestsPerMinute: l.Int(envRateLimiterAuthRequests, defaultRateLimiterAuthRequests),
			TrustedProxies:        l.List(envRateLimiterTrustedProxies),
		},
		Redis: RedisConfig{
			URL: l.String(envRedisURL, ""),
		},
		Storage: StorageConfig{
			Provider: storage.Provider(strings.ToLower(l.String(envStorageProvider, string(storage.ProviderR2)))),
			Config: storage.Config{
				BucketName:      l.String(envStorageBucketName, ""),
				AccountID:       l.String(envStorageAccountID, ""),
				AccessKeyID:     l.String(envStorageAccessKeyID, ""),
				SecretAccessKey: l.String(envStorageSecretAccessKey, ""),
				PublicDomain:    l.String(envStoragePublicDomain, ""),
				UsePublicURL:    l.Bool(envStorageUsePublicURL, false),
			},
		},
	}

	if err := errors.Join(l.Err(), cfg.Validate()); err != nil {
		return nil, fmt.Errorf("invalid configuration:\n%w", err)
	}

	return cfg, nil
}

// LoadDatabase reads only what migrations need, so the migrate step does not
// require the API's secrets (JWT_SECRET) to run.
func LoadDatabase() (DatabaseConfig, error) {
	if err := env.LoadDotEnv(); err != nil {
		return DatabaseConfig{}, err
	}

	l := env.New()
	db := DatabaseConfig{URL: l.Required(envDatabaseURL)}
	if err := l.Err(); err != nil {
		return DatabaseConfig{}, fmt.Errorf("invalid configuration:\n%w", err)
	}
	return db, nil
}

// Validate checks the invariants that survive parsing and returns every
// violation joined together.
func (c *Config) Validate() error {
	var errs []error

	if !c.Environment.IsValid() {
		errs = append(errs, fmt.Errorf("%s: unknown environment %q, want one of %q, %q, %q",
			envAppEnv, c.Environment, EnvDevelopment, EnvStaging, EnvProduction))
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("%s: %d is out of the valid port range 1-65535", envPort, c.Server.Port))
	}

	if c.RateLimiter.Enabled {
		if c.RateLimiter.RequestsPerTimeFrame < 1 {
			errs = append(errs, fmt.Errorf("%s: %d, want at least 1",
				envRateLimiterRequests, c.RateLimiter.RequestsPerTimeFrame))
		}
		if c.RateLimiter.TimeFrame <= 0 {
			errs = append(errs, fmt.Errorf("%s: %v, want a positive duration",
				envRateLimiterTimeFrame, c.RateLimiter.TimeFrame))
		}
		if c.RateLimiter.AuthRequestsPerMinute < 1 {
			errs = append(errs, fmt.Errorf("%s: %d, want at least 1",
				envRateLimiterAuthRequests, c.RateLimiter.AuthRequestsPerMinute))
		}
		// A malformed CIDR here would be dropped silently and the proxy it was
		// meant to describe would stop being trusted, which changes who gets
		// counted. Fail at startup instead.
		for _, cidr := range c.RateLimiter.TrustedProxies {
			if _, err := netip.ParsePrefix(cidr); err != nil {
				errs = append(errs, fmt.Errorf("%s: %q is not a CIDR block: %w",
					envRateLimiterTrustedProxies, cidr, err))
			}
		}
	}

	if c.Auth.TokenExpiration <= 0 {
		errs = append(errs, fmt.Errorf("%s: must be positive, got %s", envJWTTokenExpiration, c.Auth.TokenExpiration))
	}

	// A refresh token that expires before the access token it renews can never
	// be used, which surfaces as users being logged out at random.
	if c.Auth.RefreshExpiration <= c.Auth.TokenExpiration {
		errs = append(errs, fmt.Errorf("%s (%s) must be longer than %s (%s)",
			envJWTRefreshExpiration, c.Auth.RefreshExpiration, envJWTTokenExpiration, c.Auth.TokenExpiration))
	}

	if c.Environment.IsProduction() && len(c.Auth.JwtSecret) < minJWTSecretLength {
		errs = append(errs, fmt.Errorf("%s: must be at least %d characters in %s", envJWTSecret, minJWTSecretLength, EnvProduction))
	}

	if c.Runtime.ShutdownTimeout <= 0 {
		errs = append(errs, fmt.Errorf("%s: must be positive, got %s", envShutdownTimeout, c.Runtime.ShutdownTimeout))
	}

	if c.Runtime.DrainDelay < 0 {
		errs = append(errs, fmt.Errorf("%s: must not be negative, got %s", envDrainDelay, c.Runtime.DrainDelay))
	}

	if c.Runtime.RequestTimeout <= 0 {
		errs = append(errs, fmt.Errorf("%s: must be positive, got %s", envRequestTimeout, c.Runtime.RequestTimeout))
	}

	if c.Runtime.MaxRequestBody <= 0 {
		errs = append(errs, fmt.Errorf("%s: must be positive, got %d", envMaxRequestBody, c.Runtime.MaxRequestBody))
	}

	if c.RateLimiter.Enabled {
		if c.RateLimiter.RequestsPerTimeFrame <= 0 {
			errs = append(errs, fmt.Errorf("%s: must be positive when the rate limiter is enabled, got %d",
				envRateLimiterRequests, c.RateLimiter.RequestsPerTimeFrame))
		}
		if c.RateLimiter.TimeFrame <= 0 {
			errs = append(errs, fmt.Errorf("%s: must be positive when the rate limiter is enabled, got %s",
				envRateLimiterTimeFrame, c.RateLimiter.TimeFrame))
		}
	}

	errs = append(errs, c.Storage.validate())

	return errors.Join(errs...)
}

// validate reports partially configured storage. Storage is opt-in, but half a
// set of credentials is always a mistake and would otherwise fail on first use.
func (c StorageConfig) validate() error {
	credentials := map[string]string{
		envStorageAccountID:       c.AccountID,
		envStorageAccessKeyID:     c.AccessKeyID,
		envStorageSecretAccessKey: c.SecretAccessKey,
	}

	var missing []string
	for key, value := range credentials {
		if value == "" {
			missing = append(missing, key)
		}
	}

	// Nothing configured at all: storage stays disabled.
	if !c.Enabled() && len(missing) == len(credentials) {
		return nil
	}

	var errs []error

	if !c.Enabled() {
		errs = append(errs, fmt.Errorf("%s is required when storage credentials are set", envStorageBucketName))
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		errs = append(errs, fmt.Errorf("storage is configured but %s are missing", strings.Join(missing, ", ")))
	}
	if !c.Provider.IsValid() {
		errs = append(errs, fmt.Errorf("%s: unsupported provider %q", envStorageProvider, c.Provider))
	}

	return errors.Join(errs...)
}
