# Observability, Resilience, Configuration, Security and Operations for Scalable Go Backends

Research date: 2026-11-01. Scope note: about 20 tool calls. Items marked "not verified in this session" come from background knowledge and were not confirmed against a fetched source, so treat them as leads, not citations.

## Logging: log/slog best practices, comparison with zap/zerolog, what to log and never log

### Takeaway
`log/slog` (Go 1.21+) is the standard structured logging frontend. Its Logger/Handler split lets zap or other backends sit behind it. For production, use `JSONHandler`, `LogAttrs` on hot paths, `With` for request-scoped fields, the context-taking methods so handlers can add trace IDs, and `LogValuer` to redact sensitive types at the type level.

### Cited Findings
- slog is Go's standard structured logging package (Go 1.21). Structured logs use key-value pairs so they can be "parsed, filtered, searched, and analyzed quickly and reliably". — [Go blog: Structured Logging with slog](https://go.dev/blog/slog)
- The package is split into a frontend (`Logger`) and a backend interface (`Handler`). Zap, logr and hclog have handlers, so they can share the slog frontend. — [Go blog: slog](https://go.dev/blog/slog)
- Built-in handlers: `TextHandler` writes `key=value`, and `JSONHandler` writes one JSON object per call, which suits log pipelines. A handler can wrap another handler to add behavior such as level filtering. `testing/slogtest` verifies custom handlers. — [Go blog: slog](https://go.dev/blog/slog)
- Levels are integers (Info=0, Warn=4), so intermediate levels are possible. — [Go blog: slog](https://go.dev/blog/slog)
- Performance: `Attr` values plus `LogAttrs` "work together to minimize memory allocations". The Go team found "over 95% of calls to logging methods pass five or fewer attributes" and optimized allocations for that case. `Logger.With` and handler `WithAttrs`/`WithGroup` let attributes be pre-formatted once. `Enabled` runs first, so a handler can drop events cheaply. — [Go blog: slog](https://go.dev/blog/slog)
- Context: some methods take a `context.Context` so a handler can pull values such as trace IDs. Canceling the context does not stop the record from being written. — [Go blog: slog](https://go.dev/blog/slog)
- `LogValuer`: a type's `LogValue` method controls how it is logged, for example as a group of fields or redacted. — [Go blog: slog](https://go.dev/blog/slog)
- A `go vet` check catches mismatched alternating key/value arguments. — [Go blog: slog](https://go.dev/blog/slog)
- The official OTel bridge for slog is the contrib module `go.opentelemetry.io/contrib/bridges/otelslog`. — [pkg.go.dev otelslog](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/otelslog); [Uptrace otelslog guide](https://uptrace.dev/guides/opentelemetry-slog)
- OpenFeature's Go SDK ships a `LoggingHook` built on slog, a sign the ecosystem is converging on slog as the shared logging interface. — [OpenFeature Go SDK](https://openfeature.dev/docs/reference/sdks/server/go/)
- What never to log, from the security side: excessive data exposure and secrets are core API risks (API3:2023 Broken Object Property Level Authorization, API8:2023 Security Misconfiguration). — [OWASP API Security Top 10 2023](https://api-security.owasp.org/editions/2023/en/0x11-t10)

### Inferences
- Recommended pattern: one `*slog.Logger` built at startup with `slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: ...})`, injected as a dependency (avoid `slog.SetDefault` in libraries). A middleware adds `request_id`, `trace_id`, route and user ID through `With`, and callers use `InfoContext`/`ErrorContext` so an otelslog or custom handler can correlate logs with traces.
- Implement `LogValue()` on types such as `Password`, `Token`, `Email` and `User` that returns `slog.StringValue("REDACTED")` or a safe subset. Redaction then holds even when someone logs the whole struct. A `ReplaceAttr` in `HandlerOptions` can be a second, key-based line of defense (not verified in this session: [slog docs](https://pkg.go.dev/log/slog)).
- Writing to stdout and leaving routing to the platform matches 12-factor's logs-as-event-streams factor (factor text not fetched in this session).
- Never log credentials, tokens, session cookies, full Authorization headers, raw request bodies with PII, card data or secrets from config. Log the decision context instead: status, latency, route template (not the raw path with IDs if cardinality matters), error class.

### Gaps
- No primary-source benchmark comparing slog with zap and zerolog was fetched. Background knowledge (not verified in this session): zap and zerolog advertise zero-allocation encoders and remain faster in microbenchmarks. zap provides `zapslog` so it can serve as an slog backend. The usual advice is slog as the API, with zap behind it only if profiling shows logging is hot.
- Third-party linters such as sloglint were not researched.

## OpenTelemetry Go, Prometheus, RED/USE, exemplars

### Takeaway
As of late 2026, all three OpenTelemetry Go signals are stable. Traces and metrics have been stable for a while, and the Logs API/SDK reached its first stable release in v1.47.0 (2026-10-02), though opentelemetry.io's status page still said "release candidate" when fetched. Semantic conventions are at semconv v1.43.0, and the SDK now requires Go 1.26+.

### Cited Findings
- opentelemetry.io lists Go Traces: Stable, Metrics: Stable, Logs: Release candidate. — [OpenTelemetry Go docs](https://opentelemetry.io/docs/languages/go/)
- The opentelemetry-go CHANGELOG says release 1.47.0 (tags 1.47.0/0.69.0/0.23.0/0.1.0, dated 2026-10-02) "contains the first stable release of the OpenTelemetry Go Logs API and SDK" (`go.opentelemetry.io/otel/log`, `go.opentelemetry.io/otel/sdk/log`). This conflicts with the "Release candidate" label on the docs page, which is probably out of date. — [opentelemetry-go CHANGELOG](https://raw.githubusercontent.com/open-telemetry/opentelemetry-go/main/CHANGELOG.md); contradicted by [OpenTelemetry Go docs](https://opentelemetry.io/docs/languages/go/)
- History: the first alpha of the Go Logs SDK shipped in v1.26.0, and the first beta of the Logs Bridge API/SDK in v1.27.0/v0.49.0. — [opentelemetry-go releases v1.26.0](https://github.com/open-telemetry/opentelemetry-go/releases/tag/v1.26.0)
- Semantic conventions: `go.opentelemetry.io/otel/semconv/v1.43.0` was added in 1.45.0 (2026-08-03). — [opentelemetry-go CHANGELOG](https://raw.githubusercontent.com/open-telemetry/opentelemetry-go/main/CHANGELOG.md)
- Go version policy: 1.42.0 dropped Go 1.24. 1.46.0 was the last release supporting Go 1.25, so 1.47.0 requires Go 1.26+. — [opentelemetry-go CHANGELOG](https://raw.githubusercontent.com/open-telemetry/opentelemetry-go/main/CHANGELOG.md)
- otelslog is the contrib bridge from slog to the OTel Logs API. — [pkg.go.dev otelslog](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/otelslog)

### Inferences
- Pin Go 1.26+ in services that want current OTel releases.
- Standard instrumentation stack (not verified in this session): `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` (`otelhttp.NewHandler` server-side, `otelhttp.NewTransport` client-side), `.../google.golang.org/grpc/otelgrpc` (stats handlers `otelgrpc.NewServerHandler`/`NewClientHandler`), and `github.com/XSAM/otelsql` for `database/sql`. Export over OTLP to an OpenTelemetry Collector rather than straight to a vendor, and set the `service.name` resource attribute.
- RED (Rate, Errors, Duration) for request-driven services and USE (Utilization, Saturation, Errors) for resources (Tom Wilkie / Brendan Gregg; not verified in this session). In Prometheus client_golang that means a histogram `http_server_request_duration_seconds` labelled by route template, method and status class. Keep label cardinality bounded: no user IDs or raw paths.
- Exemplars attach a trace ID to a histogram bucket observation. client_golang supports them through `ObserveWithExemplar`, and they need OpenMetrics exposition (not verified in this session). This links a latency spike on a dashboard to a concrete trace.

### Gaps
- Didn't fetch the otelhttp/otelgrpc/otelsql or client_golang exemplar docs, so specific API names above are unverified.
- Couldn't confirm the version number of the otelslog contrib module at the time of the stable Logs release. The pkg.go.dev snapshot showed v0.8.0 (Dec 2024), which is old.
- No Grafana/Honeycomb/Datadog engineering-blog material was fetched.

## Profiling: pprof exposure, continuous profiling, PGO, GOMAXPROCS (Go 1.25), GOMEMLIMIT, GC tuning

### Takeaway
Since Go 1.25, GOMAXPROCS follows the container's CPU limit (not its request) and updates when the limit changes, so `automaxprocs` is mostly unnecessary. In containers you own, set GOMEMLIMIT to the container memory limit minus 5-10% headroom. Feed merged production CPU profiles into PGO (`default.pgo`, auto-detected since Go 1.21) for about 2-14% gains. Expose pprof only on an internal port.

### Cited Findings
- Go 1.25: "GOMAXPROCS will default to the CPU limit if it is less than the core count." Fractional limits round up. The runtime "will also periodically check the CPU limit and adjust GOMAXPROCS automatically if it changes." — [Go blog: Container-aware GOMAXPROCS](https://go.dev/blog/container-aware-gomaxprocs)
- CPU requests are not used, because pods with requests but no limits may use idle CPU. The new default applies only when GOMAXPROCS isn't set through the env var or `runtime.GOMAXPROCS`. It requires `go 1.25.0`+ in go.mod. Spiky workloads may see added latency. — [Go blog: Container-aware GOMAXPROCS](https://go.dev/blog/container-aware-gomaxprocs)
- GC: target heap = live heap + (live heap + GC roots) * GOGC/100. "Doubling GOGC will double heap memory overheads and roughly halve GC CPU cost." — [Go GC guide](https://go.dev/doc/gc-guide)
- GOMEMLIMIT (Go 1.19+) is a soft limit on total runtime memory. Use it when "the Go program is the only program with access to some set of resources (i.e. ... a container memory limit)". "Leave an additional 5-10% of headroom." GC CPU is capped at about 50% over a 2*GOMAXPROCS CPU-second window to prevent thrashing, so a bad limit slows a program "at most by 2x". Don't use it to dodge OOM when already near the limit, because that "replaces an out-of-memory risk with a risk of severe application slowdown". `GOGC=off` with a limit is valid only when the program owns the memory. — [Go GC guide](https://go.dev/doc/gc-guide)
- Monitor RSS, not VSS. Prefer cleanups over finalizers (Go 1.24+), and use explicit `Close` for non-memory resources. — [Go GC guide](https://go.dev/doc/gc-guide)
- PGO: "building with PGO improves performance by around 2-14%" (Go 1.22 benchmarks). `default.pgo` in the main package is picked up automatically (`-pgo=auto`, the default since Go 1.21), and should be committed. "Your production environment is the best source of representative profiles." Merge multiple equal-duration profiles with `go tool pprof -proto a.pprof b.pprof > merged.pprof`. Microbenchmarks are poor sources. Refresh profiles regularly, because renames or moves of hot functions degrade matching. — [Go PGO docs](https://go.dev/doc/pgo)
- net/http/pprof registers handlers under `/debug/pprof/` on import as a side effect. With a non-default mux you must register them yourself. Endpoints include profile (CPU, 30s default), trace (1s), heap, allocs, goroutine, block, mutex, threadcreate, cmdline and symbol. The doc example listens on `localhost:6060`. Since Go 1.22 the paths must be requested with GET. — [pkg.go.dev net/http/pprof](https://pkg.go.dev/net/http/pprof)

### Inferences
- Exposure rule: never import `net/http/pprof` into a binary that serves public traffic on `http.DefaultServeMux`. Run a separate admin `http.Server` on an internal port (or localhost behind `kubectl port-forward`) with pprof, metrics and health. `/debug/pprof/cmdline` leaks process arguments.
- Continuous profiling (Grafana Pyroscope through its Go SDK or Grafana Alloy scraping pprof endpoints; Parca through eBPF) supplies the fleet-wide merged profiles PGO wants (not verified in this session).
- Set a CPU limit (or GOMAXPROCS explicitly) for Go pods. Otherwise a 2-CPU-request pod on a 128-core node gets GOMAXPROCS=128.

### Gaps
- Pyroscope and Parca docs were not fetched.
- Didn't confirm whether a GODEBUG setting exists to disable the 1.25 behavior (background knowledge says `containermaxprocs=0` and `updatemaxprocs=0`; not verified in this session).

## Health checks and graceful shutdown

### Takeaway
Liveness should check only the process (no dependencies), readiness can check dependencies and gates traffic, and a startup probe covers slow boot. For zero-downtime rollouts: preStop sleep, then SIGTERM, then fail readiness, then `http.Server.Shutdown` with a deadline below `terminationGracePeriodSeconds`.

### Cited Findings
- Startup probe: "Kubernetes does not execute liveness or readiness probes until the startup probe succeeds." Liveness failure restarts the container. Readiness failure removes the Pod IP from EndpointSlices without restarting it. — [Kubernetes probes](https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/)
- "Liveness probes must be configured carefully to ensure that they truly indicate unrecoverable application failure, for example a deadlock." "Incorrect implementation of liveness probes can lead to cascading failures." The documented pattern has liveness pass when the app itself is healthy, while readiness also checks required back-ends. — [Kubernetes probes](https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/)
- If startup usually takes longer than `initialDelaySeconds + failureThreshold × periodSeconds`, use a startup probe instead of loosening liveness. Pod deletion already marks endpoints not-ready, so a readiness probe isn't strictly needed for draining. — [Kubernetes probes](https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/)
- preStop runs before SIGTERM. A short sleep lets endpoint removal propagate. Kubernetes 1.30+ has a native `sleep` preStop action, so no sleep binary is needed (useful for distroless images). preStop time counts against `terminationGracePeriodSeconds`. — [jorijn.com: Kubernetes graceful shutdown](https://jorijn.com/en/knowledge-base/kubernetes/troubleshooting/kubernetes-graceful-shutdown-sigterm-pod-termination/) (third-party blog)
- Go side: `signal.NotifyContext` for SIGTERM, run `ListenAndServe` in a goroutine and treat `http.ErrServerClosed` as normal, then call `srv.Shutdown(ctx)`, which stops accepting connections and waits for in-flight requests. Set the Shutdown timeout below grace period minus preStop. — [jorijn.com](https://jorijn.com/en/knowledge-base/kubernetes/troubleshooting/kubernetes-graceful-shutdown-sigterm-pod-termination/); [Carlos Becker: k8s pod shutdown lifecycle](https://carlosbecker.com/posts/k8s-pod-shutdown-lifecycle/); [Medium: graceful shutdown for zero downtime](https://medium.com/@yutaroyamanaka4842/understand-how-graceful-shutdown-can-achieve-zero-downtime-during-k8s-rolling-update-ae9edcada5cb)

### Inferences
- Concrete budget example: preStop sleep 5-10s, Shutdown timeout 20s, grace period 30s (the Kubernetes default). After the HTTP drain, stop background workers, flush the OTel exporters (`TracerProvider.Shutdown`, `MeterProvider.Shutdown`, `LoggerProvider.Shutdown`), then close DB pools.
- `http.Server.Shutdown` doesn't wait for hijacked connections (WebSockets). Those need `RegisterOnShutdown` or their own tracking (not verified in this session).
- Readiness that checks the DB can flap every pod at once during a DB blip. Many teams keep readiness to "process initialized and not shutting down" and rely on circuit breakers for dependencies.

### Gaps
- The official Kubernetes pod lifecycle page (termination sequence) wasn't fetched. The preStop guidance rests on third-party blogs.

## Resilience: timeouts, retries, backoff/jitter, circuit breakers, bulkheads, rate limiting, load shedding, idempotency, SRE overload

### Takeaway
Set a timeout on every remote call, derived from downstream latency percentiles. Retry only idempotent operations, at a single layer, with capped exponential backoff, jitter and a retry budget (token bucket or 10% ratio). Add circuit breakers (sony/gobreaker v2), client-side adaptive throttling and criticality-aware load shedding to prevent cascading failure.

### Cited Findings
- Timeouts: pick an acceptable false-timeout rate (e.g. 0.1%) and use the matching downstream percentile (p99.9). Include connection establishment and TLS handshake in the budget. A roughly 20 ms timeout failed because of new TLS connections. — [AWS Builders' Library: Timeouts, retries and backoff with jitter](https://builder.aws.com/content/3EumjoZascWd1oZiEgL8ORlv3qE/timeouts-retries-and-backoff-with-jitter) (moved from aws.amazon.com/builders-library)
- Retry amplification: five layers with three retries each multiply database load 243x, "making it unlikely to ever recover". Best practice is "to retry at a single point in the stack". A local token bucket limits retries, and the AWS SDK has done this since 2016. — [AWS Builders' Library](https://builder.aws.com/content/3EumjoZascWd1oZiEgL8ORlv3qE/timeouts-retries-and-backoff-with-jitter)
- Backoff grows exponentially, with a cap and a maximum attempt count. "Jitter adds some amount of randomness to the backoff to spread the retries around in time." Also jitter periodic jobs, ideally with a consistent per-host offset. "APIs with side effects aren't safe to retry unless they provide idempotency", for example client tokens as in EC2 RunInstances. Retry "only when we observe that the dependency is healthy." — [AWS Builders' Library](https://builder.aws.com/content/3EumjoZascWd1oZiEgL8ORlv3qE/timeouts-retries-and-backoff-with-jitter)
- Google SRE client-side adaptive throttling: track `requests` and `accepts` over 2 minutes and reject locally once requests ≥ K·accepts. K=2 is typical. Lower K is more aggressive (K=1.1 rejects about 1 in 10). — [Google SRE book: Handling Overload](https://sre.google/sre-book/handling-overload/)
- Criticality levels CRITICAL_PLUS, CRITICAL (the default for production), SHEDDABLE_PLUS (batch) and SHEDDABLE propagate downstream. Provision capacity for CRITICAL+. — [SRE book: Handling Overload](https://sre.google/sre-book/handling-overload/)
- Retry budgets: at most 3 attempts per request, and a per-client retry ratio under 10%. That caps worst-case growth at about 1.1X instead of about 3X. Retry only at the layer directly above the rejecting one, and use an "overloaded; don't retry" response. Per-customer quotas may oversubscribe capacity. — [SRE book: Handling Overload](https://sre.google/sre-book/handling-overload/)
- Load shedding: reject based on utilization (CPU, sometimes memory) or executor load average, with lower criticality shed first. A batch proxy tier acts as a fuse. — [SRE book: Handling Overload](https://sre.google/sre-book/handling-overload/)
- sony/gobreaker v2 is generic: `gobreaker.NewCircuitBreaker[T](Settings)` and `cb.Execute(func() (T, error))`. Settings: MaxRequests (half-open probes, 0 means 1), Interval (closed-state count reset), BucketPeriod (rolling window), Timeout (open to half-open, default 60s), ReadyToTrip (default opens after more than 5 consecutive failures), OnStateChange, IsSuccessful, IsExcluded (e.g. ignore context cancellation). A panic counts as failure and is re-panicked. — [sony/gobreaker](https://github.com/sony/gobreaker)

### Inferences
- Go mapping: deadlines through `context.WithTimeout` propagated end to end (gRPC propagates deadlines natively). Always build a custom `http.Client{Timeout: ...}` with a tuned `Transport`, because the default client has no timeout. Bulkheads with semaphores (`golang.org/x/sync/semaphore`) or bounded worker pools per dependency. Per-instance rate limiting with `golang.org/x/time/rate` (token bucket: `rate.NewLimiter(r, burst)`, `Allow`/`Wait`). Distributed limits with Redis (e.g. `go-redis/redis_rate`, GCRA). Backpressure by returning 429/503 with `Retry-After` instead of queueing without bound. Idempotency keys stored with the response (e.g. in Postgres/Redis) and replayed on retry. None of these Go library specifics were verified in this session.
- Feed gobreaker's IsExcluded with `errors.Is(err, context.Canceled)`, so client disconnects don't trip the breaker.
- SLIs/SLOs/error budgets: the SRE book chapters "Service Level Objectives" and "Embracing Risk" (sre.google/sre-book/service-level-objectives/) were not fetched. Background knowledge: an SLO of 99.9% gives a 0.1% error budget, which gates release velocity. Alert on burn rate (SRE Workbook, multi-window multi-burn-rate).

### Gaps
- The SRE "Addressing Cascading Failures" and SLO chapters weren't fetched. The adaptive throttling probability formula is only an image in the source: max(0, (requests − K·accepts)/(requests + 1)), from background knowledge and not verified in this session.
- gobreaker's README as fetched didn't mention a Redis-backed distributed breaker, so it is unconfirmed.

## Configuration: 12-factor and critiques, Go config libraries, validation, secrets

### Takeaway
12-factor remains the baseline and is now an open-source, community-modernized spec (since Nov 2024). In Go, prefer a typed config struct loaded from env (caarlos0/env or envconfig) or koanf for layered sources, validated once at startup with fail-fast. Avoid viper's global, key-lowercasing, dependency-heavy design.

### Cited Findings
- The Twelve-Factor methodology was open-sourced on 2024-11-12 to modernize it as a living, community-maintained document. Heroku says the examples are dated but the core ideas are "remarkably relevant". — [12factor.net: open-source announcement](https://www.12factor.net/blog/open-source-announcement); [12factor blog](https://12factor.net/blog)
- The New Stack (Jan 2025): the methodology "has largely remained the same" while the platform landscape changed a lot. — [The New Stack](https://thenewstack.io/open-source-drives-the-twelve-factor-modernization-project/)
- koanf positions itself as "a cleaner, lighter alternative to spf13/viper". Its criticisms of viper: it "breaks JSON, YAML, TOML, HCL language specs by forcibly lowercasing keys", "significantly bloats build sizes", "pulls a large number of third party dependencies into the core package", "tightly couples config parsing with file extensions", and hardcodes sources. — [knadh/koanf](https://github.com/knadh/koanf)
- koanf separates providers (file, env/v2, posflag, s3, vault/v2, appconfig/v2, etcd/v2, consul/v2, parameterstore/v2, ...) from parsers (json, yaml, toml, dotenv, hcl, ...). Each is a separate module. — [knadh/koanf](https://github.com/knadh/koanf)

### Inferences
- Common critiques of "store config in env" (not sourced in this session): env vars leak into child processes, crash dumps and `/proc/<pid>/environ`, are untyped strings, and handle nested or structured config badly. The modern practice is env for non-secret settings plus mounted secret files or a secret manager (Vault, AWS Secrets Manager, GCP Secret Manager, External Secrets Operator / CSI driver) for credentials.
- Pattern: `type Config struct` with `env:"PORT" envDefault:"8080"` tags (caarlos0/env), a `Validate() error` method (or go-playground/validator) called in `main` before anything starts, and the process exiting non-zero on invalid config so Kubernetes surfaces CrashLoopBackOff. Never log the config struct without `LogValuer` redaction.

### Gaps
- caarlos0/env, kelseyhightower/envconfig and Vault/cloud secret manager docs weren't fetched. Library feature claims are unverified.
- The modernized 12-factor text (if any factors changed by Nov 2026) wasn't checked.

## Security for Go services

### Takeaway
Design against the OWASP API Top 10 (2023), with BOLA, auth, resource consumption and SSRF the most relevant to Go APIs. Run govulncheck in CI (reachability-based, low noise). Use FIPS 140-3 mode only when required (`GOFIPS140` at build time, module v1.0.0 certified as CMVP #5247). Build minimal images (ko produces SBOMs by default).

### Cited Findings
- OWASP API Security Top 10 2023: API1 Broken Object Level Authorization, API2 Broken Authentication, API3 Broken Object Property Level Authorization, API4 Unrestricted Resource Consumption, API5 Broken Function Level Authorization, API6 Unrestricted Access to Sensitive Business Flows, API7 Server Side Request Forgery, API8 Security Misconfiguration, API9 Improper Inventory Management, API10 Unsafe Consumption of APIs. — [OWASP API Security Top 10 2023](https://api-security.owasp.org/editions/2023/en/0x11-t10)
- govulncheck "only surfaces vulnerabilities that actually affect you, based on which functions in your code are transitively calling vulnerable functions". The Go vuln DB (vuln.go.dev, OSV format) is curated by the Go Security team from NVD, GHSA and maintainers. Usage: `go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...`. — [Go vulnerability management](https://go.dev/doc/security/vuln/)
- FIPS 140-3: native since Go 1.24. Build-time `GOFIPS140=off|latest|v1.0.0|v1.26.0|inprocess|certified`. Run-time `GODEBUG=fips140=on|only` (`only` "is not intended for production"). Check with `crypto/fips140.Enabled()`/`Version()`. Module v1.0.0 has CMVP certificate #5247. v1.26.0 (Go 1.26+, adds ML-DSA and a CPU jitter entropy source) was "Pending Review" as of 2026-04-28. In FIPS mode `crypto/rand.Reader` uses an SP 800-90A DRBG and `crypto/tls` refuses non-approved algorithms. Pairwise tests can slow some key generation up to 2x. Go+BoringCrypto is incompatible and slated for removal. "Only enable FIPS mode if you need it." — [Go FIPS 140-3](https://go.dev/doc/security/fips140)
- ko builds Go images with plain `go build`, no Docker needed. It "produces SBOMs by default", does multi-platform builds easily, and fits single Go binaries without cgo or OS packages. — [ko.build](https://ko.build/)
- SSRF (API7) is the risk of fetching user-supplied URIs without validation. Unrestricted resource consumption (API4) covers bandwidth, compute and paid third-party calls. — [OWASP API Top 10 2023](https://api-security.owasp.org/editions/2023/en/0x11-t10)

### Inferences
- Go controls per OWASP item (not verified in this session): authorize every object fetch by owner or tenant in the repository query (API1/API3). Use `http.MaxBytesReader` and `http.Server` `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout`/`MaxHeaderBytes`, plus pagination caps (API4). For SSRF, use a custom `net.Dialer.Control` that rejects private, loopback, link-local and metadata IPs after DNS resolution, which defeats DNS rebinding (API7). Use only `$1`/`?` placeholders in `database/sql`/pgx/sqlc and never `fmt.Sprintf` into SQL. Use `crypto/rand` for tokens (`rand.Text()` exists since Go 1.24 per background knowledge) and never `math/rand`. Compare secrets with `crypto/subtle.ConstantTimeCompare`. Hash passwords with argon2id or bcrypt.
- Static analysis: gosec, plus staticcheck and govulncheck in CI. Supply chain: SLSA provenance and signed artifacts (goreleaser with cosign/Sigstore, ko with SBOM). Images: `CGO_ENABLED=0` static binaries on distroless/static or scratch (scratch needs CA certs and tzdata copied in), run as non-root with a read-only root filesystem.

### Gaps
- gosec, SLSA (slsa.dev), goreleaser signing, distroless and ko default base image docs weren't fetched. Unverified: ko's default base is `cgr.dev/chainguard/static`.
- Couldn't confirm govulncheck binary-scan mode (`-mode=binary`) or the official GitHub Action from the fetched page.

## Deployment and scaling: stateless services, Kubernetes patterns, feature flags

### Takeaway
Scale horizontally with stateless pods: externalize state, give each container an explicit CPU limit (which now drives GOMAXPROCS) and GOMEMLIMIT, and roll out with correct probes and graceful drain. Use OpenFeature's Go SDK (v1.19.0, Go 1.26+) as a vendor-neutral feature-flag API.

### Cited Findings
- OpenFeature Go SDK: implements spec v0.7.0, release v1.19.0, requires Go 1.26 ("only supports currently maintained Go language versions"). Providers are registered with `openfeature.SetProviderAndWait`, and domains or an experimental multi-provider allow several providers. Evaluation context can be set globally, per client or per invocation. Hooks can be global, client or invocation scoped, and include a slog-based `LoggingHook`. Example: `client.Boolean(ctx, "v2_enabled", true, openfeature.EvaluationContext{})`. — [OpenFeature Go SDK](https://openfeature.dev/docs/reference/sdks/server/go/)
- GOMAXPROCS follows CPU limits, not requests, since Go 1.25. — [Go blog](https://go.dev/blog/container-aware-gomaxprocs)
- GOMEMLIMIT suits containers with fixed memory. — [Go GC guide](https://go.dev/doc/gc-guide)
- Readiness and pod deletion remove a pod from EndpointSlices, which lets rolling updates drain traffic. — [Kubernetes probes](https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/)

### Inferences
- Go-specific Kubernetes checklist: requests = limits for CPU in latency-sensitive services (predictable GOMAXPROCS, no CFS throttling surprises); GOMEMLIMIT ≈ 90-95% of the memory limit through the downward API (`resources.limits.memory`); HPA on CPU or RED metrics; PodDisruptionBudget; topology spread; separate admin port for metrics, pprof and health; one process per container.
- State that breaks horizontal scaling in Go apps: in-memory sessions, in-process rate limiters (use Redis for global limits), local file uploads (use object storage), in-memory caches without TTLs or invalidation, cron jobs running on every replica (use leader election or Kubernetes CronJob).
- Use OpenFeature as the API and a provider (flagd, LaunchDarkly, Unleash, etc.) behind it, so flag vendors can be swapped. Pass user or tenant in the evaluation context, and treat flags as config that must be cleaned up.

### Gaps
- Kubernetes HPA/PDB docs and the flagd provider weren't researched. The checklist above is inference.
