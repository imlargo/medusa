# Roadmap

Where Medusa is going and how the work is run. Status and context live in [context.md](../context.md); the decisions behind what is built are in [adr/](adr/).

The goal: a batteries-included Go framework on Gin whose batteries are opt-in (ADR 0002), so that the code most APIs need is written once, here, and never again in each app.

## Where it stands (2026-11-27)

On `main` and released: the core, `env`, `postgres` and `storage` at `v0.1.0`, `auth` at `v0.1.1`.

- Medusa separated from the app (ADR 0001); pgx + sqlc + goose instead of GORM.
- Core: `medusa`, `apperr`, `httperr`, `middleware`, `health`, `httpserver`, `lifecycle`, `job`, `metrics`. The core no longer pulls the AWS SDK or godotenv.
- Batteries: `auth`, `env`, `postgres` (+ `pgtest`), `storage` (ADR 0002).
- Errors as declared `apperr` values, RFC 9457 responses, app by feature (ADR 0003).
- CI (`.github/workflows/ci.yml`) checks every module on its own: tidy, vet, `go test -race -shuffle=on` against a real Postgres with the database tests required, golangci-lint v2, govulncheck, and the Docker build.

### Is it a base for large APIs?

An independent review of the code (2026-11-03) found the core sound. The error model, the module boundaries, goroutine ownership, graceful shutdown and the database test setup are better than most in-house Go toolkits, and none of what is missing needs a redesign. It found ten bugs, all fixed:

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
3. No tracing (3.3). Request IDs already reach every log line (3.2).
4. Per-process state: rate limits, SSE and jobs (phase 4).
5. Routes are documented by swag comments that drift (2.1).

Closed since the review: shutdown coordinated with readiness (1.6), a deadline on every request (1.7), secure headers (1.8), the error catalog (1.4), and `storage` and `env` as batteries (2.7, 2.2).

## First consumer: Butter's backend

Butter's backend is being designed on Medusa as three deployables: `api` (HTTP, phone-agent websockets, SSE), `worker` (queued and scheduled jobs, event handlers) and `engine` (a media service with its own database that takes a resolved spec over HTTP and returns artifacts through presigned URLs and a signed callback). An audit of Medusa against that design (2026-11-27) found twelve gaps. They set the order of the work, ahead of the phase order below:

| Order | Gap | Items | Issue |
|---|---|---|---|
| a | A job queue with a `worker` entrypoint | 2.4, 4.1 | #22 |
| a | Presigned uploads, streams, head, copy and list in `storage` | 2.11 | #23 |
| a | The caller in `context.Context`, scopes, access and refresh tokens | 1.1, 1.5, 2.3 | #24 |
| a | An outbound HTTP client | 2.6 | #25 |
| b | Service-to-service auth: keys per client, signed callbacks | 2.12 | #26 |
| b | Inbound webhooks verified and stored before processing | 2.9 | #27 |
| b | Observability beyond HTTP | 3.3, 3.5 | #28 |
| c | Events through a transactional outbox | 2.13 | #29 |
| c | More than one replica: `redis`, rate limits, SSE, websockets | 2.5, 4.2, 4.3, 4.5 | #30 |
| c | Module boundaries at scale | 2.14 | #31 |
| c | The API contract: typed input, idempotency, cursors, docs | 2.1, 5.1, 5.2, 5.3 | #32 |
| c | Feature flags and mail | 2.15, 2.10 | #33 |

Order a is what nothing can be built without. Order b is built with the engine as first consumer: it is small, has its own database and needs no user auth. Order c is built with Butter's `api` and `worker`.

## Phases

Each item has an ID for its issue, a size (S: under a day, M: days, L: a week or more) and what it depends on. Within a phase, items are in the order to take them.

### Phase 0: Ship what exists

| ID | Item | Size | Depends on | Status |
|---|---|---|---|---|
| 0.1 | **CI** for every module: tidy, vet, `-race -shuffle`, Postgres with `REQUIRE_DB_TESTS=1`, govulncheck, Docker build | M | | Done |
| 0.2 | **golangci-lint v2** with `.golangci.yml`, code at zero findings | M | 0.1 | Done |
| 0.3 | **Vulnerable dependencies**: quic-go, aws s3 and eventstream (Dependabot), x/net 0.60.0 and the Go 1.27.3 toolchain (govulncheck) | S | 0.1 | Done |
| 0.4 | **End to end on a real database**: register, login and the signed-in user through the router and middleware, against Postgres in CI (`TestAccountFlow`). There is no deployed database to check, so the GORM copy is moot | S | 0.1 | Done |
| 0.5 | **PR and merge**, with the breaking changes listed for API clients: the error format (ADR 0003) and the token format (`sub`, `iss`; everyone signs in again once). No clients run it (D5), so no migration window. | S | | Done |
| 0.6 | **First releases**: a `CHANGELOG.md` per module, `release.yml`, the core and every battery at `v0.1.0`, `auth` on the released core | S | 0.5 | Done |

