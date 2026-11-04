# Changelog

All notable changes to `github.com/imlargo/medusa/postgres`. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while it is `v0`, a minor version may break the API.

## [Unreleased]

## [0.1.0] - 2026-11-04

### Added

- `Open`: a pgx pool with pool defaults the URL can override, a ping at startup and a slow query log.
- `Migrate`: goose migrations from any `fs.FS`, serialized by an advisory lock.
- `Conn`, `IsNoRows`, `IsUniqueViolation`.
- `pgtest.New`: a fresh database per test, cloned from a template migrated once.

[Unreleased]: https://github.com/imlargo/medusa/compare/postgres/v0.1.0...HEAD
[0.1.0]: https://github.com/imlargo/medusa/releases/tag/postgres/v0.1.0
