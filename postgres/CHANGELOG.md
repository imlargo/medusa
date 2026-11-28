# Changelog

All notable changes to `github.com/imlargo/medusa/postgres`. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while it is `v0`, a minor version may break the API.

## [Unreleased]

## [0.2.0] - 2026-11-27

### Added

- `postgres/queue`: a job queue on `FOR UPDATE SKIP LOCKED`.
  - `Install` creates and upgrades its table, `medusa_jobs`.
  - `EnqueueTx` adds a job inside the caller's transaction, with a queue, a priority, a delay, a number of attempts and unique keys.
  - `Worker` claims jobs under a lease with a fencing token, renews it, retries failures with backoff, recovers jobs whose worker died, and on shutdown hands running jobs back without spending an attempt.
  - `Schedule` runs a job at the minutes of a cron expression (UTC) and `Periodic` once per interval, each once across every replica.
  - `Failed` lists the jobs that ran out of attempts and `Retry` puts one back.
  - `Stats` reports depth, running, failed and the age of the oldest ready job per queue.

## [0.1.0] - 2026-11-04

### Added

- `Open`: a pgx pool with pool defaults the URL can override, a ping at startup and a slow query log.
- `Migrate`: goose migrations from any `fs.FS`, serialized by an advisory lock.
- `Conn`, `IsNoRows`, `IsUniqueViolation`.
- `pgtest.New`: a fresh database per test, cloned from a template migrated once.

[Unreleased]: https://github.com/imlargo/medusa/compare/postgres/v0.2.0...HEAD
[0.2.0]: https://github.com/imlargo/medusa/releases/tag/postgres/v0.2.0
[0.1.0]: https://github.com/imlargo/medusa/releases/tag/postgres/v0.1.0
