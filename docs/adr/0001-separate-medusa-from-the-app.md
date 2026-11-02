# 0001: Separate Medusa from the app

Date: 2026-11-01. Status: accepted.

## Context

The repository mixed Medusa, a Gin-based framework, with an application built on it. Medusa's public API exposed GORM (`core/repository`), zap (`core/logger`), swaggo (`core/docs`) and the app's identity model (`UserID uint`). A future app could not depend on Medusa without inheriting all of that, and Medusa could not be versioned separately.

The research in `reports/` points the same way on every count:

- Mercado Libre's Go toolkit splits a portable core, versioned and stdlib-like, from the bootstrap tied to its environment.
- go.dev recommends moving reusable packages into their own module.
- None of the reference projects is built on a web framework.
- Meli removed its own observability abstraction in favor of the standard one.

## Decision

- **Medusa is a toolkit, not a framework**: small independent packages, standard library types below the Gin edge, no `main`, no global state, no data access, config or identity.
- **One repository, two modules**: Medusa at the root (`github.com/imlargo/medusa`), the app in `app/` (`github.com/imlargo/medusa/app`). The app uses `replace => ../`, and a committed `go.work` ties both together.
- **Moved to the app**: JWT and the user ID (now `int64`), config and `.env` loading, Swagger serving, URL helpers.
- **Deleted**: the GORM repository, the zap logger wrapper, base structs to embed, and `cache`, `push` and `oauth` (no consumers).
- **Data access**: the app uses pgx v5 + sqlc, with goose migrations run by `cmd/migrate`, never on startup. The baseline migration reproduces the GORM schema with `IF NOT EXISTS` so existing databases adopt it.
- **Logging**: Medusa takes `*slog.Logger`; the app uses `slog.NewJSONHandler`.
- **Package names**: `context` and `server/http` (which shadowed the stdlib) and `core/responses` became the root package, `httpserver` and `httperr`. `app` became `lifecycle` so it would not collide with the app's directory.

## Alternatives considered

- **Two repositories**: the app could be cloned on its own, but every change touching both needs two PRs and a Medusa release in between. Rejected for now.
- **One module**: cheapest, but Medusa could not be versioned or consumed by another app.
- **Keep GORM and switch later**: avoids a second risky change, but leaves query errors at runtime and `AutoMigrate` in production.
- **Keep zap and switch later**: every Medusa signature would change twice.

## Consequences

- An app copied out of `app/` must change its module path and imports, drop the `replace` and require a released Medusa version.
- `./...` from the root does not include the app: check both with `./... ./app/...`.
- Log keys changed from zap's (`ts`, `caller`) to slog's (`time`).
- Medusa has to be tagged (`v0.1.0`) before any other repository can depend on this layout.
