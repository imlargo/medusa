package main

import (
	"context"

	"github.com/gin-gonic/gin"
	medusadocs "github.com/imlargo/medusa/pkg/medusa/core/docs"
	"github.com/imlargo/medusa/pkg/medusa/tools"

	"github.com/imlargo/medusa/internal/config"
	"github.com/imlargo/medusa/internal/database"
	"github.com/imlargo/medusa/internal/handlers"
	"github.com/imlargo/medusa/internal/services"
	"github.com/imlargo/medusa/internal/store"
	"github.com/imlargo/medusa/pkg/medusa/core/app"
	"github.com/imlargo/medusa/pkg/medusa/core/handler"
	"github.com/imlargo/medusa/pkg/medusa/core/jwt"
	"github.com/imlargo/medusa/pkg/medusa/core/logger"
	"github.com/imlargo/medusa/pkg/medusa/core/metrics"
	"github.com/imlargo/medusa/pkg/medusa/core/ratelimiter"
	"github.com/imlargo/medusa/pkg/medusa/core/repository"
	"github.com/imlargo/medusa/pkg/medusa/core/server/http"
	"github.com/imlargo/medusa/pkg/medusa/core/service"
	"github.com/imlargo/medusa/pkg/medusa/middleware"
	"github.com/imlargo/medusa/pkg/medusa/services/cache"
	"github.com/imlargo/medusa/pkg/medusa/services/storage"
)

// @title Meudsa API
// @version 1.0
// @description Template api

// @contact.name Default
// @contact.url https://default.dev
// @license.name MIT
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// @securityDefinitions.apiKey ApiKey
// @in header
// @name X-API-Key
func main() {
	cfg := config.LoadConfig()

	logger := logger.NewLogger()
	defer logger.Sync()

	router := gin.Default()
	srv := http.NewServer(
		router,
		logger,
		http.WithServerHost(cfg.Server.Host),
		http.WithServerPort(cfg.Server.Port),
	)

	app := app.NewApp(
		app.WithName("butter"),
		app.WithServer(srv),
	)

	Mount(app, &cfg, router, logger)

	println("...")
	println("App running on: ", tools.GetFullAppUrl(cfg.Server.Host, cfg.Server.Port))
	println("Docs running on: ", tools.GetFullDocsUrl(cfg.Server.Host, cfg.Server.Port))
	println("...")

	app.Run(context.Background())
}

func Mount(app *app.App, cfg *config.Config, router *gin.Engine, logger *logger.Logger) {

	// Docs
	medusadocs.RegisterDocs(router, cfg.Server.Host, cfg.Server.Port)

	// Rate Limiter
	if cfg.RateLimiter.Enabled {
		logger.Info("Rate Limiter is enabled")
	}
	rl := ratelimiter.NewTokenBucketLimiter(ratelimiter.Config{
		RequestsPerTimeFrame: cfg.RateLimiter.RequestsPerTimeFrame,
		TimeFrame:            cfg.RateLimiter.TimeFrame,
	})

	jwtAuth := jwt.NewJwt(jwt.Config{})

	// Database
	db, err := database.NewPostgresDatabase(cfg.Database.URL)
	if err != nil {
		logger.Fatal("Could not connect to the database: " + err.Error())
		return
	}

	// Storage
	_, err = storage.NewFileStorage(storage.StorageProviderR2, cfg.Storage)
	if err != nil {
		logger.Fatal("Could not initialize storage: " + err.Error())
		return
	}

	// Redis
	redisClient, err := database.NewRedisClient(cfg.Redis.Url)
	if err != nil {
		logger.Fatal("Could not connect to Redis: " + err.Error())
		return
	}

	// Cache
	_ = cache.NewRedisCache(redisClient)

	// Metrics
	metricsService := metrics.NewPrometheusMetrics()

	// Repositories
	medusaStore := repository.NewStore(db, logger)
	store := store.NewStore(medusaStore)

	baseService := service.NewService(logger)
	serviceContainer := services.NewService(baseService, store, cfg)
	userService := services.NewUserService(serviceContainer)
	authService := services.NewAuthService(serviceContainer, userService, jwtAuth)

	// Handlers
	baseHandler := handler.NewHandler(logger)

	healthHandler := handler.NewHealthHandler(baseHandler)
	authHandler := handlers.NewAuthHandler(baseHandler, authService)

	// Middlewares
	authTokenMiddleware := middleware.NewAuthTokenMiddleware(jwtAuth)
	rateLimiterMiddleware := middleware.NewRateLimiterMiddleware(rl)
	metricsMiddleware := middleware.NewMetricsMiddleware(metricsService)
	corsMiddleware := middleware.NewCorsMiddleware(cfg.Server.Host, []string{})

	// Handlers
	router.Use(corsMiddleware)
	router.GET("/health", healthHandler.Health)

	v1 := router.Group("/v1")

	{
		auth := v1.Group("/auth")
		{
			auth.POST("/login", authHandler.LoginWithPassword)
			auth.POST("/register", authHandler.Register)
			auth.GET("/user", authTokenMiddleware, authHandler.GetUser)
		}
	}

	v1.Use(authTokenMiddleware, metricsMiddleware)
	if cfg.RateLimiter.Enabled {
		v1.Use(rateLimiterMiddleware)
	}
}
