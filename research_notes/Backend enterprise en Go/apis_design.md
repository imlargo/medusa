# Building HTTP/RPC APIs in Go and the API design guides large companies publish (as of Nov 2026)

Verification note: claims marked "(fetched)" were read from the primary source in this session (2026-11-01). Claims marked "(not re-verified)" come from the researcher's prior knowledge plus the canonical URL. Treat those as lower confidence; the Gaps sections list them.

## Go HTTP stack: net/http ServeMux, routers/frameworks, middleware, timeouts, body limits, JSON, validation, handler patterns

### Takeaway
Since Go 1.22 the stdlib `http.ServeMux` handles method matching, path wildcards, most-specific-wins precedence and automatic 405 responses, so a third-party router is optional for most services. The Go team still calls third-party routers "a fine choice" for advanced routing. Go 1.27 (Aug 2026) makes `encoding/json/v2` the default implementation behind `encoding/json`. A production server has to set its own timeouts and body limits, because the zero-value or package-level server sets none.

### Cited Findings
**ServeMux (Go 1.22+)** (fetched)
- Patterns can include a method, e.g. `GET /posts/{id}`. GET also matches HEAD; other methods match exactly — [Go blog: Routing Enhancements](https://go.dev/blog/routing-enhancements)
- `{name}` matches one segment, `{name...}` matches the rest of the path, and `/posts/{$}` matches only `/posts/`. Values are read with `Request.PathValue`, and `Request.SetPathValue` lets external routers expose their values the same way — [Go blog](https://go.dev/blog/routing-enhancements)
- When the path matches but the method doesn't, the mux replies 405 with an `Allow` header — [Go blog](https://go.dev/blog/routing-enhancements)
- Precedence: the most specific pattern wins regardless of registration order (`/posts/latest` beats `/posts/{id}`). Overlapping patterns where neither is more specific panic at registration. GODEBUG `httpmuxgo121` restores the old behavior — [Go blog](https://go.dev/blog/routing-enhancements)
- The Go team on routers: "Third-party web frameworks remain a fine choice for current users or programs with advanced routing needs." — [Go blog](https://go.dev/blog/routing-enhancements)
- Mat Ryer's 2024 pattern uses plain `http.NewServeMux()` with `mux.Handle` / `mux.HandleFunc`, not a router — [Grafana blog, Mat Ryer](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/)

**Mat Ryer's handler patterns** (fetched; [Grafana blog](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/))
- `NewServer(deps...) http.Handler` takes every dependency as a typed argument, builds the mux, calls `addRoutes`, wraps the result in top-level middleware (logging, tracing, auth) and returns an `http.Handler`.
- A single `routes.go` lists the whole API surface. Per-route middleware is applied there, so it is visible which routes each one covers.
- `func run(ctx, args, getenv, stdin, stdout, stderr) error` holds the program. `main` only calls `run` and exits non-zero on error. OS dependencies are injected so tests can control them, and context cancellation drives graceful shutdown.
- Handlers are "maker" functions that take dependencies and return an `http.Handler` (usually a `HandlerFunc` closure). State captured in the closure must be read-only or behind a mutex.
- Generic `encode[T]` / `decode[T]` helpers keep JSON handling in one place.
- Validation goes through a `Validator` interface: `Valid(ctx) map[string]string` maps field names to problems, and an empty map means valid. `decodeValid[T]` decodes and then validates. Expensive checks such as DB lookups belong elsewhere.
- Middleware has the shape `func(http.Handler) http.Handler`. When a middleware needs many dependencies, a constructor returns that adapter.
- `sync.Once` defers expensive setup (e.g. parsing templates) to the first request.
- Tests start `run` in a goroutine, cancel it through `t.Cleanup`, and poll `/healthz` for readiness. He prefers end-to-end tests over many isolated unit tests.

**Fiber / fasthttp caveats** (fetched; [gofiber/fiber README](https://github.com/gofiber/fiber))
- Fiber is built on fasthttp, not net/http.
- "Due to Fiber's usage of unsafe, the library may not always be compatible with the latest Go version." Fiber v3 is tested with Go 1.26 or higher.
- Values from the context "are not immutable by default and will be reused across requests". Handlers must copy anything they keep after returning.
- net/http handlers can be mounted, but they "don't get access to fiber.Ctx features and incur the overhead of the compatibility layer."
- Current major line is v3 (`github.com/gofiber/fiber/v3`), with a migration CLI from v2.

**Huma** (fetched; [huma.rocks](https://huma.rocks/))
- A Go micro framework for HTTP REST/RPC APIs. The current module is `huma/v2`. It brings your own router, generates OpenAPI 3.1 and JSON Schema, and validates requests from that schema. It also offers conditional requests, auto-PATCH, SSE, streaming, a service CLI, generated docs and client SDKs.
- The project describes itself as neither API-first nor code-first but "Huma-first": you write Go input/output types and Huma generates the spec — [Applied Go spotlight](https://appliedgo.net/spotlight/4-tools-for-generating-rest-apis/)

**chi / echo / gin** (not re-verified)
- chi is a lightweight router built on net/http types (`http.Handler`, `func(http.Handler) http.Handler` middleware) — [go-chi/chi](https://github.com/go-chi/chi)
- echo and gin use their own context types (`echo.Context`, `*gin.Context`) instead of plain `http.Handler`, which makes them less interchangeable with stdlib middleware — [labstack/echo](https://github.com/labstack/echo), [gin-gonic/gin](https://github.com/gin-gonic/gin)

**http.Server timeouts**
- Cloudflare's "complete guide" (fetched) dates from June 29, 2016 and targets Go 1.6/1.7. It predates `ReadHeaderTimeout` and `IdleTimeout` and does not cover them — [Cloudflare blog](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/)
- In that guide, `ReadTimeout` runs from Accept to the end of the body read (or the end of headers if the body isn't read). `WriteTimeout` normally runs from the end of the header read to the end of the response write; for HTTPS it starts at Accept and also covers the TLS handshake — [Cloudflare](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/)
- `http.ListenAndServe`, `http.ListenAndServeTLS` and `http.Serve` "leave the Timeouts to their default off value, with no way of enabling them". Connections can then leak until file descriptors run out. The guide recommends an explicit `http.Server` with timeouts for public servers — [Cloudflare](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/)
- `http.TimeoutHandler` wraps a handler, buffers its response and returns 504 when the deadline passes. Streaming servers often have to unset `WriteTimeout` — [Cloudflare](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/)
- On the client side, `http.Get` and the default client have no timeout. Set `http.Client.Timeout`, or the transport-level timeouts (`Dialer.Timeout`, `TLSHandshakeTimeout`, `ResponseHeaderTimeout`, `ExpectContinueTimeout`) — [Cloudflare](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/)
- Go 1.27 adds `Server.MaxHeaderValueCount`, with `DefaultMaxHeaderValueCount` used when unset. It also adds HTTP/2 client-priority support (RFC 9218), opt-out via `Server.DisableClientPriority`; automatic draining of HTTP/1 `Response.Body` on close; and `httptest.NewTestServer` for use with `testing/synctest` — [Go 1.27 release notes](https://go.dev/doc/go1.27) (fetched)
- `ReadHeaderTimeout` (Go 1.8+) bounds the time to read request headers and is the standard Slowloris mitigation; linters such as gosec flag servers without it (not re-verified) — [pkg.go.dev net/http Server](https://pkg.go.dev/net/http#Server)

**MaxBytesReader** (fetched; [pkg.go.dev net/http](https://pkg.go.dev/net/http#MaxBytesReader))
- Works like `io.LimitReader` but is meant for request bodies. It returns a `ReadCloser`, returns `*MaxBytesError` when the limit is exceeded, and tells the ResponseWriter to close the connection when possible. Its purpose is to stop clients from sending oversized requests that waste server resources.

**JSON: encoding/json/v2 status** (fetched)
- Go 1.25 added `encoding/json/v2` and `encoding/json/jsontext` behind `GOEXPERIMENT=jsonv2`. With the experiment on, v1 runs on the new implementation and only error text may change. Decoding is "substantially faster"; encoding is at parity — [Go 1.25 release notes](https://go.dev/doc/go1.25)
- Go 1.26 still kept v2 behind `GOEXPERIMENT=jsonv2` — [golang/go#79788](https://github.com/golang/go/issues/79788); secondary: [huizhou92 blog](https://huizhou92.com/go-1-26-shipped-without-json-v2-heres-the-real-reason-why/)
- Go 1.27 (August 2026) ships `encoding/json/v2`. v1 `encoding/json` "is now backed by the v2 implementation". v2 defaults are stricter: it rejects invalid UTF-8 and duplicate object names. The opt-out `GOEXPERIMENT=nojsonv2` "is expected to be removed in a future release". Late API changes: the `format` and `unknown` tag options, `DiscardUnknownMembers` and `SkipFunc` were removed, and `inline` was renamed to `embed` — [Go 1.27 release notes](https://go.dev/doc/go1.27)
- Under `nojsonv2`, any package that imports v2 or jsontext fails to compile. The jsonv2 working group closed a proposal to change that as "not planned" (June 2026), saying the flag is a short-term workaround — [golang/go#79788](https://github.com/golang/go/issues/79788)
- Conflict: the Go 1.27 notes page is written in future tense ("arrives in August 2026"), but the date is past and pkg.go.dev lists `go1.26.7` patch releases — [pkg.go.dev](https://pkg.go.dev/encoding/json@go1.26.7). Most likely 1.27 is released and the notes keep the template wording. Confirm on go.dev/dl.

**Validation**
- Mat Ryer validates by hand through `Valid(ctx) map[string]string` — [Grafana blog](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/). Huma validates from JSON Schema derived from the Go types — [huma.rocks](https://huma.rocks/). go-playground/validator uses struct-tag rules (`validate:"required,email"`) (not re-verified) — [go-playground/validator](https://github.com/go-playground/validator)

### Inferences
- The default for a new service can be stdlib `ServeMux` + `func(http.Handler) http.Handler` middleware + an explicit `http.Server`. A sensible setup sets ReadHeaderTimeout, ReadTimeout, WriteTimeout and IdleTimeout, wraps bodies in `MaxBytesReader`, and handles shutdown via context. chi is the lowest-friction upgrade if route groups or sub-routers are needed, because it keeps net/http types.
- Fiber's object reuse and `unsafe` caveats make it a poor fit for an "enterprise" codebase that values stdlib compatibility.
- Teams on Go 1.27 should test against v2's stricter defaults (duplicate keys, invalid UTF-8). Code that adopted pre-release v2 tag options (`format`, `unknown`, `inline`) has to be updated.

### Gaps
- I did not fetch the `Server` field docs for ReadHeaderTimeout/IdleTimeout (the page was truncated). No updated, authoritative timeout guide newer than Cloudflare's 2016 post was found.
- No primary-source benchmark or maintainer statement for chi/echo/gin was fetched this session.
- go-playground/validator vs hand validation: no authoritative comparison source found.

## RPC: gRPC-Go, Connect, buf, grpc-gateway

### Takeaway
Connect (connectrpc.com, from Buf, a CNCF sandbox project) serves gRPC, gRPC-Web and its own curl-able HTTP/JSON protocol from one handler on top of `net/http`, so it slots into a stdlib server. `buf breaking` catches schema breaks in CI against git or the BSR, with FILE as the strictest and default category.

### Cited Findings
- Connect generates handlers and type-safe clients from Protobuf. Servers accept gRPC, gRPC-Web and the Connect protocol by default, and clients default to Connect — [connectrpc.com intro](https://connectrpc.com/docs/introduction) (fetched)
- It is "just Protocol Buffers and `net/http`". The Connect protocol runs over HTTP/1.1, HTTP/2 and HTTP/3, supports JSON or binary Protobuf, and can be called with plain cURL — [connectrpc.com](https://connectrpc.com/docs/introduction)
- The Go and ECMAScript implementations are stable and used in production. Swift is stable, Kotlin and Python are beta, Rust is pre-1.0. It is a CNCF sandbox project created by Buf — [connectrpc.com](https://connectrpc.com/docs/introduction)
- `buf breaking` compares the current schema against a BSR module, git ref (`--against '.git#branch=main'`), tarball or image. Categories from strictest to loosest are FILE (the default), PACKAGE, WIRE_JSON and WIRE — [buf docs: breaking](https://buf.build/docs/breaking/) (fetched)
- gRPC-Go (`google.golang.org/grpc`) is the reference gRPC implementation and uses its own HTTP/2 server rather than net/http handlers (not re-verified) — [grpc/grpc-go](https://github.com/grpc/grpc-go)
- grpc-gateway generates a reverse proxy that maps REST/JSON onto gRPC using `google.api.http` annotations, and can emit OpenAPI (not re-verified) — [grpc-ecosystem/grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway)
- Google's AIPs are written in Protobuf terms (`request_id`, `page_token`, `google.rpc.Status`), so a proto-first API can follow them directly — [AIP-158](https://google.aip.dev/158), [AIP-193](https://google.aip.dev/193)

### Inferences
- For a Go backend that needs both browser/JSON clients and service-to-service RPC, Connect avoids running a separate gateway process the way grpc-gateway does. Use grpc-gateway when the requirement is resource-style REST URLs (`GET /v1/things/{id}`) derived from gRPC, not Connect's POST-based RPC URLs.
- `buf lint` + `buf breaking --against` the main branch is the proto equivalent of an OpenAPI diff check in CI.

### Gaps
- The connectrpc intro page does not compare Connect with grpc-go. Performance and interceptor-ecosystem comparisons were not sourced.
- I did not fetch the `buf lint` STANDARD rule set or the grpc-gateway docs.

## OpenAPI in Go: oapi-codegen, ogen, huma, swaggo; spec-first vs code-first

### Takeaway
The production-grade spec-first options are oapi-codegen (most used, generates for chi/echo/gin/fiber/stdlib) and ogen (stricter, reflection-free). Huma is the main code-first option and emits OpenAPI 3.1. For 3.1, stable oapi-codegen support was not confirmed; its experimental successor parses 3.1/3.2.

### Cited Findings
- A GitHub discussion calls oapi-codegen and ogen the only two "production ready" spec-first generators, and notes oapi-codegen has more users and stars — [oapi-codegen discussion #1027](https://github.com/oapi-codegen/oapi-codegen/discussions/1027) (community opinion)
- oapi-codegen supports server packages such as Chi, Echo, Fiber and Gin. Its experimental successor `oapi-codegen-exp` uses libopenapi to parse OpenAPI 3.1 and 3.2 — [Applied Go spotlight](https://appliedgo.net/spotlight/4-tools-for-generating-rest-apis/), [oapi-codegen-exp listing](https://awesome.ecosyste.ms/projects/github.com%2Foapi-codegen%2Foapi-codegen-exp)
- ogen generates code without reflection or `interface{}`. A January 2026 comparison says it enforces the spec more strictly, especially responses, while oapi-codegen fits better when you rely on familiar routers and middleware — [zenn.dev comparison](https://zenn.dev/otakakot/articles/43653194611d42), [ogen](https://github.com/ogen-go/ogen)
- Huma: write Go types and get OpenAPI 3.1 + JSON Schema, with bring-your-own-router — [huma.rocks](https://huma.rocks/)
- Zalando rule #100 "MUST follow API first principle" and #101 "MUST provide API Specifications using OpenAPI" — [Zalando guidelines](https://opensource.zalando.com/restful-api-guidelines/) (fetched)
- swaggo/swag builds a Swagger 2.0 spec from comment annotations (not re-verified). The usual criticism is that annotations drift from the code, nothing type-checks them, and the tool targets Swagger 2.0 rather than OpenAPI 3.x — [swaggo/swag](https://github.com/swaggo/swag)

### Inferences
- Spec-first (oapi-codegen strict server or ogen) fits contract-driven, multi-team "enterprise" work and Zalando-style API-first review. Code-first with Huma gives one source of truth without comment annotations. Annotation-based swag is the weakest option, because the spec isn't derived from types.

### Gaps
- Stable oapi-codegen OpenAPI 3.1 support was not confirmed. swag's current OpenAPI 3 status (v2 branch) was not verified, and no primary-source critique of annotation-based swag was fetched.

## API design guides: concrete rules (resources, naming, errors, pagination, filtering, versioning, idempotency, LRO, rate limits)

### Takeaway
The major guides agree on these points: plural noun collections, no breaking changes, opaque cursor/token pagination, machine-readable error codes separate from human messages, and client-supplied idempotency keys for POST. They differ on casing (Zalando snake_case, Microsoft camelCase), on the error envelope (RFC 9457 problem+json vs Google's `google.rpc.Status` vs Microsoft's `{"error":{code,message,...}}`), and on versioning (Microsoft dated `api-version` query param, Stripe date-named releases, Zalando no URL versioning, Google `v1` in the path).

### Cited Findings
**Errors**
- RFC 9457 (July 2023, Standards Track) obsoletes RFC 7807. The media type is `application/problem+json`, with members `type` (URI, the primary identifier), `title` (stable per type), `status` (advisory, must match the actual status), `detail` (this occurrence) and `instance` (URI). Extension members are allowed and clients must ignore unknown ones. A missing `type` means `about:blank`. For several different problems, return "the most relevant or urgent problem" — [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html) (fetched)
- Google AIP-193: every error returns `google.rpc.Status` with a canonical code and must include `ErrorInfo`. `reason` is UPPER_SNAKE_CASE (≤63 chars) and `domain` is globally unique, usually the service name. A (reason, domain) pair is stable and never reused for a different error. Dynamic values in `message` must also be in `ErrorInfo.metadata`, so clients never parse messages — [AIP-193](https://google.aip.dev/193) (fetched)
- AIP-193 HTTP/JSON shape: top-level `error` with `code` (HTTP integer), `message`, `status` (e.g. "NOT_FOUND") and `details[]` entries carrying `@type`. `LocalizedMessage` carries a BCP-47 locale (default en-US), and `Help` carries links. Avoid partial errors; for bulk operations use LROs. Check permission before existence: return 403 PERMISSION_DENIED whether or not the resource exists, and 404 only when the caller has permission — [AIP-193](https://google.aip.dev/193)
- Microsoft (Azure): body `{"error": ErrorDetail}` with `code` and `message` required, plus optional `target`, `details[]` and `innererror`. The `x-ms-error-code` header must equal `error.code`. Error codes are part of the contract, so adding new top-level codes requires a version bump. Unknown fields or values get 400 — [Microsoft Azure REST API Guidelines](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md) (fetched)
- Zalando: #176 "MUST support problem JSON", #177 "MUST not expose stack traces", #151 "MUST specify success and error responses" — [Zalando](https://opensource.zalando.com/restful-api-guidelines/)

**Naming and resources**
- Zalando: #118 snake_case properties, #130 snake_case query params, #129 kebab-case path segments, #134 pluralize resource names, #141 verb-free URLs, #122 no null booleans, #123 null and absent mean the same — [Zalando](https://opensource.zalando.com/restful-api-guidelines/)
- Microsoft: camelCase JSON fields without uppercased acronyms, kebab-case path segments (camelCase if the segment maps to a JSON field), camelCase query params, kebab-case headers without new `x-` prefixes, unabbreviated plural collection names. Never send null-valued fields in responses — [Microsoft](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md)
- Microsoft PATCH uses JSON Merge Patch (RFC 7396, `application/merge-patch+json`): null deletes a field, and the call returns 200 on update or 201 on create — [Microsoft](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md)

**Status codes**
- Zalando: #243 official codes only, #220 the most specific code, #150 prefer common codes, #152 207 for batch/bulk, #153 429 with headers for rate limits, #251 avoid redirects — [Zalando](https://opensource.zalando.com/restful-api-guidelines/)
- Microsoft conditional requests: honor `If-Match`/`If-None-Match`. Return 304 on a matching GET and 412 on a stale `If-Match` for PUT/PATCH/DELETE. Prefer content-hash ETags over version numbers, because version-based ETags make retried conditional updates look like conflicts — [Microsoft](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md)

**Pagination and filtering**
- AIP-158: request fields are `page_size` and `page_token`. `page_size` 0 or unset means a documented default, values above the max are coerced down, and negatives are `INVALID_ARGUMENT`. A changed non-size parameter with a token should be `INVALID_ARGUMENT`. `next_page_token` is empty only at the end, which is the only end signal. `total_size` is optional and may be an estimate. Tokens must be opaque (base64 of readable data doesn't count), never grant authorization, and may expire after about 3 days. Optional `skip` skips resources. Paginate from day one, because adding it later is breaking — [AIP-158](https://google.aip.dev/158) (fetched)
- Microsoft: list responses are `{"value": [...], "nextLink": "<absolute URL incl. api-version>"}`. Omit `nextLink` on the last page rather than sending null, and document that items may be skipped or duplicated across pages. Query options are `filter`, `orderby`, `skip`, `top`, `maxpagesize`, `select` and `expand`, with no `$` prefix. Return an error for unsupported params — [Microsoft](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md)
- Zalando: #159 MUST paginate, #160 SHOULD prefer cursor-based over offset-based, #161 SHOULD use pagination links, #254 SHOULD avoid total counts — [Zalando](https://opensource.zalando.com/restful-api-guidelines/)

**Versioning and compatibility**
- Microsoft: `api-version=YYYY-MM-DD[-preview]` is required on every call, including `nextLink`/`operation-location`. Missing gets 400 `MissingApiVersionParameter`; unknown gets 400 `UnsupportedApiVersionValue`. No version segment in the path, no breaking changes, previews last at most one year, and GA must use a later date than its preview — [Microsoft](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md)
- Zalando: #106 MUST not break backward compatibility, #108 clients must tolerate compatible extensions, #113 SHOULD avoid versioning, #114 MUST use media-type versioning, #115 MUST NOT use URL versioning, #116 semantic versioning — [Zalando](https://opensource.zalando.com/restful-api-guidelines/)
- Stripe (2017 design): rolling date-named versions (e.g. `2017-05-24`). An account is pinned to the latest version at its first request, and the `Stripe-Version` header overrides per request. Each breaking change is a "version change module" that transforms responses backward from current to the target version. The changelog is generated from those modules. Breaking changes include removing or renaming fields and changing types; adding endpoints or fields is safe — [Stripe blog: APIs as infrastructure](https://stripe.com/blog/api-versioning) (fetched)
- Stripe today: since `2024-09-30.acacia`, monthly releases carry no breaking changes, and twice a year a named major release (e.g. Basil) begins with breaking changes. Current version: `2026-10-28.endive`. The account default is set in Workbench, `Stripe-Version` overrides it, and strongly typed SDKs (stripe-go, stripe-java) pin the version current at SDK release — [Stripe docs: Versioning](https://docs.stripe.com/api/versioning) (fetched)

**Idempotency**
- Stripe (API v1): `Idempotency-Key` header, up to 255 chars, a V4 UUID suggested, no sensitive data. Stripe saves the status and body of the first request once execution begins, and replays them, 500s included. Keys can be pruned after 24 hours. Reusing a key with different parameters returns an error. Validation failures and concurrent conflicts are not saved, so they can be retried. All POSTs accept keys; GET/DELETE don't need them. API v2 instead retries failed work to completion — [Stripe docs: Idempotent requests](https://docs.stripe.com/api/idempotent_requests) (fetched)
- AIP-155: optional `string request_id` on the request message (never on the resource). It should accept UUIDs and may require them (annotated `format = UUID4`). Supplying it must guarantee idempotency. On a duplicate, return the earlier response, or the current resource state if that's impossible. The retention window is left to the API — [AIP-155](https://google.aip.dev/155) (fetched)
- Microsoft: all methods must be idempotent. POST should support OASIS Repeatable Requests headers (`Repeatability-Request-ID`, `Repeatability-First-Sent`, `Repeatability-Result`) with at least a 5-minute window. Operations that don't support them return 501 when the headers are sent — [Microsoft](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md)
- Zalando: #229 SHOULD design POST/PATCH idempotent, #231 SHOULD use secondary keys for idempotent POST, #230 MAY support `Idempotency-Key` — [Zalando](https://opensource.zalando.com/restful-api-guidelines/)

**Long-running operations**
- Microsoft: use an LRO when p99 latency exceeds 1 s. The initial response carries an `operation-location` header with an absolute status-monitor URL: PUT returns 201 with `operation-id`, DELETE returns 202, and a POST action returns 202 with a status body. No PATCH LROs and no LRO POST to create resources. The monitor has `id`, `status` (NotStarted/Running/Succeeded/Failed/Canceled), `error` and `result`. Polling returns 200 with `retry-after` until the operation is terminal. Keep monitors at least 24 h. A reused `Operation-Id` gets 409 unless the request is an identical retry — [Microsoft](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md)
- AIP-193 points bulk and partial-failure operations to LROs — [AIP-193](https://google.aip.dev/193). The dedicated LRO AIP (AIP-151, `google.longrunning.Operation`) was not fetched — [AIP-151](https://google.aip.dev/151) (not re-verified)

**Rate-limit headers**
- IETF `draft-ietf-httpapi-ratelimit-headers-11` (23 May 2026, expires 24 Nov 2026) is an active HTTPAPI WG draft on the Standards Track, not yet an RFC. It defines two structured-field headers. `RateLimit-Policy: "burst";q=100;w=60` declares quota `q` (required), unit `qu` (default "requests"), window `w` in seconds, and partition key `pk`. `RateLimit: "default";r=50;t=30` reports remaining quota `r` (required) and seconds `t` until reset. Neither header may appear in trailers — [IETF datatracker](https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/) (fetched)
- Zalando #153 requires 429 plus headers — [Zalando](https://opensource.zalando.com/restful-api-guidelines/). The fetched portion did not name which headers.

### Inferences
- A Go API that wants RFC compliance with Google-style machine-readable codes could return `application/problem+json` with a stable `type` URI plus an extension member such as `code`/`reason` (AIP-193 ErrorInfo semantics). RFC 9457 explicitly allows extensions.
- The guides converge on opaque cursor tokens, with `next_page_token` / `nextLink` absence as the end signal and no mandatory total count. Offset is at most an optional extra (AIP `skip`, Microsoft `skip`).
- The draft RateLimit header syntax changed substantially from older drafts and the `X-RateLimit-*` conventions. Implementations should track draft-11 or later and expect possible further changes before RFC.

### Gaps
- JSON:API (jsonapi.org) was not fetched; no rules from it are included.
- AIP-121/131–135 (resource-oriented design, standard methods), AIP-151 (LRO), AIP-160 (filtering) and AIP-185 (versioning) were not fetched.
- I found no "general" (non-Azure) Microsoft guideline in the fetched page. The repo's vNext branch centers on the Azure guidelines and older general guidelines are archived (not verified).
- The Zalando page was truncated after 100k chars, so only rule titles were captured, not bodies (e.g. the exact headers for #153 and the Idempotency-Key semantics for #230).

## AuthN/AuthZ in Go: JWT, OIDC, sessions, authorization engines, CSRF/CORS

### Takeaway
Go 1.25's `http.CrossOriginProtection` provides token-free CSRF protection in the stdlib, based on `Sec-Fetch-Site` and `Origin`. It does not replace CORS: CSRF is about accepting state-changing requests, CORS about reading responses. The other library-level recommendations in this section were not re-verified this session.

### Cited Findings
- `CrossOriginProtection` rejects non-safe cross-origin browser requests. It uses `Sec-Fetch-Site`, or compares the `Origin` host with `Host`. GET/HEAD/OPTIONS are always allowed. Requests without either header are treated as same-origin or non-browser and allowed. The zero value is usable, and `NewCrossOriginProtection()`, `AddTrustedOrigin`, `AddInsecureBypassPattern` (ServeMux pattern syntax; panics on conflicts), `Check`, `SetDenyHandler` and `Handler(h)` are available — [pkg.go.dev net/http](https://pkg.go.dev/net/http#CrossOriginProtection), [Go 1.25 release notes](https://go.dev/doc/go1.25) (fetched)
- Filippo Valsorda's algorithm (the one implemented in Go 1.25), in order:
  1. Allow safe methods.
  2. Allow an allow-listed Origin.
  3. If `Sec-Fetch-Site` is present, allow only `same-origin` or `none`.
  4. If neither header is present, allow.
  5. Otherwise require the Origin host:port to equal Host.

  — [Filippo Valsorda: CSRF](https://words.filippo.io/csrf/) (fetched)
- Caveats from the same post: `Sec-Fetch-Site` is sent only to trustworthy origins (HTTPS/localhost). Same-site requests are rejected because subdomains may be less trusted. The Host fallback breaks behind proxies that rewrite Host (fix by allow-listing the origin) and is scheme-agnostic, so use HSTS. GET handlers must not change state. Sec-Fetch-Site has been in all major browsers since 2023 — [Filippo Valsorda](https://words.filippo.io/csrf/)
- CORS controls whether a cross-origin page can read responses; CSRF is about state-changing requests the attacker can't read. Preflight is weak protection, because forms produce simple requests — [Filippo Valsorda](https://words.filippo.io/csrf/)
- Huma's docs include OAuth 2.0 & JWT guidance — [huma.rocks](https://huma.rocks/)
- Libraries (not re-verified; descriptions from prior knowledge):
  - JWT: `github.com/golang-jwt/jwt/v5`, the community successor to dgrijalva/jwt-go — [golang-jwt/jwt](https://github.com/golang-jwt/jwt)
  - OIDC client/verification: `github.com/coreos/go-oidc/v3` with `golang.org/x/oauth2` — [coreos/go-oidc](https://github.com/coreos/go-oidc)
  - Policy engines: OPA/Rego, embeddable in Go — [open-policy-agent/opa](https://github.com/open-policy-agent/opa); Casbin, an in-process ACL/RBAC/ABAC library — [casbin/casbin](https://github.com/casbin/casbin)
  - Zanzibar-style relationship-based authorization (ReBAC) services: OpenFGA (CNCF) — [openfga/openfga](https://github.com/openfga/openfga); SpiceDB — [authzed/spicedb](https://github.com/authzed/spicedb)
  - Server-side sessions: alexedwards/scs — [alexedwards/scs](https://github.com/alexedwards/scs)
  - CORS middleware: rs/cors — [rs/cors](https://github.com/rs/cors)

### Inferences
- For cookie-session browser apps on Go ≥1.25, wrapping the mux in `http.NewCrossOriginProtection().Handler(mux)` replaces gorilla/csrf-style tokens, provided GETs are side-effect-free and the site is HTTPS. Bearer-token-only APIs aren't CSRF-exposed in the same way, but still need a strict CORS allow-list if browsers call them.
- Rough split for authorization engines: Casbin or in-code checks for simple RBAC in one service, OPA for policy-as-code across services and infrastructure, and OpenFGA/SpiceDB for fine-grained per-object sharing models.

### Gaps
- None of the AuthN/AuthZ library repos or docs were fetched this session (tool-call budget), so current versions, maintenance status and CNCF levels for golang-jwt, go-oidc, OPA, Casbin, OpenFGA and SpiceDB are unverified.
- No primary source was found on JWT vs opaque session tokens (e.g. revocation trade-offs), and none on gorilla/csrf's status after Go 1.25.
