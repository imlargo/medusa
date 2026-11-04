# Roadmap

Where Medusa is going and how the work is run. Status and context live in [context.md](../context.md); the decisions behind what is built are in [adr/](adr/).

The goal: a batteries-included Go framework on Gin whose batteries are opt-in (ADR 0002), so that the code most APIs need is written once, here, and never again in each app.

## Where it stands (2026-11-03)

Built on branch `refactor/separate-medusa-20261101`, not merged:

- Medusa separated from the app (ADR 0001); pgx + sqlc + goose instead of GORM.
- Core: `medusa`, `apperr`, `httperr`, `middleware`, `health`, `httpserver`, `lifecycle`, `job`, `metrics`, `storage`.
- Batteries: `postgres` (+ `pgtest`), `auth` (ADR 0002).
- Errors as declared `apperr` values, RFC 9457 responses, app by feature (ADR 0003).
- CI (`.github/workflows/ci.yml`) checks every module on its own: tidy, vet, `go test -race -shuffle=on` against a real Postgres with the database tests required, golangci-lint v2, govulncheck, and the Docker build.

Never run: the end-to-end flows against a real database (0.4).

### Is it a base for large APIs?

An independent review of the code (2026-11-03) found the core sound. The error model, the module boundaries, goroutine ownership, graceful shutdown and the database test setup are better than most in-house Go toolkits, and none of what is missing needs a redesign. It found ten bugs, fixed on the branch:

- panics escaped the access log, the metrics and the problem format;
- validation errors named Go fields instead of JSON fields;
- an unparsable query parameter was reported as a malformed body;
- `/ready` sent driver errors (hosts, users) to the public;
- `postgres.Open` overwrote pool settings given in the URL;
- passwords over 72 bytes answered 500;
- login timing revealed which emails exist;
- a signal at startup could hang shutdown;
- `Sort` accepted any column when no allow list was given;
- the transaction recipe led to an import cycle (documented in AGENTS.md).

What still stands between it and large production use, in order:

1. Access and refresh tokens are not told apart (1.1).
2. Only authentication exists: no scopes or roles, and the caller is not in `context.Context` for services (1.5).
3. Shutdown is not coordinated with readiness, so rolling deploys drop requests (1.6).
4. No per-request deadline (1.7).
5. No tracing, and only the access log carries the request ID (3.2, 3.3).
6. Per-process state: rate limits, SSE and jobs (phase 4).
7. Routes are documented by swag comments that drift (2.1).

## Phases

Each item has an ID for its issue, a size (S: under a day, M: days, L: a week or more) and what it depends on. Within a phase, items are in the order to take them.

### Phase 0: Ship what exists

| ID | Item | Size | Depends on | Status |
|---|---|---|---|---|
| 0.1 | **CI** for every module: tidy, vet, `-race -shuffle`, Postgres with `REQUIRE_DB_TESTS=1`, govulncheck, Docker build | M | | Done |
| 0.2 | **golangci-lint v2** with `.golangci.yml`, code at zero findings | M | 0.1 | Done |
| 0.3 | **Vulnerable dependencies**: quic-go, aws s3 and eventstream (Dependabot), x/net 0.60.0 and the Go 1.27.3 toolchain (govulncheck) | S | 0.1 | Done |
| 0.4 | **Adoption check on a real database**: `cmd/migrate` against a copy of a database GORM created, then register, login, me and SSE end to end | S | 0.1 | |
| 0.5 | **PR and merge**, with the breaking changes listed for API clients: the error format (ADR 0003) and the token format (`sub`, `iss`; everyone signs in again once). Needs D5. | S | 0.4 | |
| 0.6 | **First releases**: `CHANGELOG.md`, tag the core `v0.1.0`, then drop the `replace` in `auth/go.mod` and tag `postgres/v0.1.0` and `auth/v0.1.0` | S | 0.5 | |

### Phase 1: Security and contract gaps

| ID | Item | Size | Depends on | Status |
|---|---|---|---|---|
| 1.1 | **Access and refresh tokens told apart**: a token kind that `auth.Middleware` checks, and `POST /v1/auth/refresh`. Part of the auth battery (2.3). Needs D1. | M | 0.6 | |
| 1.2 | **Password policy** at registration, as a rule with its declared error. Needs D2. | S | | |
| 1.3 | **Server timeouts**: read, write and idle defaults in `httpserver`; long routes extend their own deadline | M | | Done |
| 1.4 | **Error code catalog**: a test that finds every declared `apperr` error, fails on duplicate codes, and generates `app/api/errors.md`. It later feeds the error responses in the OpenAPI spec (2.1). | M | | Done (`apperr.Declared`, `TestErrorCatalog`) |
| 1.5 | **The caller in `context.Context`**: `auth` puts a `Principal` (subject, scopes, method) in the request context, so services and jobs see who acts, and `auth.Require(scopes...)` guards routes. Casbin, OpenFGA or Cerbos stay recipes, not batteries. | M | | |
| 1.6 | **Shutdown that drains**: on SIGTERM `/ready` answers 503 and the server waits a configurable delay before closing the listener, so the load balancer stops sending first. | S | | Done (`SHUTDOWN_DRAIN_DELAY`) |
| 1.7 | **Request deadline** middleware: every request's context gets a deadline, and an expired one is `server.timeout` (503). | S | | Done (`REQUEST_TIMEOUT`) |
| 1.8 | **Headers**: CORS allows `X-Request-ID` and `X-API-Key` and exposes `X-Request-ID`, `Retry-After` and the rate-limit headers; `X-Content-Type-Options: nosniff` on every response; `http.CrossOriginProtection` when auth uses cookies. | S | | Done but `CrossOriginProtection`, which waits for cookie sessions (2.3) |

