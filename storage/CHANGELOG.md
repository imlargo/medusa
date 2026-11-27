# Changelog

All notable changes to `github.com/imlargo/medusa/storage`. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while it is `v0`, a minor version may break the API.

## [Unreleased]

### Added

- `PresignUpload`: a presigned PUT, with the content type and the exact size signed into it, for uploads that go from a browser or another service straight to the bucket.
- `UploadStream`: uploads a reader of unknown size in parts of `StreamPartSize`, and discards the parts if it fails.
- `Head` and `Exists`: an object's size, content type, ETag and last modification, without its content.
- `Copy`: a copy inside the bucket, done by the provider.
- `List`: the objects under a prefix, paged with a cursor.

### Changed

- **Breaking for implementations of `FileStorage`:** the interface has the six methods above. Code that only calls it is not affected; a fake or a wrapper of your own has to add them.

## [0.1.0] - 2026-11-04

### Added

- S3-compatible object storage (Cloudflare R2 by default): upload, download, presigned URLs, bulk delete, and key validation. Moved out of the core so APIs without object storage do not download the AWS SDK.

[Unreleased]: https://github.com/imlargo/medusa/compare/storage/v0.1.0...HEAD
[0.1.0]: https://github.com/imlargo/medusa/releases/tag/storage/v0.1.0
