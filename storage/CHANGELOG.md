# Changelog

All notable changes to `github.com/imlargo/medusa/storage`. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while it is `v0`, a minor version may break the API.

## [Unreleased]

## [0.1.0] - 2026-11-04

### Added

- S3-compatible object storage (Cloudflare R2 by default): upload, download, presigned URLs, bulk delete, and key validation. Moved out of the core so APIs without object storage do not download the AWS SDK.

[Unreleased]: https://github.com/imlargo/medusa/compare/storage/v0.1.0...HEAD
[0.1.0]: https://github.com/imlargo/medusa/releases/tag/storage/v0.1.0
