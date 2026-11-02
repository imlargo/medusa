# Roadmap

Where Medusa is going and how the work is run. Status and context live in [context.md](../context.md); the decisions behind what is built are in [adr/](adr/).

The goal: a batteries-included Go framework on Gin whose batteries are opt-in (ADR 0002), at the quality of the research in `reports/`.

## Where it stands (2026-11-02)

Built on branch `refactor/separate-medusa-20261101`, not merged:

- Medusa separated from the app (ADR 0001); pgx + sqlc + goose instead of GORM.
- Core: `medusa`, `apperr`, `httperr`, `middleware`, `health`, `httpserver`, `lifecycle`, `job`, `metrics`, `storage`.
- Batteries: `postgres` (+ `pgtest`), `auth` (ADR 0002).
- Errors as declared `apperr` values, RFC 9457 responses, app by feature (ADR 0003).

Never run: the database tests, the end-to-end flows, the Docker build, `-race` beyond `job`. There is no CI.

## Phases

Each item has an ID for its issue, a size (S: under a day, M: days, L: a week or more) and what it depends on. Within a phase, items are in the order to take them.

### Phase 0: Ship what exists

Nothing else starts until this phase is done: every later item is verified by the CI built here.

| ID | Item | Size | Depends on |
|---|---|---|---|
| 0.1 | **CI pipeline** (GitHub Actions) for every module: `go vet`, `go test -race -shuffle=on`, `govulncheck`, and a Postgres service with `REQUIRE_DB_TESTS=1` so the database tests run. Plus `docker build -f app/Dockerfile .`. | M | |
| 0.2 | **golangci-lint v2** with a committed `.golangci.yml`, run in CI, and the code at zero findings. | M | 0.1 |
| 0.3 | **Dependabot alerts**: quic-go to >= 0.59.1, aws-sdk-go-v2 `service/s3` to >= 1.97.3 and `aws/protocol/eventstream` to >= 1.7.8 (3 moderate, DoS). | S | 0.1 |
| 0.4 | **Adoption check on a real database**: run `cmd/migrate` against a copy of a database GORM created, and the register, login, me and SSE flows end to end. | S | 0.1 |
| 0.5 | **PR and merge** of the branch, with the breaking changes listed for API clients: the error format (ADR 0003) and the token format (`sub`, `iss`; everyone signs in again once). | S | 0.1-0.4 |
| 0.6 | **First releases**: `CHANGELOG.md`, tag the core `v0.1.0`, then drop the `replace` in `auth/go.mod` and tag `postgres/v0.1.0` and `auth/v0.1.0`. | S | 0.5 |

### Phase 1: Security and contract gaps

Known holes in what already ships.

| ID | Item | Size | Depends on |
|---|---|---|---|
| 1.1 | **Access and refresh tokens are told apart.** Today a refresh token (7 days) works as an access token. Tokens get a kind (or audience) that `auth.Middleware` checks, plus a `POST /v1/auth/refresh` endpoint. Needs decision D1. | M | 0.6 |
| 1.2 | **Password policy** at registration, as a `user` rule with its declared error. Needs decision D2. | S | |
| 1.3 | **Server timeouts**: read, write and idle timeouts in `httpserver`. Long-lived routes (SSE) extend their own deadline with `http.ResponseController` instead of the server having no write timeout. | M | |
| 1.4 | **Error code catalog**: a test that finds every declared `apperr` error, fails on duplicate codes, and generates `docs/errors.md` (code, kind, status, message). It is the page the problem `type` can later point to. | M | |

### Phase 2: Batteries

The parts most APIs need, each an opt-in module (ADR 0002). Each one is extracted from the app or built with the app as its first consumer.

| ID | Item | Size | Depends on |
|---|---|---|---|
| 2.1 | **`config`**: env loading with a `.env` fallback, typed getters, one error listing every problem. Extracted from `app/internal/config`'s loader. | M | 0.6 |
| 2.2 | **`cache`**: Redis client from a URL, `Get`/`Set`/`Delete`, `Remember` with single flight, health check. The app's `database/redis.go` moves here. | M | 0.6 |
| 2.3 | **`httpclient`** (core, stdlib only): outbound client with a timeout and `ctx` always, capped response bodies, typed status errors as `apperr`, and opt-in retries only for idempotent requests. butter rebuilt this in about a dozen packages. | M | |
| 2.4 | **OpenAPI**: replace swaggo (Swagger 2.0 from comments that drift). Needs decision D3. Whichever wins becomes the `docs` battery. | L | D3 |

### Phase 3: Starter kit and observability