### Phase 2: Batteries

Each one is an opt-in module (ADR 0002), built with the app as its first consumer. The ecosystem research (2026-11-03) shaped this phase: take the ideas of FastAPI and Huma, build on the libraries the industry already maintains, and write ourselves only what is small.

| ID | Item | Size | Depends on |
|---|---|---|---|
| 2.1 | **`api` battery: typed operations, FastAPI style.** One input struct (path, query, header and body tags) drives binding, validation, OpenAPI 3.1 served at startup, security schemes and the error responses. Built on Huma through its Gin adapter, so Gin stays the router and routes move one at a time; `apperr` renders through Huma's error hook. Replaces swag and `make docs`. Needs D3. | L | 1.4 |
| 2.2 | **`config`**: env loading with a `.env` fallback, typed getters, one error listing every problem. Extracted from `app/internal/config`. `cmd/migrate` stops needing `JWT_SECRET`. | M | |
| 2.3 | **Auth battery v1**, split so heavy dependencies stay opt-in. Needs D1, D2, D6. `auth/oidc` is its own module. **Never built:** an authorization server or OIDC provider, SAML, SCIM, our own crypto, or a user admin UI. Apps that need SSO or share one login across apps use an IdP (Zitadel, Ory, Keycloak) and only the verifier. | L | 1.5 |
| 2.4 | **`queue` battery on River**: Postgres jobs enqueued in the same transaction as the write that causes them, with retries and a UI. Replaces the custom outbox and covers cluster-once jobs. Needs D7. | M | |
| 2.5 | **`cache`**: Redis client from a URL, `Get`/`Set`/`Delete`, `Remember` with single flight, health check. The app's `database/redis.go` moves here. | M | |
| 2.6 | **`httpclient`** (core, standard library only): outbound client with a timeout and `ctx` always, capped bodies, status errors as `apperr`, retries only for idempotent requests. | M | |
| 2.7 | **`storage` out of the core** into its own battery, so an API without object storage stops downloading the AWS SDK. Breaking for the core. | S | |
| 2.8 | **Test kit**: `pgtest` starts a Postgres container (testcontainers-go) when `TEST_DATABASE_URL` is empty, and clones a migrated template database instead of migrating per test. Migrations take goose's session lock. | M | |
| 2.9 | **`webhook`**: signed delivery out and verification in, on the Standard Webhooks spec, delivered as `queue` jobs. | M | 2.4 |
| 2.10 | **`mail`**: a `Sender` interface, SMTP (wneessen/go-mail) and a development sender that logs, sent as `queue` jobs. | S | 2.4 |

The auth battery packages for 2.3:

| Package | Holds |
|---|---|
| `auth` | `Principal`, `Authenticator` (bearer, API key, session tried in order), `Middleware`, `Require`, `SubjectFrom` |
| `auth/token` | Access tokens with a kind and `kid`; EdDSA or ES256 keys plus `/.well-known/jwks.json` (HS256 stays an option); a JWKS verifier for tokens from an IdP; opaque refresh tokens, stored hashed, rotated on use with reuse detection, behind a `RefreshStore` interface |
| `auth/password` | argon2id with the OWASP parameters, stored as a PHC string, `NeedsRehash`, the policy from D2 |
| `auth/oidc` | Login with go-oidc and x/oauth2, PKCE S256, state and nonce. Providers: any OIDC issuer, Google, Microsoft, and GitHub over OAuth2. Returns a verified `Identity`; accounts are never linked on an unverified email |

Later versions add passkeys (go-webauthn), TOTP, magic links and email verification (needs `mail`), and cookie sessions.

Because a battery never imports another, `auth` cannot ship its Postgres store. It defines the store interfaces, and `medusa gen auth` (6.1) writes the migrations, queries and store into the app.

### Phase 3: Starter kit and observability

From "a set of packages" to "an app in 30 lines of `main`".