### Phase 1: Security and contract gaps

| ID | Item | Size | Depends on | Status |
|---|---|---|---|---|
| 1.1 | **Access and refresh tokens told apart**: a token kind that `auth.Middleware` checks, and `POST /v1/auth/refresh`. Part of the auth battery (2.3). Needs D1. | M | 0.6 | |
| 1.2 | **Password policy** at registration, as a rule with its declared error. Needs D2. | S | | |
| 1.3 | **Server timeouts**: read, write and idle defaults in `httpserver`; long routes extend their own deadline | M | | Done |
| 1.4 | **Error code catalog**: a test that finds every declared `apperr` error, fails on duplicate codes, and generates `app/api/errors.md`. It later feeds the error responses in the OpenAPI spec (2.1). | M | | Done (`apperr.Declared`, `TestErrorCatalog`) |
| 1.5 | **The caller in `context.Context`**: `auth` puts a `Principal` (subject, tenant or workspace, scopes, method) in the request context, so services and jobs see who acts, and `auth.Require(scopes...)` guards routes. A job carries the principal that enqueued it. Casbin, OpenFGA or Cerbos stay recipes, not batteries. | M | | |
| 1.6 | **Shutdown that drains**: on SIGTERM `/ready` answers 503 and the server waits a configurable delay before closing the listener, so the load balancer stops sending first. | S | | Done (`SHUTDOWN_DRAIN_DELAY`) |
| 1.7 | **Request deadline** middleware: every request's context gets a deadline, and an expired one is `server.timeout` (503). | S | | Done (`REQUEST_TIMEOUT`) |
| 1.8 | **Headers**: CORS allows `X-Request-ID` and `X-API-Key` and exposes `X-Request-ID`, `Retry-After` and the rate-limit headers; `X-Content-Type-Options: nosniff` on every response; `http.CrossOriginProtection` when auth uses cookies. | S | | Done but `CrossOriginProtection`, which waits for cookie sessions (2.3) |

### Phase 2: Batteries

Each one is an opt-in module (ADR 0002), built with the app as its first consumer. The ecosystem research (2026-11-03) shaped this phase: take the ideas of FastAPI and Huma, build on the libraries the industry already maintains, and write ourselves only what is small.

