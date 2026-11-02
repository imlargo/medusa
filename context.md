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

Not verified yet:

- Queries and migrations against a real Postgres, including adopting the baseline migration on a database GORM created.
- Register, login and the user endpoint end to end.
- `docker build -f app/Dockerfile .` and tests with `-race`.

Pending:

- Review and merge the branch.
- Tag Medusa `v0.1.0` (`v0.0.1` is the old layout).
- A CI pipeline: vet, test with `-race`, govulncheck, image build.

## Next phase: best practices

Applying the research to the whole codebase:

- API contracts: problem+json errors, cursor pagination, Idempotency-Key.
- Hardening: read, write and idle timeouts in `httpserver`, `http.CrossOriginProtection`, a length limit on `X-Request-ID`.
- Observability: OpenTelemetry, an admin port for pprof, metrics and health, signal handling in `main`.
- Tooling: golangci-lint v2, a swaggo replacement.
- App code: services returning domain errors instead of `httperr`, packages by feature instead of by layer, integration tests for repositories against Postgres.
