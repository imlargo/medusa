<div align="center">
  <h1>🪼 Medusa</h1>
  <p><strong>A batteries-included Go backend framework for building modern, scalable backends</strong></p>
  
  [![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
  [![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
  
  <p>
    <a href="#features">Features</a> •
    <a href="#quick-start">Quick Start</a> •
    <a href="#architecture">Architecture</a> •
    <a href="#documentation">Documentation</a> •
    <a href="#roadmap">Roadmap</a>
  </p>
</div>

---

## What is Medusa?

Medusa is a **production-ready framework** for Go that eliminates the tedious setup of modern backend applications. It's not just another HTTP router it's a complete platform that integrates everything you need to build enterprise-grade systems.

**Stop wasting time on boilerplate.** Start building features on day one. 

### Built for

- ✅ REST APIs with authentication, validation, and rate limiting
- ✅ Real-time systems with SSE, WebSockets, and pub/sub
- ✅ Microservices architectures with clean boundaries
- ✅ SaaS platforms with storage, email, and notifications
- ✅ Event-driven applications with message queues
- ✅ Scalable backends with observability and metrics

### Design Philosophy

- **🔋 Batteries Included, Decisions Optional** Everything ready out-of-the-box, but never imposed
- **🧩 Modular & Composable** Use what you need, ignore what you don't
- **🏗️ Clean Architecture** SOLID principles without unnecessary ceremony
- **🚀 Fast to Ship, Built to Scale** Rapid iteration without technical debt
- **���� Pragmatic, Not Dogmatic** Sensible conventions, always flexible

---

## Features

### Core Framework

- **🎯 Application Lifecycle** Graceful shutdown, signal handling, context propagation
- **🌐 HTTP Server** Built on Gin with extensible middleware and multiple server support
- **📝 Structured Logging** Production-ready logging with Zap
- **🗄️ Repository Pattern** Clean data layer abstractions with GORM
- **📊 Observability** Prometheus metrics, health checks, and monitoring
- **⚙️ Configuration** Environment-based config with validation

### Authentication & Security

- **🔐 JWT Authentication** Token generation, validation, and refresh tokens
- **🔑 API Key Auth** Header and Bearer token strategies
- **🛡️ CORS** Configurable cross-origin policies
- **⏱️ Rate Limiting** Token bucket algorithm with per-IP limiting
- **🔒 Middleware Chain** Extensible security pipeline

### Services (The Batteries)

- **💾 Cache** Redis-backed distributed caching with clean interface
- **📦 Storage** Multi-provider file storage (S3, Cloudflare R2) with presigned URLs
- **📧 Email** Transactional email via Resend
- **🔔 Push Notifications** Web Push API integration
- **📡 Server-Sent Events** Real-time server-to-client streaming with client management
- **🐰 PubSub** RabbitMQ message queue with publisher/subscriber pattern
- **🗃️ Database** PostgreSQL with GORM and automatic migrations

---

## Quick Start

### Prerequisites

- Go 1.21 or higher
- PostgreSQL 14+
- Redis 7+
- (Optional) RabbitMQ for message queues

### Installation

```bash
# Clone the repository
git clone https://github.com/imlargo/medusa. git
cd medusa

# Install dependencies
go mod download

# Copy environment configuration
cp .env.example .env
# Edit .env with your database and Redis credentials

# Run the application
go run cmd/api/main.go
```

The server will start at `http://localhost:8080`

### Your First Endpoint

```go
package main

import (
    "context"
    "github.com/gin-gonic/gin"
    "github.com/imlargo/go-api/pkg/medusa/core/app"
    "github.com/imlargo/go-api/pkg/medusa/core/logger"
    "github.com/imlargo/go-api/pkg/medusa/core/responses"
    "github.com/imlargo/go-api/pkg/medusa/core/server/http"
)

func main() {
    log := logger.NewLogger()
    defer log.Sync()

    router := gin.Default()
    srv := http.NewServer(router, log,
        http.WithServerHost("localhost"),
        http.WithServerPort("8080"),
    )

    app := app.NewApp(
        app. WithName("my-api"),
        app.WithServer(srv),
    )

    // Define your routes
    router.GET("/ping", func(c *gin.Context) {
        responses.SuccessOK(c, gin. H{"message": "pong"})
    })

    // Run with graceful shutdown
    app.Run(context.Background())
}
```

### Using Services

#### Cache with Redis

```go
import "github.com/imlargo/go-api/pkg/medusa/services/cache"

redisClient := database.NewRedisClient("redis://localhost:6379")
cache := cache.NewRedisCache(redisClient)

// Set value with expiration
cache.Set(ctx, "user:123", userData, 1*time.Hour)

// Get value
var user User
cache.Get(ctx, "user:123", &user)
```

#### File Storage (S3/R2)

```go
import "github.com/imlargo/go-api/pkg/medusa/services/storage"

storage, _ := storage.NewFileStorage(storage.StorageProviderR2, config)

// Upload file
file, _ := storage.Upload("avatars/user-123.jpg", fileReader, "image/jpeg", fileSize)
fmt. Println(file. Url) // Public URL

// Generate presigned URL for secure downloads
url, _ := storage.GetPresignedURL("documents/secret.pdf", 15*time.Minute)
```

#### Real-time with Server-Sent Events

```go
import "github.com/imlargo/go-api/pkg/medusa/services/sse"

sseManager := sse. NewSSEManager()

// Client connects
router.GET("/stream", func(c *gin.Context) {
    userID := getUserID(c)
    deviceID := c.Query("device_id")
    
    client, _ := sseManager.Subscribe(c. Request.Context(), userID, deviceID)
    
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    
    for {
        select {
        case msg := <-client. GetChannel():
            c.SSEvent("message", msg)
            c.Writer. Flush()
        case <-c.Request.Context().Done():
            return
        }
    }
})

// Send event to user (from anywhere in your app)
sseManager.Send(userID, &sse. Message{
    Event: "notification",
    Data:  gin.H{"title": "New message", "body": "You have a new message"},
})
```

#### Background Jobs with PubSub

```go
import "github.com/imlargo/go-api/pkg/medusa/services/pubsub"

// Publisher
publisher := pubsub.NewRabbitMQPublisher(config)
publisher.Publish(ctx, "user.registered", UserRegisteredEvent{
    UserID:  123,
    Email:  "user@example.com",
})

// Subscriber
subscriber := pubsub.NewRabbitMQSubscriber(config)
subscriber.Subscribe(ctx, "user.registered", func(msg []byte) error {
    var event UserRegisteredEvent
    json.Unmarshal(msg, &event)
    
    // Send welcome email
    sendWelcomeEmail(event. Email)
    return nil
})
```

#### Send Email

```go
import "github.com/imlargo/go-api/pkg/medusa/services/email"

emailService := email.NewResendClient(apiKey)
emailService.SendEmail(&email.SendEmailParams{
    From:    "noreply@myapp.com",
    To:      []string{"user@example.com"},
    Subject: "Welcome to MyApp",
    Html:    "<h1>Welcome! </h1><p>Thanks for joining. </p>",
})
```

---

## Architecture

Medusa follows **Clean Architecture** principles with a pragmatic twist structure without bureaucracy. 

```
.
├── cmd/                       # Application entry points
│   ├── api/                  # Main HTTP server
│   ├── sse/                  # Dedicated SSE server (optional)
│   └── worker/               # Background workers (future)
│
├── internal/                  # Private application code
│   ├── config/               # Configuration management
│   ├── database/             # Database connections
│   ├── handlers/             # HTTP handlers (controllers)
│   ├── models/               # Domain entities
│   ├── repository/           # Data access layer
│   ├── service/              # Business logic
│   └── store/                # Repository composition
│
└── pkg/medusa/               # 🪼 THE FRAMEWORK (reusable)
    ├── core/                 # Core components
    │   ├── app/             # Application lifecycle
    │   ├── env/             # Environment utilities
    │   ├── handler/         # Base handler
    │   ├── jwt/             # JWT auth
    │   ├── logger/          # Structured logging
    │   ├── metrics/         # Observability
    │   ├── ratelimiter/     # Rate limiting
    │   ├── repository/      # Repository pattern
    │   ├── responses/       # HTTP response helpers
    │   └── server/          # Server abstractions
    │
    ├── middleware/           # HTTP middleware
    │   ├── auth. go          # JWT authentication
    │   ├── api_key.go       # API key auth
    │   ├── cors.go          # CORS policies
    │   ├── metrics.go       # Metrics collection
    │   └── rate_limiter.go  # Rate limiting
    │
    ├── services/             # Infrastructure services
    │   ├── cache/           # Redis cache
    │   ├── email/           # Email service
    │   ├── notification/    # Push notifications
    │   ├── pubsub/          # Message queue
    │   ├── sse/             # Server-Sent Events
    │   └── storage/         # File storage
    │
    └── tools/                # Utilities
        ├── bind. go          # Data binding helpers
        └── url.go           # URL utilities
```

### Layer Principles

- **`cmd/`** Minimal entry point, bootstrap only
- **`internal/`** Your application-specific code
- **`pkg/medusa/`** Pure framework, no app dependencies
- **Golden Rule**:  `internal/` imports `pkg/medusa/`, never the reverse

### Modularity

Every component in `pkg/medusa` is independently usable:

```go
// Use only what you need
import "github.com/imlargo/go-api/pkg/medusa/services/cache"
import "github.com/imlargo/go-api/pkg/medusa/core/logger"

// No need to import the entire framework
```

---

## Examples

### Complete REST API with Auth

```go
func main() {
    log := logger.NewLogger()
    router := gin.Default()
    
    // Database
    db, _ := database.NewPostgresDatabase(os.Getenv("DATABASE_URL"))
    
    // JWT Auth
    jwtAuth := jwt.NewJwt(jwt.Config{Secret: os.Getenv("JWT_SECRET")})
    
    // Public routes
    router.POST("/auth/register", RegisterHandler)
    router.POST("/auth/login", LoginHandler)
    
    // Protected routes
    protected := router.Group("/api")
    protected.Use(middleware.AuthTokenMiddleware(jwtAuth))
    {
        protected.GET("/profile", GetProfileHandler)
        protected.PUT("/profile", UpdateProfileHandler)
    }
    
    // Rate limited routes
    limited := router.Group("/api/actions")
    limited.Use(middleware.NewRateLimiterMiddleware(rateLimiter))
    {
        limited.POST("/upload", UploadHandler)
    }
    
    srv := http.NewServer(router, log)
    app := app.NewApp(app.WithServer(srv))
    app.Run(context.Background())
}
```

### Multi-Server Application

```go
// Run HTTP and SSE servers concurrently
func main() {
    log := logger.NewLogger()
    
    // HTTP Server
    httpRouter := gin.Default()
    httpServer := http.NewServer(httpRouter, log,
        http.WithServerPort("8080"),
    )
    
    // SSE Server
    sseRouter := gin.Default()
    sseServer := http.NewServer(sseRouter, log,
        http.WithServerPort("8081"),
    )
    
    // Application with multiple servers
    app := app. NewApp(
        app.WithName("multi-server"),
        app.WithServer(httpServer, sseServer),
    )
    
    // Both servers managed with single graceful shutdown
    app.Run(context.Background())
}
```

### Repository Pattern

```go
// Define repository interface
type UserRepository interface {
    Create(ctx context.Context, user *User) error
    FindByID(ctx context.Context, id uint) (*User, error)
    Update(ctx context.Context, user *User) error
}

// Implementation
type userRepository struct {
    *medusarepo.Repository
}

func NewUserRepository(repo *medusarepo.Repository) UserRepository {
    return &userRepository{Repository: repo}
}

func (r *userRepository) Create(ctx context.Context, user *User) error {
    return r. DB.WithContext(ctx).Create(user).Error
}

func (r *userRepository) FindByID(ctx context.Context, id uint) (*User, error) {
    var user User
    err := r. DB.WithContext(ctx).First(&user, id).Error
    return &user, err
}
```

---

## Configuration

Medusa uses environment variables for configuration.  Create a `.env` file:

```bash
# Server
SERVER_HOST=localhost
SERVER_PORT=8080

# Database
DATABASE_URL=postgres://user:password@localhost:5432/dbname? sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379

# JWT
JWT_SECRET=your-secret-key-change-this
JWT_TOKEN_EXPIRATION=15m
JWT_REFRESH_EXPIRATION=168h

# Rate Limiting
RATE_LIMITER_ENABLED=true
RATE_LIMITER_REQUESTS_PER_TIME_FRAME=100
RATE_LIMITER_TIME_FRAME=60s

# Storage (S3/R2)
STORAGE_PROVIDER=r2
STORAGE_ACCOUNT_ID=your_account_id
STORAGE_ACCESS_KEY_ID=your_access_key
STORAGE_SECRET_ACCESS_KEY=your_secret_key
STORAGE_BUCKET_NAME=your_bucket
STORAGE_USE_PUBLIC_URL=true

# Email (Resend)
RESEND_API_KEY=re_your_api_key

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
```

---

## Middleware

### Available Middleware

```go
import "github.com/imlargo/go-api/pkg/medusa/middleware"

// JWT Authentication
router.Use(middleware.AuthTokenMiddleware(jwtAuth))

// API Key Authentication (Header)
router.Use(middleware.ApiKeyMiddleware("your-api-key"))

// API Key Authentication (Bearer)
router.Use(middleware.BearerApiKeyMiddleware("your-api-key"))

// CORS
router.Use(middleware.NewCorsMiddleware("https://myapp.com", []string{
    "https://app.myapp.com",
}))

// Rate Limiting
router.Use(middleware.NewRateLimiterMiddleware(rateLimiter))

// Metrics Collection
router.Use(middleware.NewMetricsMiddleware(metricsService))
```

### Creating Custom Middleware

```go
func LogRequestMiddleware(log *logger.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        c.Next()
        
        duration := time.Since(start)
        log.Info("Request completed",
            zap.String("method", c.Request.Method),
            zap.String("path", c.Request.URL. Path),
            zap.Int("status", c.Writer.Status()),
            zap.Duration("duration", duration),
        )
    }
}
```

---

## Development

### Hot Reload

Medusa includes Air for hot reload during development:

```bash
# Install Air (if not already installed)
go install github.com/cosmtrek/air@latest

# Run with hot reload
air
```

Configuration is in `.air.toml`.

### Available Commands

```bash
# Run the application
make run

# Run with hot reload
make dev

# Format code
make format

# Run tests
make test

# Build binary
make build

# Generate Swagger docs
make swag

# Clean build artifacts
make clean
```

---

## Testing

```go
package handlers_test

import (
    "testing"
    "net/http/httptest"
    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
)

func TestPingHandler(t *testing.T) {
    gin.SetMode(gin.TestMode)
    
    router := gin.Default()
    router.GET("/ping", PingHandler)
    
    req := httptest.NewRequest("GET", "/ping", nil)
    w := httptest.NewRecorder()
    
    router.ServeHTTP(w, req)
    
    assert.Equal(t, 200, w.Code)
    assert.Contains(t, w.Body.String(), "pong")
}
```

---

## Roadmap

### ✅ v0.1 - Foundation (Current)

- [x] Core framework (app, server, logger)
- [x] JWT & API key authentication
- [x] Repository pattern with GORM
- [x] Redis cache
- [x] File storage (S3/R2)
- [x] Server-Sent Events
- [x] PubSub with RabbitMQ
- [x] Email & push notifications
- [x] Rate limiting & CORS
- [x] Prometheus metrics

### 🚧 v0.2 - Developer Experience

- [ ] Comprehensive documentation
- [ ] Example applications
- [ ] Testing utilities
- [ ] Health checks & readiness probes
- [ ] Request ID middleware
- [ ] Docker Compose setup
- [ ] CI/CD examples

### 🎯 v0.3 - Validation & Documentation

- [ ] Automatic request validation
- [ ] Declarative validation tags
- [ ] OpenAPI 3.0 generation
- [ ] Swagger UI at `/docs`
- [ ] ReDoc integration
- [ ] Auto-generated examples

### 🎯 v0.4 - Type Safety & Ergonomics

- [ ] Enhanced `medusa.Context` with helpers
- [ ] Type-safe handlers with generics
- [ ] Dependency injection system
- [ ] Automatic pagination
- [ ] Query filter builders
- [ ] File upload helpers

### 🎯 v0.5 - CLI & Generators

- [ ] `medusa` CLI tool
- [ ] Project scaffolding
- [ ] Code generators (handlers, models, repos)
- [ ] Migration management
- [ ] Custom templates

### 🎯 v0.6 - Testing Framework

- [ ] Built-in testing utilities
- [ ] Test database helpers
- [ ] HTTP testing tools
- [ ] Mock generators
- [ ] Fixture factories

### 🎯 v0.7 - Background Processing

- [ ] Job queue system
- [ ] Scheduled tasks (cron)
- [ ] Worker pools
- [ ] Retry policies
- [ ] Job monitoring

### 🎯 v0.8 - Advanced Observability

- [ ] OpenTelemetry integration
- [ ] Distributed tracing
- [ ] APM metrics
- [ ] Error tracking
- [ ] Performance profiling

### 🎯 v0.9 - WebSockets

- [ ] Native WebSocket support
- [ ] Rooms & namespaces
- [ ] Broadcasting
- [ ] Presence detection

### 🎯 v1.0 - Production Ready

- [ ] Security audit
- [ ] Performance benchmarks
- [ ] Complete documentation
- [ ] Video tutorials
- [ ] Production case studies

---

## Why Medusa?

### vs. Minimalist Frameworks (Gin, Echo, Fiber)

**Gin/Echo/Fiber** are excellent routers, but you still need to: 
- Set up database connections
- Configure cache
- Implement auth
- Add file storage
- Set up observability
- Wire everything together

**Medusa** gives you all of this out-of-the-box, with clean interfaces and best practices baked in.

### vs. Full-Stack Frameworks (Buffalo, Beego)

**Buffalo/Beego** are comprehensive but: 
- Include frontend tooling you might not need
- More opinionated and harder to customize
- Heavier and less modular

**Medusa** focuses on backends only, stays modular, and never forces decisions. 

### vs. Starting from Scratch

**Building your own** means:
- Weeks of setup before writing features
- Reinventing patterns (repository, cache, storage)
- Maintenance burden for infrastructure code
- No community or shared knowledge

**Medusa** lets you ship on day one with production-ready patterns.

---

## Contributing

Contributions are welcome! Whether it's: 

- 🐛 Bug reports
- 💡 Feature requests  
- 📖 Documentation improvements
- 🔧 Code contributions

Please open an issue first to discuss what you'd like to change.

### Development Setup

```bash
# Fork and clone
git clone https://github.com/yourusername/medusa.git
cd medusa

# Install dependencies
go mod download

# Create a branch
git checkout -b feature/amazing-feature

# Make your changes and test
make test

# Commit and push
git commit -m "Add amazing feature"
git push origin feature/amazing-feature
```

Then open a Pull Request! 

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Community

- **GitHub Issues** Bug reports and feature requests
- **GitHub Discussions** Questions and community chat
- **Twitter** [@yourusername](https://twitter.com/yourusername) for updates

---

## Acknowledgments

Medusa stands on the shoulders of giants:

- [Gin](https://github.com/gin-gonic/gin) HTTP framework
- [GORM](https://gorm.io) ORM
- [Zap](https://github.com/uber-go/zap) Logging
- [Go Redis](https://github.com/redis/go-redis) Redis client
- [AWS SDK](https://github.com/aws/aws-sdk-go-v2) Cloud storage

---

<div align="center">
  <p>Built with ❤️ by <a href="https://github.com/imlargo">imlargo</a></p>
  <p>
    <a href="#quick-start">Get Started</a> •
    <a href="https://github.com/imlargo/medusa/issues">Report Bug</a> •
    <a href="https://github.com/imlargo/medusa/issues">Request Feature</a>
  </p>
</div>
