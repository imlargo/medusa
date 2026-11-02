# Medusa

Medusa is a set of small Go packages for building HTTP APIs on Gin. Each package does one job, takes standard library types, and can be used without the others. It is a toolkit, not a framework: it never owns `main`, holds no global state, and has no opinion on data access, configuration or identity.

```sh
go get github.com/imlargo/medusa
```

Requires Go 1.27. Medusa is pre-1.0, so minor versions may break the API; `v0.0.1` predates this layout.

## Packages

| Package | What it does | Depends on Gin |
|---|---|---|
| `medusa` | `Context` (extends `*gin.Context`), `Handle*` adapters for handlers that return `(result, error)`, response writers and envelopes | yes |
| `httperr` | The error model: status, stable code, client-safe message, details, and an internal cause that never reaches the client | no |
| `middleware` | Request ID, CORS, body limit, Prometheus metrics, rate limiting, API keys | yes |
| `health` | Readiness checks as plain functions, plus `/health` and `/ready` handlers | handlers only |
| `httpserver` | Serves any `http.Handler` with timeouts and graceful shutdown | no |
| `lifecycle` | Runs servers, start and stop hooks, and shuts down on SIGINT/SIGTERM | no |
| `job` | Periodic background work managed by `lifecycle`: no overlapping runs, panics contained, cancelled and awaited on shutdown | no |
| `metrics` | HTTP request counters and histograms in Prometheus | no |
| `storage` | S3-compatible object storage (Cloudflare R2), with key validation and bulk delete | no |

## Example

```go
type greeting struct {
    Name string `json:"name" binding:"required"`
}

func greet(ctx *medusa.Context, in *greeting) (string, error) {
    if in.Name == "admin" {
        return "", httperr.Forbidden("nice try")
    }
    return "hello " + in.Name, nil
}

func run(ctx context.Context) error {
    logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

    router := gin.New()
    router.Use(middleware.RequestID(), middleware.BodyLimit(1<<20))
    router.POST("/greet", medusa.Handle(greet))

    app := lifecycle.New(
        lifecycle.WithLogger(logger),
        lifecycle.WithServer(httpserver.New(router, logger, httpserver.WithPort(8080))),
    )
    return app.Run(ctx)
}
```

`medusa.Handle` binds and validates the body, calls the handler, and writes the response. A returned `*httperr.Error` sets the status. Any other error becomes a 500, and its cause goes to Gin's error list for the access log, never into the response.

## Repository layout

The repository holds two modules, tied together by a committed `go.work`:

- `/` is the Medusa module, `github.com/imlargo/medusa`.
- [`app/`](app/README.md) is an API built on it, `github.com/imlargo/medusa/app`. It is a working application and the starting point for new ones.

From the root, `go test ./... ./app/...` and `go vet ./... ./app/...` check both modules against this checkout. `./...` alone stops at the nested module.

Where new code goes, step-by-step recipes and the repo rules are in [AGENTS.md](AGENTS.md). Project status is in [context.md](context.md).

## License

MIT, see [LICENSE](LICENSE).
