# Changelog

All notable changes to the Medusa core (`github.com/imlargo/medusa`). Each battery keeps its own changelog in its directory. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while Medusa is `v0`, a minor version may break the API.

## [Unreleased]

## [0.1.0] - 2026-11-04

The first release of Medusa as a toolkit. `v0.0.1` was an application template; it is not an earlier version of this API.

### Added

- `medusa`: `Context` on Gin, typed handler adapters (`Handle`, `HandleCreate`, `HandleGet`, `HandleUpdate`, `HandleDelete`), binding with validation errors named by the client's field names, pagination and sorting limited to allowed fields, success writers, and `WriteError`.
- `apperr`: application errors with a kind, a stable `domain.rule` code and a client-safe message, independent of HTTP, plus `Declared` to build an API's error catalog.
- `httperr`: kinds to HTTP statuses, and errors rendered as RFC 9457 problem details.
- `middleware`:
  - `RequestID` (carried in the request context);
  - `SecureHeaders`;
  - `Recover`;
  - `Timeout`;
  - `CORS`;
  - `BodyLimit`;
  - `Metrics`;
  - `RateLimit`;
  - `APIKey` and `BearerAPIKey`.
- `LogHandler`: adds `request_id` to every log line written with a request's context.
- `health`: readiness checks as functions, `/health` and `/ready` handlers, and `Drain` to leave the load balancer's rotation before shutdown. Check errors stay out of the public response.
- `httpserver`: serves any `http.Handler` with read, write, idle and header timeouts and graceful shutdown.
- `lifecycle`: runs servers and hooks, and stops them on SIGINT or SIGTERM.
- `job`: periodic background work owned by `lifecycle`.
- `metrics`: Prometheus HTTP metrics on an injected registry.

[Unreleased]: https://github.com/imlargo/medusa/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/imlargo/medusa/releases/tag/v0.1.0
