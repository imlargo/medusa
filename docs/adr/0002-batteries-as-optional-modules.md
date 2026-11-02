# 0002: Batteries as optional modules

Date: 2026-11-02. Status: accepted. Builds on [0001](0001-separate-medusa-from-the-app.md).

## Context

ADR 0001 moved everything app-shaped out of Medusa. That kept the core clean, but the parts every API needs (a Postgres pool, migrations, test databases, JWT auth) ended up inside `app/`, so a second app has to copy them. Medusa's goal is to be batteries included.

Batteries-included Go frameworks exist and are used in production. go-zero ships them all in one module; Kratos keeps a small core and puts integrations with heavy dependencies in separate `contrib/` modules. Medusa follows Kratos: the constraint from ADR 0001 that the public API must not lock in a data access library is only kept if choosing a battery is opt-in at the `go.mod` level.

## Decision

- **Three layers**: the core (root module, unchanged), official batteries in their own modules under the repository root, and later a starter kit (`medusa.New`) and a code generator on top.
- **One module per battery**: `github.com/imlargo/medusa/postgres` (`postgres/`) and `github.com/imlargo/medusa/auth` (`auth/`). An app that does not import a battery never downloads its dependencies (pgx, goose, golang-jwt).
- **The core stays dependency-light**: the rule "Medusa never imports a database driver" now reads "the core never imports one". A battery may expose its library's types (`*pgxpool.Pool`, `pgx.Tx`) because choosing the battery is choosing the library.
- **What moved out of the app**:
  - `postgres`: `Open` (pool, ping, slow query log), `Migrate` (goose over any `fs.FS`), `Conn`, `IsNoRows` and `IsUniqueViolation`.
  - `postgres/pgtest`: a fresh migrated database per test.
  - `auth`: `JWT[S]` with a typed subject (`int64` or `string`), `WithIssuer`, `Middleware` and `SubjectFrom[S]`.
- **What stays in the app**: the store and its `WithTx` (they know the app's repositories), the migrations themselves, and the repositories' sentinel errors.
- **Tokens use the standard `sub` claim** instead of the app's former `user_id`, and carry `iss` (`JWT_ISSUER`, which was configured but never used).

## Alternatives considered

- **Batteries as packages of the core module**: one `go get`, but every app would download pgx, goose and golang-jwt whether it uses them or not, and a breaking change in one battery would force a major version of the whole toolkit.
- **A separate repository per battery**: independent, but a change across the core and a battery would need several PRs and releases.
- **Leave them in the app as a template**: what ADR 0001 did. Every new app forks the code, and fixes stop propagating.

## Consequences

- Each battery is tagged on its own with a path prefix: `postgres/v0.1.0`, `auth/v0.1.0`. A battery that depends on the core (`auth`) needs the core tagged first, then its own `replace` dropped.
- `./...` from the root covers only the core: check everything with `go test ./... ./auth/... ./postgres/... ./app/...`.
- Tokens issued before this change have no `sub` or `iss` and are rejected: users sign in again once after the deploy.
