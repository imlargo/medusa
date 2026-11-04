# Changelog

All notable changes to `github.com/imlargo/medusa/env`. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while it is `v0`, a minor version may break the API.

## [Unreleased]

## [0.1.0] - 2026-11-04

### Added

- `LoadDotEnv`: loads `.env` files without overriding the real environment.
- `Env`: typed getters (`String`, `Required`, `Int`, `Bool`, `Duration`, `List`) that collect every problem into one `Err`.

[Unreleased]: https://github.com/imlargo/medusa/compare/env/v0.1.0...HEAD
[0.1.0]: https://github.com/imlargo/medusa/releases/tag/env/v0.1.0
