# Context

## What this is

Medusa started as a Go framework with an application mixed into it. It is now two modules in one repository:

- **Medusa** (root): a toolkit of small packages for HTTP APIs on Gin, meant to become the library every future app depends on.
- **The app** (`app/`): a working API on Medusa (accounts, JWT, server-sent events, Postgres) and the starting point for new apps.

Production project. Quality target: an enterprise-grade Go codebase, following the research in `reports/` and `research_notes/`.

## Hard constraints

- Gin stays as the HTTP layer. Medusa works with Gin instead of hiding it.
- Medusa's public API must not lock in a data access library.

## Status (2026-11-01)

Phase 1, separating Medusa from the app, is done on branch `refactor/separate-medusa-20261101` (pushed, not merged). Data access moved from GORM to pgx v5 + sqlc + goose in the same phase.

Iterated with lessons from the hellobutter/butter backend, a large production app on the old stack:

- `RequestID` only reuses valid UUIDs.
- Empty lists are sent as `[]`.
- Canceled requests answer 499 and timeouts 503.
- New `job` package for periodic work.
- `store.WithTx` and `dbtest` in the app.

Not verified yet:

- Queries and migrations against a real Postgres, including adopting the baseline migration on a database GORM created. The repository and `WithTx` tests cover this but skip without `TEST_DATABASE_URL`.
- Register, login and the user endpoint end to end.
- `docker build -f app/Dockerfile .`, and `-race` for every package except `job`.

Pending:

- Review and merge the branch.
- Tag Medusa `v0.1.0` (`v0.0.1` is the old layout).
- A CI pipeline: vet, test with `-race`, govulncheck, image build, and a Postgres service with `REQUIRE_DB_TESTS=1`.

## Next phase: best practices

Applying the research to the whole codebase:

- API contracts: problem+json errors, cursor pagination, Idempotency-Key.
- Hardening: read, write and idle timeouts in `httpserver`, `http.CrossOriginProtection`.
- Observability: OpenTelemetry, an admin port for pprof, metrics and health, signal handling in `main`.
- Tooling: golangci-lint v2, a swaggo replacement.
- An outbound HTTP client in Medusa (timeout and ctx always, capped bodies, typed status errors, opt-in idempotent retries). butter rebuilt one in about a dozen packages.
- App code: services returning domain errors instead of `httperr`, packages by feature instead of by layer.