| ID | Item | Size | Depends on |
|---|---|---|---|
| 2.1 | **`api` battery: typed operations, FastAPI style.** One input struct (path, query, header and body tags) drives binding, validation, OpenAPI 3.1 served at startup, security schemes and the error responses. Built on Huma through its Gin adapter, so Gin stays the router and routes move one at a time; `apperr` renders through Huma's error hook. Replaces swag and `make docs`. Needs D3. | L | 1.4 |
| 2.2 | **`env`** (named so it does not clash with the app's `config`): env loading with a `.env` fallback, typed getters, one error listing every problem. Extracted from `app/internal/config`. `cmd/migrate` stops needing `JWT_SECRET`. Done. | M | |
| 2.3 | **Auth battery v1**, split so heavy dependencies stay opt-in. Needs D1, D2, D6. `auth/oidc` is its own module. **Never built:** an authorization server or OIDC provider, SAML, SCIM, our own crypto, or a user admin UI. Apps that need SSO or share one login across apps use an IdP (Zitadel, Ory, Keycloak) and only the verifier. | L | 1.5 |
| 2.4 | **`queue` battery**: Postgres jobs enqueued in the same transaction as the write that causes them, with leases, retries with backoff, a visible failed state, concurrency per queue, and metrics for depth, age and wait. The app template gains `cmd/worker`. On River, or written here on `SKIP LOCKED`: needs D7. | L | |
| 2.5 | **`redis` battery**: client from a URL with a health check, cache helpers (`Get`/`Set`/`Delete`, `Remember` with single flight) and pub/sub. The app's `database/redis.go` moves here. It is what 4.2, 4.3 and 4.5 build on. | M | |
| 2.6 | **`httpclient`** (core, standard library only): outbound client with a timeout and `ctx` always, capped bodies, status errors as `apperr`, retries only for idempotent requests, and the request ID sent on every call. | M | |
| 2.7 | **`storage` out of the core** into its own battery, so an API without object storage stops downloading the AWS SDK. Breaking for the core. Done. | S | |
| 2.8 | **Test kit**: `pgtest` starts a Postgres container (testcontainers-go) when `TEST_DATABASE_URL` is empty, and clones a migrated template database instead of migrating per test. Migrations take goose's session lock. Done but the container, which adds a dependency. | M | |
| 2.9 | **`webhook`**: signed delivery out and verification in, on the Standard Webhooks spec, delivered as `queue` jobs. Inbound webhooks (Stripe, cal.com, a posting provider) are verified and stored before they are processed, so a failed handler is retried from the stored event. | M | 2.4 |
| 2.10 | **`mail`**: a `Sender` interface, SMTP (wneessen/go-mail) and a development sender that logs, sent as `queue` jobs. | S | 2.4 |
| 2.11 | **`storage` for uploads that skip the API**: presigned PUT, upload of a stream of unknown size (multipart), `Head` and `Exists`, `Copy`, `List` by prefix. Presigned multipart for browsers follows if a single PUT (5 GiB) is not enough. | M | |
| 2.12 | **Service-to-service auth**: API keys per client (a key store, not one static key) with the client as the `Principal`, and signed callbacks between services (HMAC over the body with a timestamp, the Standard Webhooks scheme of 2.9). | M | 1.5 |
| 2.13 | **`events`: a transactional outbox with several consumers per event.** A fact is written in the same transaction as the change; a dispatcher delivers it at least once to each registered handler, retried independently and idempotent by event and handler. On top of `queue`. | L | 2.4 |
| 2.14 | **Module boundaries at scale**: each feature owns its tables, queries and migrations (one sqlc package per feature, no single `store.Store` that imports them all), with a lint rule that fails a feature importing another's storage. | L | |
| 2.15 | **Feature flags**: evaluated per request and per job, by tenant and percentage. OpenFeature behind a small interface, or a recipe: to decide with the first flag Butter's migration needs. | S | |

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
| 3.3 | **`otel` battery**: tracing for Gin, pgx, the queue and `httpclient`; exemplars on the Prometheus metrics. The request ID and the trace travel into jobs and outbound calls. | L | 2.6 |
| 3.4 | **Admin listener**: pprof, `/metrics` and the probes on a separate port | M | 3.1 |
| 3.5 | **Error reporting**: panics and `server.internal` errors sent to a tracker (Sentry or any OTLP backend) with the request ID, from requests and from jobs. | S | |

### Phase 4: More than one replica

| ID | Item | Size | Depends on |
|---|---|---|---|
| 4.1 | **Jobs that run once per cluster**: periodic jobs through River (2.4), or a `job` option backed by a Postgres advisory lock | M | 2.4 |
| 4.2 | **Shared rate limits**: the limiter's counters in Redis | M | 2.5 |
| 4.3 | **SSE across replicas**: events published on one replica reach clients on every other, through Redis or Postgres `LISTEN/NOTIFY` | L | 2.5 |
| 4.4 | **Readiness that does not take every replica out at once** on a database blip | S | |
| 4.5 | **Websockets reachable from any replica**: a connection registry in Redis, commands published to the replica that holds the socket, replies routed back, and a close with a reconnect hint on shutdown. The same routing serves SSE (4.3). | L | 2.5 |

### Phase 5: API contract features

| ID | Item | Size | Depends on |
|---|---|---|---|
| 5.1 | **Cursor pagination**: opaque cursors next to page/offset | M | |
| 5.2 | **`Idempotency-Key`** for unsafe methods, results stored in Postgres first | L | |
| 5.3 | **Binding beyond the JSON body**: one typed input for path, query, header and body in the `Handle` adapters. Comes with 2.1 if D3 picks Huma; otherwise built on Gin's binding. | M | D3 |

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
| D5 | Is the app deployed with real clients? | Decided: no. The app is the example built on Medusa, never deployed | 0.5 |
| D6 | Auth scope: which logins Medusa ships (password, OIDC providers, passkeys) and when an app must use an external IdP instead | Password and OIDC in v1, passkeys next; IdP for SSO and shared logins | 2.3 |
| D7 | The `queue` battery: River (MPL-2.0, Postgres only), or written here on `SKIP LOCKED` with leases. Butter's ADR 0004 keeps its two in-house implementations over River for now, and asks for the lease-token semantics | River, unless Butter's lease and correlation ID semantics cannot be expressed on it | 2.4, 2.13, 4.1 |

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
- **Order**: the table under "First consumer" decides what comes next. Within it, phases still hold: 3 needs 2.2, and 6 comes last.
