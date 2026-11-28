# Changelog

All notable changes to `github.com/imlargo/medusa/auth`. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while it is `v0`, a minor version may break the API.

## [Unreleased]

### Added

- `Principal`: who a request acts for (kind, subject, tenant, scopes, method), carried in the request's `context.Context`. `PrincipalFrom(ctx)` and `SubjectFrom[S](ctx)` read it anywhere, and `WithPrincipal` sets it where there is no request.
- `Authenticator`, and `Middleware(authenticators...)` trying several in order. `*JWT[S]` is one; `APIKeys(lookup)` authenticates one key per client by its hash (`HashAPIKey`).
- `Enrich`: completes the principal per request with what the application reads then, such as the tenant and its scopes.
- `Require(scopes...)`: answers 403 `auth.insufficient_scope` without them.
- `WithScopes` and `WithTenant` for `Sign`; `Claims` has `Scopes` and `Tenant`.
- `Principal.Metadata` and `PrincipalFromMetadata`, to carry who caused a job into its handler.

### Changed

- **Breaking:** `SubjectFrom` takes a `context.Context` and reads the principal from it. Passing a handler's `*medusa.Context` still works.
- **Breaking:** a request whose `Authorization` header uses a scheme other than Bearer answers `auth.missing_credentials`, no longer `auth.malformed_credentials`: another authenticator may read that scheme.

## [0.1.1] - 2026-11-04

### Fixed

- `auth` requires the released core `github.com/imlargo/medusa v0.1.0`. `v0.1.0` required a core version that does not exist and is retracted: use `v0.1.1`.

## [0.1.0] - 2026-11-04 [RETRACTED]

### Added

- `JWT[S]`: HS256 tokens with a typed subject (`int64` or `string`) in the standard `sub` claim and an optional issuer.
- `Middleware` requires a valid bearer token and answers `auth.invalid_token` otherwise; `SubjectFrom[S]` reads the subject in a handler.

[Unreleased]: https://github.com/imlargo/medusa/compare/auth/v0.1.1...HEAD
[0.1.1]: https://github.com/imlargo/medusa/releases/tag/auth/v0.1.1
[0.1.0]: https://github.com/imlargo/medusa/releases/tag/auth/v0.1.0
