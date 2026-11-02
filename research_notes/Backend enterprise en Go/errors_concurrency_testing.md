# Error handling, concurrency/context and testing in production Go backends (as of Nov 2026)

Verification note: items marked **(fetched)** were read from the primary source during this research session (1 Nov 2026). Items marked **(not re-fetched)** come from well-known primary docs whose URLs are given, but the exact wording was not re-read in this session. The report writer should treat those as high-confidence but unverified wording.

Go release context: Go 1.24 (Feb 2025), Go 1.25 (Aug 2025), Go 1.26 (Feb 2026), Go 1.27 (planned Aug 2026, release notes published; the page does not state an exact ship day) — [Go 1.26 notes](https://go.dev/doc/go1.26), [Go 1.27 notes](https://go.dev/doc/go1.27) (fetched).

## Errors: wrapping, inspection, error kinds, handle-once, boundary translation, stack traces, panics, and the 2025 syntax decision

### Takeaway
The consensus (Go team, Google, Uber, Cheney) is: return errors, add context only when it adds new information (`fmt.Errorf("op: %w", err)`, `%w` at the end), make errors inspectable only when callers really need to branch (sentinel for static, typed for data-carrying, `errors.Is`/`errors.As`/`errors.AsType`), handle each error exactly once, translate to a canonical space (HTTP/gRPC status) at system boundaries with `%v` or explicit mapping, and never panic for ordinary errors. In June 2025 the Go team formally stopped pursuing new error-handling syntax, so `if err != nil` is the long-term idiom.

### Cited Findings

**Wrapping and inspection**
- Uber's decision table: no matching needed + static message → `errors.New`; no matching + dynamic → `fmt.Errorf`; callers must match + static → top-level `var ErrX = errors.New(...)` matched with `errors.Is`; callers must match + dynamic → custom error type matched with `errors.As`. Exported errors become part of the public API. (fetched) — [Uber Go Style Guide: Error Types](https://github.com/uber-go/guide/blob/master/style.md)
- Uber: return the original error if you have nothing to add; use `%w` if callers should be able to match the underlying error, `%v` to hide it (you can switch to `%w` later); keep context short and avoid "failed to" (`"new store: %w"`, not `"failed to create new store: %w"`). (fetched) — [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- Naming: exported error vars use `Err` prefix (`ErrNotFound`), unexported `err`; custom error types use `Error` suffix (`NotFoundError`). (fetched) — [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- Google: "Do not attempt to distinguish errors based on their string form"; use sentinels, `errors.Is`/`errors.As`, or structured types like `os.PathError`. Don't repeat information the underlying error already has (e.g. file path from `os.Open`); don't annotate just to say something failed. (fetched) — [Google Go Style: Best Practices, Error handling](https://google.github.io/styleguide/go/best-practices)
- Google: "Prefer to place `%w` at the end of an error string" so text mirrors the chain; exception: with sentinel errors, place the category first. (fetched) — [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices)
- Google on `%v` vs `%w`: use `%v` to annotate for humans or deliberately hide internals, "especially useful at system boundaries (RPC, IPC, storage)" where errors should be translated into a canonical error space; use `%w` when you intend callers to unwrap and you document that as API. (fetched) — [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices)
- Google decisions: error strings are not capitalized and have no trailing punctuation; do not discard errors with `_`; prefer an extra return value over in-band sentinel values like `-1`. (fetched) — [Google Go Style Decisions](https://google.github.io/styleguide/go/decisions)
- `errors.AsType` (Go 1.26): new generic version of `errors.As`, "type-safe, faster, and, in most cases, easier to use" (idiom: `if pe, ok := errors.AsType[*fs.PathError](err); ok { ... }`). (fetched) — [Go 1.26 release notes](https://go.dev/doc/go1.26)
- Go 1.26: `fmt.Errorf("x")` without formatting now allocates about as little as `errors.New("x")`, removing a micro-optimization argument. (fetched) — [Go 1.26 release notes](https://go.dev/doc/go1.26)
- Go 1.27 vet: the printf analyzer flags `fmt.Errorf("...: %w", p)` where `p` is `*E` but the value type `E` implements `error`; this is a latent bug because `errors.Is`/`As` checks against `E` fail. (fetched) — [Go 1.27 release notes](https://go.dev/doc/go1.27)
- `errors.Join` (Go 1.20) combines multiple errors; `errors.Is`/`As` inspect each; `fmt.Errorf` supports multiple `%w` verbs. Typical uses: aggregating validation errors, combining cleanup errors (`err = errors.Join(err, f.Close())`). (not re-fetched) — [Go 1.20 release notes](https://go.dev/doc/go1.20), [pkg.go.dev/errors](https://pkg.go.dev/errors)
- Original wrapping design (Go 1.13): `Unwrap`, `errors.Is`, `errors.As`, `%w`; the blog warns that wrapping exposes the wrapped error as part of your API contract. (not re-fetched) — [Go blog: Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)

**Sentinel vs typed vs opaque (Dave Cheney, 2016)**
- Cheney's three strategies: sentinel errors (`io.EOF`, compared with `==`) create API surface and package coupling and break once context is added → avoid; error types (`os.PathError`) must be public and couple callers → avoid or keep out of public API; opaque errors (caller only knows success/failure) are the most flexible. (fetched) — [Dave Cheney: Don't just check errors, handle them gracefully](https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully)
- "Assert errors for behaviour, not type": define a small interface such as `interface{ Temporary() bool }` and an `IsTemporary(err)` helper, so callers don't import the producing package. (fetched) — [Dave Cheney](https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully)
- Note on dating: Cheney's `==` comparison critique predates Go 1.13; with `errors.Is`, wrapped sentinels still match, which weakens his "context breaks equality" argument, but the coupling/API-surface critique still stands. (inference from the two sources above)

**Handle once**
- Cheney: handling an error means making a decision; logging and then returning is two decisions and produces duplicate logs while the top caller still lacks context. (fetched) — [Dave Cheney](https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully)
- Uber "Handle Errors Once": pick one of: wrap and return; log and degrade gracefully (non-essential operation); match a specific callee error, handle it, and wrap/return the rest. (fetched) — [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- Google: "If you return an error, it's usually better not to log it yourself"; mind PII and the cost of `log.Error`. (fetched) — [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices)

**Boundary translation (domain → HTTP/gRPC)**
- Google explicitly recommends translating to a canonical error space at RPC/IPC/storage boundaries, with `%v` to avoid leaking internals. (fetched) — [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices)
- Common idiom (implementation, not a single cited rule): domain package defines `ErrNotFound`, `ErrConflict`, or a typed `*ValidationError`; a single HTTP error-mapping function does `switch { case errors.Is(err, domain.ErrNotFound): 404 ... default: log once + 500 }`; for gRPC, map to `status.Error(codes.NotFound, ...)` from `google.golang.org/grpc/status`. (not re-fetched) — [grpc-go status package](https://pkg.go.dev/google.golang.org/grpc/status), [grpc codes](https://pkg.go.dev/google.golang.org/grpc/codes)

**Stack traces debate**
- `github.com/pkg/errors` was archived by its owner on 1 Dec 2021 and is read-only. (search snippet, GitHub page) — [pkg/errors issues](https://github.com/pkg/errors/issues)
- `cockroachdb/errors` positions itself as a near drop-in replacement for stdlib errors with stack traces (plus network-portable errors, PII-safe details); it still depends on pkg/errors for stack formatting per its issue tracker; some users find it heavy. — [cockroachdb/errors on pkg.go.dev](https://pkg.go.dev/github.com/cockroachdb/errors@v1.1.0), [cockroachdb/errors issue #70](https://www.github.com/cockroachdb/errors/issues/70), [golang-nuts thread](https://groups.google.com/g/golang-nuts/c/0-cCCFnuap4)
- Alternative stack-carrying package created after pkg/errors was archived: `gitlab.com/tozd/go/errors`. — [tozd/go/errors](https://beta.pkg.go.dev/gitlab.com/tozd/go/errors@v0.4.1)
- Stdlib position: the standard `errors` package intentionally carries no stack traces; mainstream style guides (Google, Uber) rely on well-written wrap messages instead and do not recommend stack-trace libraries. (inference from the absence of any such recommendation in [Uber](https://github.com/uber-go/guide/blob/master/style.md) and [Google](https://google.github.io/styleguide/go/best-practices) guides)

**Panics and recover**
- Uber: production code returns errors; panic only for irrecoverable conditions (e.g. init failure); tests use `t.Fatal`/`t.FailNow`. Call `os.Exit`/`log.Fatal` only in `main`, at most once; put logic in `run() error`. (fetched) — [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- Google: "Do not use `panic` for normal error handling"; libraries return errors; "Don't recover panics to avoid crashes"; internal panic/recover is acceptable only if "these panics are never allowed to escape across package boundaries"; propagate init errors to `main` and exit with an actionable message. (fetched) — [Google Go Decisions](https://google.github.io/styleguide/go/decisions), [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices)
- Practical note: `net/http` recovers panics per request in the server goroutine, but a panic in any goroutine you spawn crashes the process, so recovery middleware does not protect background goroutines. (not re-fetched) — [net/http Handler docs](https://pkg.go.dev/net/http#Handler)

**2025 decision on error syntax**
- Robert Griesemer, Go blog, 3 June 2025: after `check`/`handle` (2018), `try` (2019, ~900 comments, abandoned because it hid control flow), and the `?` operator (Ian Lance Taylor, 2024), the Go team will stop pursuing syntactic changes for error handling for the foreseeable future and close such proposals. Quote: "we neither have a shared understanding of the problem, nor do we all agree that there is a problem in the first place." Also: new syntax would force everyone to adopt it "lest their code become unidiomatic". The post acknowledges error handling remains the top complaint in developer surveys. (fetched) — [Go blog: [ On | No ] syntactic support for error handling](https://go.dev/blog/error-syntax)

### Inferences
- For a backend in late 2026, a reasonable rule set: sentinels + typed errors only in the domain layer and only for conditions callers branch on; `%w` inside a service's own layers, explicit mapping (not `%w`) when crossing the transport boundary; `errors.AsType` over `errors.As` when on Go 1.26+.
- Since syntax changes are off the table, investment goes into linters (errorlint, wrapcheck, vet's new `%w` pointer check) rather than waiting for language help.

### Gaps
- Did not verify whether cockroachdb/errors has removed its pkg/errors dependency as of 2026.
- No fetched source for a canonical "domain error → HTTP status" pattern from Three Dots Labs; their articles were not fetched in this session.

## Context: rules, newer APIs (WithoutCancel, AfterFunc, causes), deadline propagation

### Takeaway
The context rules are stable and stated directly in the package docs: pass `ctx` as the first parameter, never store it in structs, never pass nil, and use values only for request-scoped data. Go 1.20/1.21 added cause-carrying cancellation, `WithoutCancel` for work that must outlive the request, and `AfterFunc` for cancellation callbacks. Go 1.26 made `signal.NotifyContext` record which signal fired as the cause.

### Cited Findings
- "Do not store Contexts inside a struct type; instead, pass a Context explicitly to each function that needs it." ctx is the first parameter, typically named `ctx`. Do not pass nil; use `context.TODO` if unsure. Use context values "only for request-scoped data that transits processes and APIs, not for passing optional parameters to functions." (fetched) — [pkg.go.dev/context](https://pkg.go.dev/context)
- Google: `context.Context` is always the first parameter; "Do not create custom context types or use interfaces other than `context.Context` in function signatures"; don't add a context member to a struct. (fetched) — [Google Go Decisions](https://google.github.io/styleguide/go/decisions)
- `WithCancelCause` (Go 1.20): cancel takes an error recorded as the cause, read with `context.Cause(ctx)`; `ctx.Err()` still returns `Canceled`; calling cancel with nil sets cause to `Canceled`; first cancellation wins. `Cause` returns nil if not yet canceled. (fetched) — [pkg.go.dev/context](https://pkg.go.dev/context)
- `WithTimeoutCause` / `WithDeadlineCause` (Go 1.21): the cause is set only when the timer expires; the returned `CancelFunc` does not set a cause. (fetched) — [pkg.go.dev/context](https://pkg.go.dev/context)
- `WithoutCancel` (Go 1.21): the child is not canceled when the parent is; has no deadline, nil `Done`, nil `Err`. Use for work that must finish after the request ends (audit write, async enqueue), keeping request values. Caveat: it severs cancellation, so add your own timeout. (fetched) — [pkg.go.dev/context](https://pkg.go.dev/context)
- `AfterFunc` (Go 1.21): runs `f` in its own goroutine once ctx is done (immediately if already done); the returned `stop` "does not wait for f to complete before returning". Coordinate explicitly if you need completion. (fetched) — [pkg.go.dev/context](https://pkg.go.dev/context)
- Go 1.26: `os/signal.NotifyContext` now cancels with a cause indicating which signal was received (read via `context.Cause`). (fetched) — [Go 1.26 release notes](https://go.dev/doc/go1.26)
- Original rationale and propagation pattern (derive child contexts with timeouts, always `defer cancel()`, check `ctx.Done()` in long loops). (not re-fetched) — [Go blog: Go Concurrency Patterns: Context](https://go.dev/blog/context)
- `go vet`'s `lostcancel` analyzer reports cancel functions from `WithCancel/WithTimeout/WithDeadline` that are not called on all paths. (not re-fetched) — [lostcancel analyzer](https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/lostcancel)

### Inferences
- Deadline propagation idiom: set the overall budget once at the edge (HTTP server timeouts / middleware `context.WithTimeout`), and inner calls only shorten it (`context.WithTimeout(ctx, 2*time.Second)` returns the earlier of the parent's deadline and the new one). Use `WithTimeoutCause` so logs say "db query budget exceeded" instead of a bare `context deadline exceeded`.
- `WithoutCancel` replaces the old anti-pattern of passing `context.Background()` into background work and losing trace/request IDs.

### Gaps
- No Go 1.27 context package changes were listed in the release notes.

## Concurrency: goroutine ownership, leaks, errgroup, primitives, structured concurrency, graceful shutdown, synctest, goleak

### Takeaway
Every goroutine needs an owner who can stop it and wait for it. In practice that means `errgroup` (with `WithContext` and `SetLimit` for bounded fan-out), `sync.WaitGroup.Go` (Go 1.25), and constructors that pair a goroutine with `Close`/`Shutdown`. The toolchain now backs this up with leak detection: goleak in tests, the runtime `goroutineleak` profile (experimental in 1.26, GA in 1.27), and `testing/synctest` (GA in 1.25) for deterministic tests of concurrent and time-based code.

### Cited Findings

**Ownership and leaks**
- Uber "Don't fire-and-forget goroutines": every goroutine must stop at a predictable time or be signalable to stop, and code must be able to block until it exits. Use `sync.WaitGroup` for many or a `done` channel for one. No goroutines in `init()`; expose a constructor plus `Close/Stop/Shutdown` that signals and waits. (fetched) — [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- Uber: channels should be unbuffered or size one; other sizes need justification. Use zero-value mutexes (`var mu sync.Mutex`), as non-pointer, non-embedded fields. (fetched) — [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- Uber recommends `go.uber.org/atomic` for type safety. Since Go 1.19 the stdlib has typed atomics (`atomic.Int64`, `atomic.Bool`, `atomic.Pointer[T]`), and Go 1.27's `go fix` adds an `atomictypes` modernizer, so the stdlib types are now the default choice. (Uber rule fetched; Go 1.27 modernizer fetched; 1.19 not re-fetched) — [Uber guide](https://github.com/uber-go/guide/blob/master/style.md), [Go 1.27 notes](https://go.dev/doc/go1.27), [Go 1.19 notes](https://go.dev/doc/go1.19)
- Runtime goroutine leak profile: experimental in Go 1.26 (`GOEXPERIMENT=goroutineleakprofile`), GA in Go 1.27 in `runtime/pprof` and at `/debug/pprof/goroutineleak`. It uses the GC to find goroutines blocked on channels/mutexes/conds that are unreachable from any runnable goroutine. It can miss leaks through globals or locals of runnable goroutines and adds no overhead unless used. The canonical example is a worker sending on an unbuffered channel after the collector returned early on error. (fetched) — [Go 1.26 notes](https://go.dev/doc/go1.26), [Go 1.27 notes](https://go.dev/doc/go1.27)
- Go 1.27: tracebacks include pprof goroutine labels in headers for `go 1.27` modules, which helps attribute goroutines in production dumps. (fetched) — [Go 1.27 notes](https://go.dev/doc/go1.27)

**errgroup, WaitGroup.Go, bounded concurrency**
- `errgroup.Group`: the zero value is valid (no limit, no cancel on error); don't reuse a group for different tasks. `WithContext` returns a derived ctx canceled on the first non-nil error or when `Wait` returns. `Wait` returns the first error. `SetLimit(n)` caps active goroutines; `Go` blocks until there is room; `TryGo` is the non-blocking variant; "The limit must not be modified while any goroutines in the group are active." The page showed v0.24.0 (23 Sep 2026) with a newer version available. (fetched) — [pkg.go.dev errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup)
- `sync.WaitGroup.Go` (Go 1.25) starts a goroutine and handles `Add`/`Done`. The 1.25 vet `waitgroup` analyzer reports misplaced `wg.Add` calls; in Go 1.27 it is renamed `waitgroupgo`. (fetched) — [Go 1.25 notes](https://go.dev/doc/go1.25), [Go 1.27 notes](https://go.dev/doc/go1.27)
- Weighted semaphore for resource-bounded concurrency: `golang.org/x/sync/semaphore.NewWeighted(n)` with `Acquire(ctx, w)`/`Release(w)`. (not re-fetched) — [pkg.go.dev semaphore](https://pkg.go.dev/golang.org/x/sync/semaphore)
- `sync.OnceFunc`, `sync.OnceValue`, `sync.OnceValues` (Go 1.21) wrap lazy one-time initialization; a panic in `f` is re-raised on every call. (not re-fetched) — [Go 1.21 notes](https://go.dev/doc/go1.21), [pkg.go.dev sync](https://pkg.go.dev/sync)
- Go 1.22 changed `for` loop variables to per-iteration, which removes the classic `v := v` capture bug in goroutines. (not re-fetched) — [Go blog: Fixing For Loops in Go 1.22](https://go.dev/blog/loopvar-preview)
- Go 1.24 reimplemented `sync.Map` for better performance on modifications. (fetched) — [Go 1.24 notes](https://go.dev/doc/go1.24)
- Go 1.25 container-aware `GOMAXPROCS`: on Linux it defaults to the cgroup CPU limit (Kubernetes CPU limit, not requests) and updates dynamically. This replaces the need for `go.uber.org/automaxprocs`. (fetched for runtime behavior; automaxprocs inference) — [Go 1.25 notes](https://go.dev/doc/go1.25)
- Go 1.25 `runtime/trace.FlightRecorder` keeps a ring buffer of recent trace data; call `WriteTo` on a significant event (e.g. slow request) to capture the last seconds. (fetched) — [Go 1.25 notes](https://go.dev/doc/go1.25)
- Channels vs mutexes: the Go wiki's guidance is to use whichever is simpler and more expressive. Channels fit passing ownership of data, distributing work and async results; mutexes fit caches and state. (not re-fetched) — [Go wiki: Use a sync.Mutex or a channel?](https://go.dev/wiki/MutexOrChannel)
- Pipelines and cancellation patterns (fan-out/fan-in, closing `done` to stop upstream stages). (not re-fetched) — [Go blog: Pipelines and cancellation](https://go.dev/blog/pipelines)

**Graceful shutdown**
- Pattern: `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`; run the server in a goroutine; on `<-ctx.Done()`, call `srv.Shutdown(shutdownCtx)` with a bounded timeout. Go 1.26 records the received signal as the context cause. (Go 1.26 part fetched) — [Go 1.26 notes](https://go.dev/doc/go1.26), [os/signal.NotifyContext](https://pkg.go.dev/os/signal#NotifyContext)
- `http.Server.Shutdown`: closes listeners, then idle connections, then waits for active connections to go idle. If the context expires first it returns the context's error. `Serve`/`ListenAndServe` return `ErrServerClosed` immediately, so the program must wait for `Shutdown` to return rather than exit. It does not close or wait for hijacked connections (WebSockets); use `RegisterOnShutdown` for those. (Partially fetched: `ErrServerClosed` returned after `Shutdown`/`Close` was confirmed. The rest is standard doc text, not re-fetched because the page was truncated.) — [net/http Server.Shutdown](https://pkg.go.dev/net/http#Server.Shutdown)

**Testing concurrency**
- `testing/synctest` (experimental in 1.24, GA in 1.25): `synctest.Test(t, f)` runs `f` in a bubble with a fake clock starting 2000-01-01 UTC; time advances only when all bubble goroutines are durably blocked; `synctest.Wait()` waits for that. Durably blocking: `time.Sleep`, channels created in the bubble, bubble WaitGroups, `sync.Cond.Wait`, `select{}`. Not durably blocking: I/O (including loopback network), syscalls, cgo, mutexes. The old `synctest.Run` API was removed in 1.26. (fetched) — [Go blog: Testing Time (Damien Neil, 26 Aug 2025)](https://go.dev/blog/testing-time), [Go 1.25 notes](https://go.dev/doc/go1.25)
- Go 1.27 adds `synctest.Sleep` (sleep plus `Wait`) and `httptest.NewTestServer`, an in-memory fake-network server that works inside synctest bubbles. (fetched) — [Go 1.27 notes](https://go.dev/doc/go1.27)
- goleak: `defer goleak.VerifyNone(t)` per test or `goleak.VerifyTestMain(m)` per package. With `t.Parallel` tests, goleak cannot tell leaks from unfinished tests, so use `VerifyTestMain`. Supports the two most recent Go minor versions. (fetched) — [uber-go/goleak](https://github.com/uber-go/goleak)
- goleak options `IgnoreTopFunction` / `IgnoreCurrent` exist for known background goroutines (README did not detail them). (not re-fetched) — [goleak options on pkg.go.dev](https://pkg.go.dev/go.uber.org/goleak)
- Race detector: `go test -race` instruments memory accesses and reports data races at runtime, with significant CPU/memory overhead, so it is usually run in CI. (not re-fetched) — [Go Data Race Detector](https://go.dev/doc/articles/race_detector)

### Inferences
- "Structured concurrency" in Go isn't a language feature. It is a convention built from errgroup + context, where a function that starts goroutines does not return until they finish. errgroup with `WithContext` is the closest stdlib-adjacent equivalent.
- Worker pool idiom for 2026: `g, ctx := errgroup.WithContext(ctx); g.SetLimit(n); for _, item := range items { g.Go(func() error { return process(ctx, item) }) }; return g.Wait()`. This replaces hand-rolled job/result channels in most backends.
- For production leak monitoring, scraping `/debug/pprof/goroutineleak` (Go 1.27) complements goleak in tests.

### Gaps
- Could not confirm from the errgroup page whether errgroup now propagates panics from `Go` callbacks to `Wait` (an upstream proposal existed). Treat that as unknown.
- No Three Dots Labs or GopherCon 2025/2026 concurrency talks were fetched.

## Testing: table-driven, subtests, helpers, golden files, doubles, assertion-library debate, integration tests with real Postgres, fuzzing, benchmarks, race, coverage, build tags

### Takeaway
Idiomatic Go testing in 2026 still centers on the stdlib: table-driven subtests, `t.Helper`/`t.Cleanup`/`t.Context`, `cmp.Diff` for comparisons, hand-written fakes over generated mocks, real dependencies via testcontainers-go (Postgres with snapshot/restore), `httptest`, built-in fuzzing, and `b.Loop` benchmarks. Google explicitly discourages assertion libraries. Uber and much of industry use testify anyway, so that debate is still open.

### Cited Findings

**Structure and helpers**
- Google: test helpers do setup/cleanup and call `t.Helper()`; "Assertion helpers are not considered idiomatic"; keep failure logic in the `Test` function. Calling `t.Fatal`/`t.FailNow` from any goroutine other than the test goroutine is incorrect; use `t.Error` and return. In tables without subtests use `t.Error` + `continue`; with `t.Run` subtests use `t.Fatal`. (fetched) — [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices)
- Google decisions: "Do not create 'assertion libraries' as helpers for testing"; failures must be diagnosable without reading the source; print got before want; identify the function (`YourFunc(%v) = %v, want %v`); prefer `t.Error` over `t.Fatal` and keep going. (fetched) — [Google Go Decisions](https://google.github.io/styleguide/go/decisions)
- Google: `cmp` "is not designed to fail tests, but rather to compare (and to diff) values"; combine with `cmpopts` in reusable helpers. (fetched) — [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices)
- Google: prefer the real transport connected to a test backend over hand-written clients or transport mocks. Name single doubles concisely (`creditcardtest.Stub`) and multiple ones by behavior (`AlwaysDeclines`). (fetched) — [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices)
- Table-driven tests with `t.Run` subtests (run individually with `-run 'TestX/case'`; subtests with `t.Parallel` run after the parent function body returns). (not re-fetched) — [Go blog: Using Subtests and Sub-benchmarks](https://go.dev/blog/subtests), [Go wiki: TableDrivenTests](https://go.dev/wiki/TableDrivenTests)
- Since Go 1.22's per-iteration loop variables, the `tc := tc` copy before `t.Parallel()` is no longer needed. (not re-fetched) — [Go blog: Fixing For Loops](https://go.dev/blog/loopvar-preview)
- `t.Context()` (Go 1.24) returns a context canceled after the test completes and before Cleanup functions run; `t.Chdir` (1.24) changes the working directory for the test. (fetched) — [Go 1.24 notes](https://go.dev/doc/go1.24)
- Go 1.24 vet `tests` analyzer runs as part of `go test` and catches malformed test names/signatures that can stop tests from running. (fetched) — [Go 1.24 notes](https://go.dev/doc/go1.24)
- Go 1.25: `t.Attr(key, value)` emits structured attributes (an "attr" action in `-json`); `t.Output()` returns an `io.Writer` to the test log. (fetched) — [Go 1.25 notes](https://go.dev/doc/go1.25)
- Go 1.26: `t.ArtifactDir()` returns a directory for test output files, kept with `go test -artifacts` and otherwise temporary. (fetched) — [Go 1.26 notes](https://go.dev/doc/go1.26)
- Go 1.27: `go test` runs the `stdversion` vet check by default; `-json` output gains an optional `OutputType` field. (fetched) — [Go 1.27 notes](https://go.dev/doc/go1.27)

**Hashimoto, "Advanced Testing with Go" (GopherCon 2017)**
- Recommendations: named table-driven cases; fixtures in a relative `test-fixtures` dir (cwd is the package dir); golden files regenerated with an `-update` flag and reviewed by hand; test helpers take `*testing.T`, fail inside, and return a cleanup closure (now largely replaced by `t.Cleanup`); avoid global state, or make it a configurable option that defaults to the global; use real loopback listeners (`127.0.0.1:0`) instead of mocking `net.Conn`; make ports, timeouts and paths configurable; use interfaces as mock points but not excessively; export test helpers and harnesses (e.g. `testing.go`) as public API; for subprocesses, run the real binary (skip if missing) or use a helper process; avoid fake time and use a timeout multiplier. He generally avoided `t.Parallel`. Primarily test exported APIs. (fetched) — [Slides on Speaker Deck](https://speakerdeck.com/mitchellh/advanced-testing-with-go), [GopherCon 2017 video](https://www.youtube.com/watch?v=8hQG7QlcLBk)
- Note: Hashimoto's "avoid fake time" (2017) predates `testing/synctest`, which gives deterministic fake time without injecting clocks. (inference from [Go blog: Testing Time](https://go.dev/blog/testing-time))

**Fakes vs mocks, testify debate**
- Google's guide favors real transports and simple named doubles over generated transport mocks, and rejects assertion libraries. (fetched) — [Google Go Best Practices](https://google.github.io/styleguide/go/best-practices), [Google Go Decisions](https://google.github.io/styleguide/go/decisions)
- `golang/mock` was archived in June 2023; Uber's maintained fork is `go.uber.org/mock` (mockgen). (not re-fetched) — [uber-go/mock](https://github.com/uber-go/mock)
- testify (`assert`, `require`, `mock`, `suite`) is the most widely used third-party test library; `require` stops the test (like `t.Fatal`) and `assert` continues (like `t.Error`). (not re-fetched) — [stretchr/testify](https://github.com/stretchr/testify)
- mockery generates testify-based mocks from interfaces. (not re-fetched) — [vektra/mockery](https://github.com/vektra/mockery)

**Integration tests with real Postgres**
- testcontainers-go Postgres: `postgres.Run(ctx, "postgres:16-alpine", postgres.WithDatabase(...), postgres.WithUsername(...), postgres.WithPassword(...), ...)`, then `ctr.ConnectionString(ctx, "sslmode=disable")`. The module sets no default wait strategy. Recommended: `wait.ForLog("database system is ready to accept connections").WithOccurrence(2)` (Postgres restarts once during init) plus `wait.ForListeningPort("5432/tcp")` to avoid flakes on macOS/Windows. (fetched) — [testcontainers-go Postgres module](https://golang.testcontainers.org/modules/postgres/)
- `WithInitScripts` (order not guaranteed) vs `WithOrderedInitScripts` (v0.37.0+). `ctr.Snapshot(ctx)` and `ctr.Restore(ctx)` (typically in `t.Cleanup`) reset DB state between tests. Restore uses the `database/sql` driver if one is registered (`postgres.WithSQLDriver("pgx")`) and falls back to slower `docker exec`. Do not name the test database `postgres`. Use `testcontainers.CleanupContainer(t, ctr)` to tie lifecycle to the test. `WithReuseByName` is experimental (v0.37.0+). (fetched) — [testcontainers-go Postgres module](https://golang.testcontainers.org/modules/postgres/)
- Build tags to separate slow integration tests: `//go:build integration` at the top of the file and `go test -tags=integration ./...`. A common alternative is `testing.Short()` with `-short`. (not re-fetched) — [go command: build constraints](https://pkg.go.dev/cmd/go#hdr-Build_constraints), [testing.Short](https://pkg.go.dev/testing#Short)

**HTTP testing**
- `httptest.NewRecorder` for handler unit tests; `httptest.NewServer` for real HTTP round trips against a loopback server. Go 1.27 adds `httptest.NewTestServer` with an in-memory network for synctest. (1.27 fetched; others not re-fetched) — [net/http/httptest](https://pkg.go.dev/net/http/httptest), [Go 1.27 notes](https://go.dev/doc/go1.27)

**Fuzzing, benchmarks, race, coverage**
- Native fuzzing (Go 1.18): `func FuzzX(f *testing.F)` with `f.Add` seeds and `f.Fuzz(func(t *testing.T, ...))`; run with `go test -fuzz=FuzzX`; failing inputs are saved under `testdata/fuzz/FuzzX` and become regression tests. (not re-fetched) — [Go Fuzzing docs](https://go.dev/doc/security/fuzz/)
- `b.Loop` (Go 1.24): `for b.Loop() { ... }` replaces `for range b.N`. Setup outside the loop runs once per `-count`, and parameters/results are kept alive so the compiler can't optimize the body away. Go 1.26 fixed `b.Loop` preventing inlining in the loop body. (fetched) — [Go 1.24 notes](https://go.dev/doc/go1.24), [Go 1.26 notes](https://go.dev/doc/go1.26)
- Compare benchmark runs statistically with `benchstat` (`-count=10`). (not re-fetched) — [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)
- Coverage: `go test -cover` / `-coverprofile`. Since Go 1.20, `go build -cover` produces instrumented binaries for integration-test coverage (`GOCOVERDIR`). (not re-fetched) — [Go blog: Code coverage for Go integration tests](https://go.dev/blog/integration-test-coverage)
- Race detector: run `go test -race` in CI. (not re-fetched) — [Data Race Detector](https://go.dev/doc/articles/race_detector)

### Inferences
- A defensible 2026 default for a Postgres-backed service: unit tests with stdlib + `go-cmp`, hand-written fakes for domain ports, `httptest` for handlers, and one testcontainers Postgres per package (in `TestMain` or a `sync.OnceValue` helper) with Snapshot/Restore per test, behind an `integration` build tag. CI runs with `-race`.
- testify vs stdlib comes down to team taste. Google's guide is the main authoritative voice against it, and Uber's own guide examples use testify. If you adopt it, `require` for preconditions and `assert` for checks matches Google's Fatal/Error guidance.

### Gaps
- Three Dots Labs articles (e.g. on testing microservices and on errors) were not fetched in this session, so their specific recommendations are not included.
- Did not confirm whether the Uber guide's "test tables" section (avoid complex conditional logic in table tests) has changed; the fetch truncated before it.
- No survey data on testify vs stdlib adoption was found or fetched.
- No fetched source confirms the latest testcontainers-go version as of Nov 2026.
