# Changelog

All notable changes to `github.com/imlargo/medusa/tracing`. The format follows [Keep a Changelog](https://keepachangelog.com), and versions follow [SemVer](https://semver.org): while it is `v0`, a minor version may break the API.

## [Unreleased]

### Added

- `Setup` starts OpenTelemetry tracing for a process and exports spans over OTLP/HTTP to the endpoint in `OTEL_EXPORTER_OTLP_ENDPOINT`. Without an endpoint nothing is traced.
- `Middleware` traces requests on Gin and `Transport` outbound HTTP calls, passing the trace on in the W3C `traceparent` header.
- `QueryObserver` traces database statements through `postgres.WithQueryObserver`, recording the statement and never its arguments.
- `Inject`, `StartJob` and `EndJob` carry a trace into the jobs a request enqueues.
- `LogHandler` adds `trace_id` and `span_id` to log lines.

[Unreleased]: https://github.com/imlargo/medusa/commits/main/tracing
