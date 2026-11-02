# Medusa app

An HTTP API built on [Medusa](../README.md): email and password accounts with JWT access and refresh tokens, a server-sent events stream per user, Postgres, and optional Redis and object storage. It runs as a real application and is the starting point for new ones.

## Stack

| Concern | Choice |
|---|---|
| HTTP | Gin, through Medusa's handler adapters and middleware |
| Database | Postgres via pgx v5 (`pgxpool`) |
| Queries | sqlc, generated from `internal/database/queries` into `internal/database/sqlc` |
| Migrations | goose, versioned SQL in `internal/database/migrations`, embedded in the binary |
| Logging | `log/slog`, JSON on stderr |
| Events | `github.com/imlargo/sse` |
| Rate limiting | `github.com/imlargo/ratelimit` |
| API docs | swag, generated into `api/docs`, served at `/docs` outside production |

## Running locally

```sh
cp .env.example .env    # set DATABASE_URL and JWT_SECRET (openssl rand -hex 32)
make migrate            # apply pending migrations
make run                # or `make dev` for hot reload with air
```

The API listens on `http://localhost:8000`. `/health` and `/ready` are the probes, `/metrics` is the Prometheus endpoint, and the routes live under `/v1`. `cmd/api/bootstrap/routes.go` lists all of them.

Every variable is documented in `.env.example`. The config is validated once at startup, and every problem is reported together.

## Migrations

Migrations run as their own step, never on API startup, so replicas cannot race each other through the same one:

```sh
make migrate                                                              # locally
docker run --rm --env-file .env --entrypoint /usr/local/bin/migrate <image>   # from the image
```

`00001_create_users.sql` reproduces the table GORM's `AutoMigrate` used to create, with `IF NOT EXISTS`. A database created by an earlier version adopts it unchanged, and goose records it as applied.

To change the schema, add the next numbered file to `internal/database/migrations`, then run `make sqlc` so the generated queries follow it.

## Tests

```sh
go test ./...                                                                   # unit tests; database tests are skipped
TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/postgres go test ./...   # also the database tests
```

Database tests use `dbtest.New(t)`. It creates a database per test on that server, applies the migrations and drops it at the end. In CI, set `REQUIRE_DB_TESTS=1` so a missing database fails the run instead of skipping it.

## Code generation

Generated code is committed and never edited by hand.

| Command | Regenerates | From |
|---|---|---|
| `make sqlc` | `internal/database/sqlc` | migrations and `internal/database/queries/*.sql` |
| `make docs` | `api/docs` | the swag annotations on the handlers |

Both run pinned images (`sqlc/sqlc:1.30.0`, `ghcr.io/swaggo/swag:v1.16.6`) and need Docker.

## Building the image

The build context is the repository root, because the app compiles against the Medusa module one directory up:

```sh
docker build -f app/Dockerfile .
```

## Starting a new project from this app

Copy `app/` into a new repository and change the module path in `go.mod` and in the `github.com/imlargo/medusa/app/...` imports. Then drop the `replace github.com/imlargo/medusa => ../` line and require a released Medusa version.
