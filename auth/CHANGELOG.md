# Changelog

All notable changes to `github.com/imlargo/medusa/auth`. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while it is `v0`, a minor version may break the API.

## [Unreleased]

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