From "a set of packages" to "an app in 30 lines of `main`".

| ID | Item | Size | Depends on |
|---|---|---|---|
| 3.1 | **Kit**: `medusa.New(...)`, which wires the slog logger, the HTTP server, health, metrics, the access log and shutdown by default, with options to replace each. Needs decision D4. The app's `bootstrap` shrinks to its own features. | L | 2.1 |
| 3.2 | **Request-scoped logging**: a slog handler that adds `request_id` (and later `trace_id`) to every log line made with the request's context. | S | |
| 3.3 | **OpenTelemetry battery** (`otel`): tracing for Gin, pgx and `httpclient`, exemplars on the Prometheus metrics. | L | 2.3 |
| 3.4 | **Admin listener**: pprof, `/metrics` and the probes on a separate port, off the public router. | M | 3.1 |

### Phase 4: More than one replica

The state that lives in each process today (see the scaling notes in [architecture.md](architecture.md)).

| ID | Item | Size | Depends on |
|---|---|---|---|
| 4.1 | **Jobs that run once per cluster**: a `job` option backed by a Postgres advisory lock (in the `postgres` battery). | M | |
| 4.2 | **Shared rate limits**: the limiter's counters in Redis. | M | 2.2 |
| 4.3 | **SSE across replicas**: events published on one replica reach clients on every other, through Redis or Postgres `LISTEN/NOTIFY`. | L | 2.2 |
| 4.4 | **Readiness that does not take every replica out at once** on a database blip: separate "can serve" from "dependency degraded". | S | |

### Phase 5: API contract features

| ID | Item | Size | Depends on |
|---|---|---|---|
| 5.1 | **Cursor pagination**: opaque cursors next to the current page/offset `Pagination`. | M | |
| 5.2 | **`Idempotency-Key`** middleware for unsafe methods, storing results in Redis or Postgres. | L | 2.2 |
| 5.3 | **Transactional outbox** in the `postgres` battery: side effects (events, emails, webhooks) written in the same transaction and delivered by a `job`. | L | 4.1 |

### Phase 6: Tooling and adoption

| ID | Item | Size | Depends on |
|---|---|---|---|
| 6.1 | **CLI** `medusa new` (a project from the app template, module path replaced) and `medusa gen feature <name>` (the feature package files, migration, queries and route). | L | 3.1, battery APIs stable |
| 6.2 | **Docs site** for the core and the batteries, with the error catalog (1.4) and a getting-started guide built on the kit. | M | 3.1 |
| 6.3 | **`v1.0.0`** of the core: the exported API frozen under semver. | S | phases 0-3 used by a second app |

## Decisions needed

These are imlargo's to make. Each blocks the item next to it, and nothing else.

| ID | Decision | Blocks |
|---|---|---|
| D1 | Refresh tokens: rotation on every use? Revocation (a token store, or a per-user "valid after" timestamp)? Lifetime? | 1.1 |
| D2 | Password policy: minimum length, maximum, any other rule. | 1.2 |
| D3 | OpenAPI: Huma (code-first, OpenAPI 3.1, has a Gin adapter but changes the handler shape), oapi-codegen (spec-first), or swag v2 (keeps comments). | 2.4 |
| D4 | Where the kit lives: in the core module (`medusa.New`), or as its own module that depends on the batteries it wires. | 3.1 |
| D5 | Is the app deployed with real clients? It decides whether 0.5 needs a migration window for the error and token format changes. | 0.5 |

## How the work runs

- **One issue per item**, titled with its ID, holding the acceptance criteria. One milestone per phase. Decisions D1-D5 are issues too, assigned to imlargo.
- **One branch and one PR per item**, merged when CI is green. Until 0.1 exists, nothing merges.
- **Done means**:
  - tests next to the code;
  - CI green, including the database tests;
  - docs updated in the same PR (`AGENTS.md` for rules and recipes, `architecture.md` for runtime behavior, an ADR for a decision with alternatives);
  - a `CHANGELOG.md` entry for any module whose API changes;
  - `make docs` when routes change.
- **Batteries are built against a consumer**: each new battery or core package lands together with the app using it. No exported API without a caller.
- **Versioning**: semver per module. A breaking change to a released module bumps its minor version while it is `v0`, and is listed in the changelog with the migration.
- **Local checks stay light** on this machine: `go vet` and `go test` on the touched package, one at a time. The full matrix (`-race`, Postgres, Docker, govulncheck) runs in CI.
- **Order**: phase 0 first and alone. After that, phases 1 and 2 can run in parallel; 3 needs 2.1; 4 and 5 can follow in either order; 6 comes last.
