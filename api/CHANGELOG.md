# Changelog

All notable changes to `github.com/imlargo/medusa/api`. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while it is `v0`, a minor version may break the API.

## [Unreleased]

## [0.1.0] - 2026-11-28

### Added

- `New` starts an API contract on a Gin engine, built on Huma: an OpenAPI 3.1 document made from the routes as they are registered, served with a page to browse it when `WithDocs` is on.
- `Get`, `Post`, `Put`, `Patch`, `Delete` register a typed handler, `func(ctx, *Input) (*Resource, error)`, on a `Group` that shares a Gin group's prefix and middleware. The input is bound and validated from its tags; the resource is answered as it is, with no envelope.
- Errors are RFC 9457 problems in Medusa's vocabulary, whether they come from validation (`request.validation_failed`, with fields named as the client sent them) or from the handler (its `apperr` error). `Errors` lists a route's errors in the document, by status and code.
- `Secured` guards a group with middleware and documents its security scheme in one declaration; `Scopes` does the same for a route's scopes.
- `Status`, `Empty`, `Summary`, `Description`, `Idempotent`, `Hidden`, `MaxBody`, group `Tags` and `Internal`.
- `Optional[T]` tells a field that was not sent from one sent as null, for partial updates.
- `Page`, `List[T]`, `NewList`, `EncodeCursor` and `DecodeCursor`: one cursor shape and one set of limits for every list.
- `Document` lists a route served by a plain Gin handler, such as a stream. `Request` gives a handler the HTTP request, for a signed webhook's headers.

[Unreleased]: https://github.com/imlargo/medusa/compare/api/v0.1.0...HEAD
[0.1.0]: https://github.com/imlargo/medusa/releases/tag/api/v0.1.0
