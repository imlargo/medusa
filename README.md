# Medusa

Medusa is a set of small Go packages for building HTTP APIs on Gin. The core does one job per package, takes standard library types, and can be used without the rest. Batteries for the parts most APIs need (Postgres, authentication) are separate modules: an app downloads only the ones it imports.

```sh
go get github.com/imlargo/medusa            # core
go get github.com/imlargo/medusa/postgres   # battery: pgx pool, goose migrations, test databases
go get github.com/imlargo/medusa/auth       # battery: JWT and its middleware
```

Requires Go 1.27. Medusa is pre-1.0, so minor versions may break the API; `v0.0.1` predates this layout.

## Core packages

| Package | What it does | Depends on Gin |
|---|---|---|
| `medusa` | `Context` (extends `*gin.Context`), `Handle*` adapters for handlers that return `(result, error)`, response writers | yes |
| `apperr` | Application errors independent of the transport: a kind, a stable `domain.rule` code, a client-safe message, invalid fields, and a cause that never reaches the client | no |
| `httperr` | Reports application errors over HTTP: kind to status, RFC 9457 problem details (`application/problem+json`) | no |
| `middleware` | Request ID, CORS, body limit, Prometheus metrics, rate limiting, API keys | yes |
| `health` | Readiness checks as plain functions, plus `/health` and `/ready` handlers | handlers only |
| `httpserver` | Serves any `http.Handler` with timeouts and graceful shutdown | no |
| `lifecycle` | Runs servers, start and stop hooks, and shuts down on SIGINT/SIGTERM | no |
| `job` | Periodic background work managed by `lifecycle`: no overlapping runs, panics contained, cancelled and awaited on shutdown | no |
| `metrics` | HTTP request counters and histograms in Prometheus | no |
| `storage` | S3-compatible object storage (Cloudflare R2), with key validation and bulk delete | no |

## Batteries

Each is its own module, with its own dependencies and versions. Choosing one is choosing its library, so its API uses that library's types.

| Module | What it does | Brings in |
|---|---|---|
| `postgres` | `Open` (pool, ping, slow query log), `Migrate` (goose over any `fs.FS`), `Conn` (pool or transaction), `IsNoRows`, `IsUniqueViolation` | pgx v5, goose |
| `postgres/pgtest` | `New(t, migrations)`: a fresh migrated database per test on `TEST_DATABASE_URL` | (same module) |
| `auth` | `JWT[S]` with a typed subject (`int64` or `string`) and optional issuer, `Middleware`, `SubjectFrom[S]` | golang-jwt v5 |

## Example

```go
var errNotAllowed = apperr.PermissionDenied("greeting.name_reserved", "that name is reserved")

type greeting struct {
    Name string `json:"name" binding:"required"`
}

func greet(ctx *medusa.Context, in *greeting) (string, error) {
    if in.Name == "admin" {
        return "", errNotAllowed
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

`medusa.Handle` binds and validates the body, calls the handler, and writes the response. A returned `*apperr.Error` sets the status through its kind and is sent as an RFC 9457 problem with its code. Any other error becomes a 500, and its cause goes to Gin's error list for the access log, never into the response.

## Repository layout

The repository holds several modules, tied together by a committed `go.work`:

- `/` is the core, `github.com/imlargo/medusa`.
- `postgres/` and `auth/` are the batteries, `github.com/imlargo/medusa/postgres` and `github.com/imlargo/medusa/auth`.
- [`app/`](app/README.md) is an API built on them, `github.com/imlargo/medusa/app`. It is a working application and the starting point for new ones.

From the root, `go test ./... ./auth/... ./postgres/... ./app/...` checks every module against this checkout. `./...` alone stops at the nested modules.

Where new code goes, step-by-step recipes and the repo rules are in [AGENTS.md](AGENTS.md). Project status is in [context.md](context.md).

## License

MIT, see [LICENSE](LICENSE).
