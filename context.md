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

## Status (2026-11-02)

Separating Medusa from the app is done on branch `refactor/separate-medusa-20261101` (pushed, not merged). Data access moved from GORM to pgx v5 + sqlc + goose in the same phase.

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
- Registration now requires `email` (valid format) and `password`. No password policy exists beyond that (decision D2 in the roadmap).

## What's next

The phases, open items, pending decisions and how the work runs are in [docs/roadmap.md](docs/roadmap.md). Phase 0 (CI, merge, first tags) comes before anything else: the database tests, the end-to-end flows and the Docker build have never run.
