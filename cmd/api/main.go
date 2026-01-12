package main

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/imlargo/go-api/internal/config"
	"github.com/imlargo/go-api/internal/database"
	"github.com/imlargo/go-api/internal/handlers"
	"github.com/imlargo/go-api/internal/services"
	"github.com/imlargo/go-api/internal/store"
	"github.com/imlargo/go-api/pkg/medusa/core/app"
	"github.com/imlargo/go-api/pkg/medusa/core/handler"
	"github.com/imlargo/go-api/pkg/medusa/core/jwt"
	"github.com/imlargo/go-api/pkg/medusa/core/logger"
	"github.com/imlargo/go-api/pkg/medusa/core/ratelimiter"
	medusarepo "github.com/imlargo/go-api/pkg/medusa/core/repository"
	"github.com/imlargo/go-api/pkg/medusa/core/responses"
	"github.com/imlargo/go-api/pkg/medusa/core/server/http"
	medusaservice "github.com/imlargo/go-api/pkg/medusa/core/service"
	"github.com/imlargo/go-api/pkg/medusa/services/cache"
	"github.com/imlargo/go-api/pkg/medusa/services/storage"
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
	_ = ratelimiter.NewTokenBucketLimiter(ratelimiter.Config{
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

	// Repositories
	medusaStore := medusarepo.NewStore(db, logger)
	store := store.NewStore(medusaStore)

	baseService := medusaservice.NewService(logger)
	serviceContainer := services.NewService(baseService, store, cfg)
	userService := services.NewUserService(serviceContainer)
	authService := services.NewAuthService(serviceContainer, userService, jwtAuth)

	// Handlers
	baseHander := handler.NewHandler(logger)
	authHandler := handlers.NewAuthHandler(baseHander, authService)

	// Middlewares

	v1 := router.Group("/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/login", authHandler.LoginWithPassword)
			auth.POST("/register", authHandler.Register)
			auth.GET("/user", authHandler.GetUser)
		}
	}
}
