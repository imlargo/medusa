package main

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/imlargo/medusa/api/docs"
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
	"github.com/imlargo/medusa/pkg/medusa/core/responses"
	"github.com/imlargo/medusa/pkg/medusa/core/server/http"
	"github.com/imlargo/medusa/pkg/medusa/core/service"
	"github.com/imlargo/medusa/pkg/medusa/middleware"
	"github.com/imlargo/medusa/pkg/medusa/services/cache"
	"github.com/imlargo/medusa/pkg/medusa/services/storage"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

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

	app.Run(context.Background())
}

func Mount(app *app.App, cfg *config.Config, router *gin.Engine, logger *logger.Logger) {

	// Ping
	router.GET("/ping", func(c *gin.Context) {
		responses.SuccessOK(c, "hello")
	})

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

	// Handlers
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

func RegisterDocs(app *app.App, cfg *config.Config, router *gin.Engine, logger *logger.Logger) {

	host := cfg.Server.Host
	if IsLocalhostUrl(host) {
		host += ":" + strconv.Itoa(cfg.Server.Port)
	}

	if IsHttps(host) {
		docs.SwaggerInfo.Schemes = []string{"https"}
	} else {
		docs.SwaggerInfo.Schemes = []string{"http"}
	}

	docs.SwaggerInfo.Host = (host)
	docs.SwaggerInfo.BasePath = "/"

	schemaUrl := host
	schemaUrl += "/docs/doc.json"

	urlSwaggerJson := ginSwagger.URL(schemaUrl)
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, urlSwaggerJson))
}

func IsLocalhostUrl(host string) bool {
	return host == "localhost"
}

func IsHttps(host string) bool {
	return !IsLocalhostUrl(host)
}
