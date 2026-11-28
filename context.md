# Context

## What this is

Medusa started as a Go framework with an application mixed into it. It is now several modules in one repository:

- **Medusa core** (root): small packages for HTTP APIs on Gin.
- **Medusa batteries** (`postgres/`, `auth/`): optional modules for what most APIs need. The goal is a batteries-included framework whose batteries are opt-in (ADR 0002).
- **The app** (`app/`): a working API on Medusa (accounts, JWT, server-sent events, Postgres) and the starting point for new apps.

Production project. Quality target: an enterprise-grade Go codebase, following the research in `reports/` and `research_notes/`.

## Hard constraints

- Gin stays as the HTTP layer. Medusa works with Gin instead of hiding it.
- Medusa's public API must not lock in a data access library.

## Status (2026-11-04)

Medusa is on `main` (PR #19) and released: the core, `env`, `postgres` and `storage` at `v0.1.0`, `auth` at `v0.1.1` (GitHub releases and the Go proxy). `auth/v0.1.0` is retracted: it was tagged before `auth` required the released core. The app is the example built on Medusa; it is not deployed and has no clients.

The separation from the app came first. Data access moved from GORM to pgx v5 + sqlc + goose in the same phase.

Iterated with lessons from the hellobutter/butter backend, a large production app on the old stack:

- `RequestID` only reuses valid UUIDs.
- Empty lists are sent as `[]`.
- Canceled requests answer 499 and timeouts 503.
- New `job` package for periodic work.
- `store.WithTx` and a migrated test database per test in the app.

Then the first batteries (2026-11-02), extracted from the app:

- `postgres`: pool, migrations, `Conn`, error helpers; `postgres/pgtest` for test databases.
- `auth`: JWT with a typed subject and issuer, middleware. Tokens now carry `sub` and `iss` (`JWT_ISSUER` was configured but unused), so tokens issued before the deploy are rejected and users sign in again once.

Then the error and business rule standard (2026-11-02, ADR 0003):

- Core package `apperr`: errors with a kind and a `domain.rule` code, independent of HTTP.
- Errors are sent as RFC 9457 problem details. This breaks the error contract for existing clients: `error` and `details` became `detail` and `errors`, and codes changed (`NOT_FOUND` -> `user.not_found`).
- The app is organized by feature: `user`, `account`, `events`.
- Registration now requires `email` (valid format) and `password`. The password policy came later (D2): see below.

Then CI and a review pass (2026-11-03):

- CI for every module (tests against Postgres, `-race`, lint, govulncheck, Docker).
- Dependencies and the Go toolchain (1.27.3) moved past their security advisories.
- Server read, write and idle timeouts.
- Ten bugs from an independent review fixed (listed in the roadmap). The client-visible ones:
  - panics answer a `server.internal` problem;
  - validation errors name fields by their JSON names;
  - bad query parameters are `request.invalid_parameter`;
  - `/ready` no longer includes the checks' error messages.

Then the rest of phase 1 that needed no decision, and more batteries (2026-11-04):

- Shutdown leaves the load balancer's rotation first (`SHUTDOWN_DRAIN_DELAY`).
- Every request has a deadline (`REQUEST_TIMEOUT`), and responses carry secure headers.
- `request_id` is on every log line.
- The error code catalog (`app/api/errors.md`) is generated and checked in CI.
- `storage` and the new `env` battery left the core.
- Migrations take a lock, and `pgtest` clones a migrated template.
- An end-to-end account flow test runs against Postgres in CI.

Then the first work for Butter's backend, Medusa's first consumer (2026-11-27):

- The roadmap lists the twelve gaps an audit against Butter's design found, with their order and issues (#22 to #33).
- `storage`: presigned uploads, stream uploads, `Head`, `Exists`, `Copy`, `List`.
- `httpclient` in the core.
- Go 1.27.4 for a `net/http` advisory.

Released as the core `v0.2.0` and `storage/v0.2.0`.

Then, for the same consumer, released on 2026-11-28 as the core `v0.3.0`, `postgres/v0.3.0` and `auth/v0.2.0`:

- A job queue on Postgres with a worker, cron schedules and metrics (ADR 0004), and events on top of it (ADR 0006).
- The caller as a `Principal` in the context, scopes, API keys per client and rotating refresh tokens (ADR 0005).
- Signed webhooks and callbacks, error reporting hooks, one request ID from request to job to callback.
- Each feature owns its tables, and transactions travel in the context (ADR 0007).

Decided on 2026-11-28: argon2id with a minimum of 12 characters (D2), no response envelope on the `api` battery (D8), OpenTelemetry in an `otel` battery (D9).

Then released on 2026-11-28 as `api/v0.1.0`, `tracing/v0.1.0`, the core `v0.4.0`, `postgres/v0.4.0` and `auth/v0.3.0`:

- The `api` battery on Huma (ADR 0008), with the example app's routes on it and swag removed.
- The `tracing` battery on OpenTelemetry.
- Feature flags (`flags`, `postgres/flagstore`).
- `auth/password`: argon2id and the password policy.

Every module is released; nothing waits on `main`. Open: mail, more than one replica, and stored `Idempotency-Key` results.

Decided on 2026-11-27: the queue is written in Medusa on `SKIP LOCKED` (D7), auth v1 ships password and OIDC logins with opaque, rotating refresh tokens (D1, D6), and the API contract is built on Huma (D3).

## What's next

The phases, open items, pending decisions and how the work runs are in [docs/roadmap.md](docs/roadmap.md). Phase 0 is done. `main` only takes PRs whose `ci` check passed, and a module is released by its tag (see Release a module in AGENTS.md). Next are the decisions D1-D7, which unblock the auth battery, the `api` battery and the `queue` battery.