| ID | Item | Size | Depends on |
|---|---|---|---|
| 3.1 | **Kit**: `medusa.New(...)` wires the slog logger, the HTTP server, health, metrics, the access log, recovery and shutdown by default, with options to replace each. Needs D4. | L | 2.2 |
| 3.2 | **Request-scoped logging**: a slog handler that adds `request_id` (and `trace_id`) to every line logged with the request's context. Done for `request_id` (`medusa.LogHandler`); `trace_id` comes with 3.3 | S | |
| 3.3 | **`otel` battery**: tracing for Gin, pgx, River and `httpclient`; exemplars on the Prometheus metrics | L | 2.6 |
| 3.4 | **Admin listener**: pprof, `/metrics` and the probes on a separate port | M | 3.1 |

### Phase 4: More than one replica

| ID | Item | Size | Depends on |
|---|---|---|---|
| 4.1 | **Jobs that run once per cluster**: periodic jobs through River (2.4), or a `job` option backed by a Postgres advisory lock | M | 2.4 |
| 4.2 | **Shared rate limits**: the limiter's counters in Redis | M | 2.5 |
| 4.3 | **SSE across replicas**: events published on one replica reach clients on every other, through Redis or Postgres `LISTEN/NOTIFY` | L | 2.5 |
| 4.4 | **Readiness that does not take every replica out at once** on a database blip | S | |

### Phase 5: API contract features

| ID | Item | Size | Depends on |
|---|---|---|---|
| 5.1 | **Cursor pagination**: opaque cursors next to page/offset | M | |
| 5.2 | **`Idempotency-Key`** for unsafe methods, results stored in Postgres first | L | |

### Phase 6: Tooling and adoption

| ID | Item | Size | Depends on |
|---|---|---|---|
| 6.1 | **CLI**: `medusa new` (a project from the app template), `medusa gen feature <name>` (feature files, migration, queries, route) and `medusa gen auth` (the auth store) | L | 3.1, battery APIs stable |
| 6.2 | **Docs site** for the core and the batteries, with the error catalog and a getting-started guide on the kit | M | 3.1 |
| 6.3 | **`v1.0.0`** of the core: the exported API frozen under semver | S | phases 0-3 used by a second app |

### What Medusa does not copy

| From | What | Why not |
|---|---|---|
| Encore | Infrastructure declared through its compiler | You cannot build without their toolchain |
| go-zero | `.api` DSL and generated code | A second language that drifts from the Go code |
| Goravel | Facades and a service container | Global state that hides dependencies |
| Fiber | fasthttp | Leaves the net/http ecosystem |
| Kratos | Protobuf first | protoc for a REST-only API |
| GoFrame | One module with everything | The opposite of opt-in batteries |
| Wire, fx | Dependency injection | Wire is archived; fx hides the wiring. `container.go` stays explicit |
| FastAPI | In-process background tasks | Work is lost on a crash; use `queue` or `job` |
| | i18n, admin panels, multi-tenancy as batteries | Error codes already work as translation keys; the rest are recipes until two apps need the same code |

## Decisions needed

These are imlargo's to make. Each blocks the item next to it, and nothing else.

| ID | Decision | Recommendation | Blocks |
|---|---|---|---|
| D1 | Refresh tokens: format, rotation, revocation, lifetime | Opaque, stored hashed, rotated on every use, the whole family revoked when an old one comes back | 1.1, 2.3 |
| D2 | Password policy | NIST SP 800-63B-4 (check its SHALL/SHOULD wording first): a long minimum, a maximum of at least 64, no composition rules, a blocklist of breached passwords | 1.2, 2.3 |
| D3 | OpenAPI: Huma, oapi-codegen or swag v2 | Huma through `humagin`, behind the `api` battery | 2.1 |
| D4 | Where the kit lives: the core module, or its own module | | 3.1 |
| D5 | Is the app deployed with real clients? It decides whether 0.5 needs a migration window | | 0.5 |
| D6 | Auth scope: which logins Medusa ships (password, OIDC providers, passkeys) and when an app must use an external IdP instead | Password and OIDC in v1, passkeys next; IdP for SSO and shared logins | 2.3 |
| D7 | River as a dependency for the `queue` battery (MPL-2.0, Postgres only) | Yes | 2.4 |

## How the work runs

- **One issue per item**, titled with its ID, holding the acceptance criteria. One milestone per phase. Decisions are issues too, assigned to imlargo.
- **One branch and one PR per item**, merged when CI is green.
- **Done means**:
  - tests next to the code;
  - CI green;
  - docs updated in the same PR (`AGENTS.md` for rules and recipes, `architecture.md` for runtime behavior, an ADR for a decision with alternatives);
  - a `CHANGELOG.md` entry for any module whose API changes.
- **Batteries are built against a consumer**: each lands together with the app using it. No exported API without a caller.
- **Versioning**: semver per module. While a module is `v0`, a breaking change bumps its minor version and the changelog says how to migrate.
- **Local checks stay light** on this machine: `go vet` and `go test` on the touched package, one at a time. The full matrix runs in CI.
- **Order**: phase 0 first. Then phase 1 and phase 2 in parallel; 3 needs 2.2; 4 and 5 in either order; 6 last.
