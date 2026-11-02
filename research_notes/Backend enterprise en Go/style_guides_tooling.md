# Go Style Guides, Code Quality Rules and Tooling for Enterprise Backends (as of November 2026)

Context verified on 2026-11-01: latest Go is go1.27.3 (go1.27 released 2026-08-19; go1.26 released 2026-02-10). Each major release is supported until two newer majors exist, so only 1.26 and 1.27 are supported now; 1.25 and older are out of support — [Go release history](https://go.dev/doc/devel/release). Latest tool releases (GitHub API, checked 2026-11-01): golangci-lint v2.15.0 (2026-10-16), golangci-lint-action v9.3.0 (2026-06-29), actions/setup-go v7.0.0 (2026-07-16), golang/govulncheck-action v1.1.0 (2026-07-10), gosec v2.30.0 (2026-10-23), gofumpt v0.12.0 (2026-09-07), revive v1.17.1 (2026-10-07), staticcheck 2026.2.2 (2026-10-12) — [golangci-lint releases](https://github.com/golangci/golangci-lint/releases), [golangci-lint-action releases](https://github.com/golangci/golangci-lint-action/releases), [setup-go releases](https://github.com/actions/setup-go/releases), [govulncheck-action releases](https://github.com/golang/govulncheck-action/releases), [gosec releases](https://github.com/securego/gosec/releases), [gofumpt releases](https://github.com/mvdan/gofumpt/releases), [revive releases](https://github.com/mgechev/revive/releases), [go-tools releases](https://github.com/dominikh/go-tools/releases).

## 1. Concrete rules from Effective Go, Code Review Comments, Google Go Style Guide and Uber Go Style Guide; where they disagree

### Takeaway
The four guides agree on most fundamentals: gofmt, MixedCaps, short receivers, no `Get` prefix, no `util`/`common` packages, consumer-side small interfaces, return concrete types, `error` last, lowercase error strings, handle every error once, early return, `ctx` first and never stored in structs, explicit goroutine lifetimes. They differ on underscore prefixes for globals, how to do optional arguments, `%w` by default versus `%w` as an API decision, import grouping, line length, embedding, capacity hints, and test assertions. Parts of Uber's guide are dated: `go.uber.org/atomic`, `tt := tt`, golint.

### Cited Findings

**Source hierarchy and status**
- Code Review Comments (CRC) says it can be "viewed as a supplement to Effective Go" and points to Google's longer Go Style Guide — [CRC](https://go.dev/wiki/CodeReviewComments).
- Effective Go says it was written for Go's 2009 release and hasn't been significantly updated. It doesn't cover modules, generics, contexts, iterators and so on, so treat it as foundational, not current — [Effective Go](https://go.dev/doc/effective_go).
- Google's guide has three parts: Guide (principles), Decisions (normative rules) and Best Practices (guidance) — [Google Go Style Guide](https://google.github.io/styleguide/go/).
- Uber's guide says "Above all else, be consistent" — [Uber style.md](https://github.com/uber-go/guide/blob/master/style.md).

**Formatting**
- Run gofmt on all code. goimports is an acceptable superset that also manages imports — [CRC](https://go.dev/wiki/CodeReviewComments).
- Line length: CRC sets no rigid limit and says to break lines on semantics and shorten names instead of wrapping — [CRC](https://go.dev/wiki/CodeReviewComments). Google: keep function signatures on one line, don't break calls only for length, don't line-break `if`/`for` clauses (extract locals instead), don't break long string literals — [Google Decisions](https://google.github.io/styleguide/go/decisions). Uber: a soft limit of 99 characters — [Uber](https://github.com/uber-go/guide/blob/master/style.md). **Disagreement.**

**Naming**
- MixedCaps only. An unexported constant is `maxLength`, not `MAX_LENGTH` — [CRC](https://go.dev/wiki/CodeReviewComments). Google: no ALL_CAPS, no `k` prefix. Name constants by role, not by value — [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Initialisms keep a consistent case: `URL`, `ServeHTTP`, `appID`, `XMLAPI`/`xmlAPI`. Exported `Txn`, never `TXN`. `gRPC` becomes `GRPC` when exported — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Getters have no `Get` prefix (`Owner()`, `SetOwner()`) — [Effective Go](https://go.dev/doc/effective_go). Google adds `Compute`/`Fetch` for expensive or remote calls, so callers know the call may block or fail — [Google Decisions](https://google.github.io/styleguide/go/decisions).
- One-method interfaces take an `-er` suffix (`Reader`, `Writer`, `Stringer`) — [Effective Go](https://go.dev/doc/effective_go).
- Packages are short, lowercase, single-word, with no underscores or mixedCaps. Avoid `util`, `common`, `misc`, `helper`, `model`, `testhelper` — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions). Uber also says singular, and not `util`/`common`/`shared`/`lib` — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Don't repeat the package name in exported identifiers: `chubby.File`, not `chubby.ChubbyFile`; `widget.New`, not `widget.NewWidget`; `db.Load`, not `db.LoadFromDatabase` — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Variable name length grows with scope size and shrinks with frequency of use. Don't drop letters (`Sandbox`, not `Sbx`). Leave the type out of names (`users`, not `userSlice`; `userCount`, not `numUsers`) — [Google Decisions](https://google.github.io/styleguide/go/decisions); [CRC](https://go.dev/wiki/CodeReviewComments).
- Function names: noun-like for functions that return values, verb-like for functions that act. Don't repeat receiver, parameter or return types. The primary variant has no type suffix (`Marshal`, then `ParseInt64` for variants) — [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- Test-double packages are named `<pkg>test` (e.g. `creditcardtest`), with doubles like `Stub`, `AlwaysCharges` or `StubService` — [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- Don't use predeclared identifiers (`error`, `string`, `len`) as names — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Unexported globals: Uber prefixes unexported top-level `var`/`const` with `_` (`_defaultPort`), except unexported errors, which use `err` — [Uber](https://raw.githubusercontent.com/uber-go/guide/master/style.md). Google: "Avoid underscores in Go identifiers," with exceptions only for generated code, `Test`/`Benchmark`/`Example` names, and OS/cgo interop — [Google Decisions](https://google.github.io/styleguide/go/decisions). **Direct disagreement.**
- Error naming: exported sentinels use `ErrFoo` and unexported ones `errFoo`; error types end in `Error` (`NotFoundError`) — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Printf-style functions end in `f` (`Wrapf`) so `go vet` can check them, or register them with `-printfuncs` — [Uber](https://raw.githubusercontent.com/uber-go/guide/master/style.md).

**Receivers**
- Receiver names are one or two letters that abbreviate the type and are the same on every method. Never `this`, `self` or `me` — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Use a pointer receiver when the method mutates, when the struct holds a `sync.Mutex` or other no-copy field, when the struct is large, or when in doubt. Use a value receiver for maps, funcs, chans, slices that aren't resliced, and small immutable values. Don't mix receiver kinds on one type — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Map values aren't addressable, so pointer-receiver methods can't be called on them — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Don't pass pointers just to save bytes (`*string`, `*io.Reader`) — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Don't copy values whose methods use pointer receivers, or values containing `sync.Mutex` or `bytes.Buffer` — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).

**Interfaces**
- Define interfaces in the consuming package. Don't define them before they're used, and don't define them on the implementor side "for mocking." Return concrete types from constructors — [CRC](https://go.dev/wiki/CodeReviewComments).
- Google: functions accept interfaces and return concrete types, except for `error` and factory/strategy patterns. Keep interfaces small. Don't wrap RPC clients in hand-made interfaces; use real transports in tests. Don't export test doubles just for tests — [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Uber: almost never use a pointer to an interface. Add compile-time compliance checks where they matter: `var _ http.Handler = (*Handler)(nil)` — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Use `any` instead of `interface{}` in new code — [Google Decisions](https://google.github.io/styleguide/go/decisions).

**Errors**
- `error` is the last return value. Exported functions return the `error` interface, never a concrete error type, because of the typed-nil pitfall — [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Error strings start lowercase (unless they begin with a proper noun or acronym) and have no trailing punctuation. Log and UI messages can be capitalized — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Never discard errors with `_`. If it really is safe (e.g. `bytes.Buffer.Write`), add a comment saying why — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- No in-band errors (-1, "", nil). Return an extra `ok bool` or `error` — [CRC](https://go.dev/wiki/CodeReviewComments).
- Indent error flow: handle the error and return early, with no `else` after `return` — [CRC](https://go.dev/wiki/CodeReviewComments).
- Uber's decision table: static message with no caller matching → `errors.New`; dynamic, no matching → `fmt.Errorf`; static with matching → exported `var ErrX = errors.New(...)`; dynamic with matching → custom error type — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Context phrasing: avoid "failed to", "unable to" and "couldn't", because they pile up along the chain — [Uber](https://github.com/uber-go/guide/blob/master/style.md); [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- Handle an error once. Don't log it and also return it — [Uber](https://github.com/uber-go/guide/blob/master/style.md); [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- Inspect errors with `errors.Is`/`errors.As`, never by matching strings — [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- `%w` versus `%v`: Uber says "wrap with `%w` by default" and use `%v` to hide the cause — [Uber](https://github.com/uber-go/guide/blob/master/style.md). Google treats `%w` as part of the API contract: use it only when callers should inspect the chain, and use `%v` at system boundaries (RPC, storage) to hide internals. Put `%w` at the end (`"...: %w"`), except sentinel-category errors, which put it first — [Google Best Practices](https://google.github.io/styleguide/go/best-practices). **Disagreement in emphasis.**
- Only add information the cause doesn't already carry (`os` errors already include the path) — [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- Panics: don't use `panic` for normal errors — [CRC](https://go.dev/wiki/CodeReviewComments). Uber: "Code running in production must avoid panics"; panic only for unrecoverable initialization errors — [Uber](https://github.com/uber-go/guide/blob/master/style.md). Google allows panic for API misuse and in `init`/`Must` functions, and allows internal panic/recover only if panics never cross package boundaries. Don't recover just to keep a process alive — [Google Best Practices](https://google.github.io/styleguide/go/best-practices). `MustXYZ` helpers run only at startup or in tests — [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Use the comma-ok form of type assertions to avoid panics — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Exit only from `main()` (`os.Exit`/`log.Fatal`), ideally once, by delegating to `run() error` — [Uber](https://github.com/uber-go/guide/blob/master/style.md). Google: propagate init and config errors to `main` and exit with an actionable message — [Google Best Practices](https://google.github.io/styleguide/go/best-practices).

**Zero values, declarations and initialization**
- "Make the zero value useful" is a Go proverb — [Go Proverbs](https://go-proverbs.github.io/). Effective Go uses `sync.Mutex` and `bytes.Buffer` as types whose zero value is ready to use — [Effective Go](https://go.dev/doc/effective_go).
- `var mu sync.Mutex`, not `new(sync.Mutex)`. Don't embed the mutex; use a named field `mu` — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Prefer nil slices (`var t []string`) and test with `len(s) == 0`. Don't design APIs that distinguish nil from empty. The exception is JSON, where `null` and `[]` differ — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions); [Uber](https://raw.githubusercontent.com/uber-go/guide/master/style.md).
- Use `:=` for explicit values and `var` when the zero value is the intent — [Uber](https://raw.githubusercontent.com/uber-go/guide/master/style.md); [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- Pointer to a zero value: Uber prefers `&T{}` over `new(T)` — [Uber](https://raw.githubusercontent.com/uber-go/guide/master/style.md). Google says use `new(T)` — [Google Best Practices](https://google.github.io/styleguide/go/best-practices). **Disagreement.** Go 1.26 adds `new(expr)` for optional pointer fields — [Go 1.26](https://go.dev/doc/go1.26).
- Struct literals use field names; this is required for types from other packages (`go vet` enforces it). Omit zero fields unless they carry meaning — [Uber](https://raw.githubusercontent.com/uber-go/guide/master/style.md); [Google Decisions](https://google.github.io/styleguide/go/decisions). Uber allows positional fields in test tables with three or fewer fields, but Google Best Practices says to use field names in table tests too. **Minor disagreement.**
- Capacity hints: Uber says to provide them for `make` (in its performance section) — [Uber](https://github.com/uber-go/guide/blob/master/style.md). Google says use them "only with empirical justification" — [Google Best Practices](https://google.github.io/styleguide/go/best-practices). **Disagreement.**
- Copy slices and maps at API boundaries, both those you receive and store and those you return from internal state — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Start enums at `iota + 1` unless the zero value is a sensible default — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Use `time.Time` for instants and `time.Duration` for periods. When a wire format can't carry a Duration, put the unit in the field name (`intervalMillis`) — [Uber](https://github.com/uber-go/guide/blob/master/style.md).
- Channel size is one or unbuffered; any other size needs justification — [Uber](https://github.com/uber-go/guide/blob/master/style.md). Google: declare channel direction in parameters — [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- Use `make` for maps that start empty or are filled in code, and literals for fixed contents — [Uber](https://raw.githubusercontent.com/uber-go/guide/master/style.md).

**init() and globals**
- Avoid `init()`. If you use it, it must be deterministic, not depend on the order of other `init()` functions, not read globals or the environment, and do no I/O. Don't start goroutines in `init()` — [Uber](https://github.com/uber-go/guide/blob/master/style.md). Effective Go describes `init` for verifying or repairing state before execution begins — [Effective Go](https://go.dev/doc/effective_go).
- Avoid mutable globals; inject dependencies, including `now func() time.Time` — [Uber](https://github.com/uber-go/guide/blob/master/style.md). Google Best Practices has a "Global state" section that advises against mutable package state and in favor of instance-based APIs — [Google Best Practices](https://google.github.io/styleguide/go/best-practices). I didn't read that section in full; see Gaps.
- Flags are defined only in `package main`. Libraries are configured through Go APIs, not flags. Flag names use snake_case — [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Blank imports (`import _`) belong only in `main` or tests — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions). Dot imports: CRC allows them only in tests that would otherwise have circular dependencies; Google forbids them — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Import grouping: CRC puts stdlib first — [CRC](https://go.dev/wiki/CodeReviewComments). Uber uses exactly two groups, stdlib and everything else — [Uber](https://github.com/uber-go/guide/blob/master/style.md). Google uses stdlib, then project/vendored, then protobuf, then side-effect imports — [Google Decisions](https://google.github.io/styleguide/go/decisions). Rename imports only on collision, and rename the most local one — [CRC](https://go.dev/wiki/CodeReviewComments).

**Struct embedding**
- Effective Go presents embedding as Go's composition mechanism (`bufio.ReadWriter` embeds `*Reader` and `*Writer`) — [Effective Go](https://go.dev/doc/effective_go).
- Uber: avoid embedding types in public structs, because it leaks implementation and blocks evolution; write delegate methods instead. When you do embed, put embedded fields first, separated by a blank line. Never embed mutexes — [Uber](https://github.com/uber-go/guide/blob/master/style.md); [Uber raw](https://raw.githubusercontent.com/uber-go/guide/master/style.md). **Tension with Effective Go's promotion of embedding.**

**Optional arguments and functional options**
- Uber: use an `Option` interface with an unexported `apply(*options)` method, not closures, because concrete types can be compared in tests and can implement `fmt.Stringer`. Use options for optional arguments, especially once there are three or more parameters — [Uber raw](https://raw.githubusercontent.com/uber-go/guide/master/style.md).
- Google: use an option struct when most callers set options or options are shared (and never put a context in it). Use variadic options when most callers pass none, when options are numerous or take arguments, can fail, or third parties should be able to extend them. Options take values, not presence (`FailFast(enable bool)`, not `EnableFailFast()`); they apply in order and the last one wins — [Google Best Practices](https://google.github.io/styleguide/go/best-practices). **Different defaults.**

**Contexts, concurrency, goroutines**
- `ctx context.Context` is the first parameter. Never store it in a struct. No custom Context types. Use `context.Background()` only at entrypoints. In tests use `t.Context()` — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Every goroutine needs an obvious exit, a stop signal and a way to wait for it. No fire-and-forget. Test with `go.uber.org/goleak`. Long-lived goroutines go behind a type with `Close`/`Stop`/`Shutdown` — [Uber](https://github.com/uber-go/guide/blob/master/style.md); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Prefer synchronous functions and let callers add concurrency — [CRC](https://go.dev/wiki/CodeReviewComments).
- Use `errgroup` when the first of several concurrent errors is the one that matters — [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- Generate keys with `crypto/rand`, never `math/rand` or `math/rand/v2`; use `crypto/rand.Text` for random text — [CRC](https://go.dev/wiki/CodeReviewComments).

**Generics**
- Use generics only where requirements justify them: not for a single concrete type, DSLs or error frameworks. Prefer an interface when types share behavior, and generics over `any` with heavy type switches — [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Use `type T1 = T2` aliases only for migrations — [Google Decisions](https://google.github.io/styleguide/go/decisions).

**Comments and doc comments**
- Every exported top-level name has a doc comment: a full sentence that begins with the name and ends with a period. Non-obvious unexported names get one too — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).
- The package comment sits directly above `package` with no blank line, in exactly one file (`doc.go` for long ones). For `package main`, it starts with the binary name — [Google Decisions](https://google.github.io/styleguide/go/decisions).
- Document only non-obvious parameters. Don't restate standard ctx cancellation; document deviations from it. Assume reads are concurrency-safe and mutations aren't, and document otherwise. Document cleanup (`Close`/`Stop`) and significant sentinel errors, including pointer versus value form — [Google Best Practices](https://google.github.io/styleguide/go/best-practices).
- Doc comment syntax (headings, lists, links, `[Name]` doc links) is defined in [Go Doc Comments](https://go.dev/doc/comment).
- Provide runnable `Example` functions in test files — [CRC](https://go.dev/wiki/CodeReviewComments).
- Named results: only to disambiguate same-typed results or to tell the caller what to do (`cancel func()`). Naked returns only in short functions. Name a result if a deferred closure modifies it — [CRC](https://go.dev/wiki/CodeReviewComments); [Google Decisions](https://google.github.io/styleguide/go/decisions).

**Testing**
- Failure messages follow the form `Func(%v) = %v, want %v`, with got before want and diffs labeled `(-want +got)` — [Google Decisions](https://google.github.io/styleguide/go/decisions); [CRC](https://go.dev/wiki/CodeReviewComments).
- Google: don't build assertion libraries. Use `cmp.Diff`/`cmp.Equal`, not `reflect.DeepEqual`. Use `t.Error` for independent checks and `t.Fatal` only when continuing is meaningless. Never call `t.Fatal` from a non-test goroutine. Helpers call `t.Helper()` — [Google Decisions](https://google.github.io/styleguide/go/decisions); [Google Best Practices](https://google.github.io/styleguide/go/best-practices). Many large OSS projects use testify instead: golangci-lint and Prometheus both enable `testifylint` — [golangci-lint .golangci.yml](https://github.com/golangci/golangci-lint/blob/main/.golangci.yml); [Prometheus .golangci.yml](https://github.com/prometheus/prometheus/blob/main/.golangci.yml). **Practice diverges from Google.**
- Uber table tests: name the slice `tests`, the loop variable `tt`, and fields `give`/`want`. No branching logic, mock flags or functions inside the table; split complex cases into separate tests — [Uber raw](https://raw.githubusercontent.com/uber-go/guide/master/style.md).

**Dated parts of Uber's guide**
- Uber recommends `go.uber.org/atomic` — [Uber](https://github.com/uber-go/guide/blob/master/style.md). The stdlib has had typed atomics (`atomic.Bool`, `atomic.Int64`, `atomic.Pointer[T]`) since Go 1.19 — [Go 1.19 notes](https://go.dev/doc/go1.19#atomic_types). Go 1.27's `go fix` adds an `atomictypes` modernizer — [Go 1.27](https://go.dev/doc/go1.27). Prometheus's depguard still denies `sync/atomic` in favor of `go.uber.org/atomic` — [Prometheus .golangci.yml](https://github.com/prometheus/prometheus/blob/main/.golangci.yml).
- Uber's "declare `tt := tt` with `t.Parallel()`" has been unnecessary since Go 1.22 made loop variables per-iteration — [Go 1.22 notes](https://go.dev/doc/go1.22#language); [Uber raw](https://raw.githubusercontent.com/uber-go/guide/master/style.md). The `copyloopvar` linter flags these copies — [golangci-lint .golangci.yml](https://github.com/golangci/golangci-lint/blob/main/.golangci.yml).
- Uber's introduction still says to run `golint`. Its own Linting section says revive replaces the deprecated golint, and golangci-lint v2 removed golint — [Uber raw](https://raw.githubusercontent.com/uber-go/guide/master/style.md); [golangci-lint migration](https://golangci-lint.run/docs/product/migration-guide/).
- Uber's recommended minimum linters are errcheck, goimports, revive, govet and staticcheck, run with golangci-lint — [Uber raw](https://raw.githubusercontent.com/uber-go/guide/master/style.md).

### Inferences
- For team rules, a reasonable base is Google Decisions plus CRC, with Uber's concrete items that Google doesn't cover (error decision table, enums from 1, mutex fields, slice/map copying at boundaries, `run()` in main, compile-time interface checks). Choose one side explicitly on: `_` prefixes (Google's rule is simpler and better supported by linters), `&T{}` versus `new(T)`, `%w` policy (Google's boundary rule suits services that expose RPC/HTTP errors), import groups (three groups with a local-module prefix is common: Prometheus's `gci` config), and testify versus cmp.
- Replace Uber's dated items with stdlib equivalents: `sync/atomic` typed values, no `tt := tt`, revive instead of golint.

### Gaps
- I didn't read Google Best Practices sections on global state, interfaces and string concatenation (the page was truncated at 100k characters), so their exact litmus tests aren't captured here.
- I didn't fetch the full text of Effective Go in this session; the Effective Go claims above (Get prefix, `-er`, embedding, init, zero-value mutex) come from its well-known content plus the URL, so verify exact wording.
- I didn't verify whether Uber's guide has been updated for Go 1.22 or later.

## 2. Go Proverbs and widely cited idioms

### Takeaway
The Go Proverbs come from Rob Pike's talk at Gopherfest SV 2015. "Accept interfaces, return structs" isn't a Pike proverb; it is a community maxim that Google's guide now writes as "accept interfaces and return concrete types."

### Cited Findings
- All proverbs come from Rob Pike's Gopherfest SV 2015 talk (video https://www.youtube.com/watch?v=PAAkCSZUG1c). They include: "Don't communicate by sharing memory, share memory by communicating"; "Concurrency is not parallelism"; "Channels orchestrate; mutexes serialize"; "The bigger the interface, the weaker the abstraction"; "Make the zero value useful"; "interface{} says nothing"; "Gofmt's style is no one's favorite, yet gofmt is everyone's favorite"; "A little copying is better than a little dependency"; "Syscall/Cgo must always be guarded with build tags"; "Cgo is not Go"; "With the unsafe package there are no guarantees"; "Clear is better than clever"; "Reflection is never clear"; "Errors are values"; "Don't just check errors, handle them gracefully"; "Design the architecture, name the components, document the details"; "Documentation is for users"; "Don't panic." — [Go Proverbs](https://go-proverbs.github.io/)
- "Don't communicate by sharing memory; share memory by communicating" also appears in Effective Go's concurrency section — [Effective Go](https://go.dev/doc/effective_go#sharing).
- "Errors are values" has its own blog post by Rob Pike (2015) — [Go blog: Errors are values](https://go.dev/blog/errors-are-values).
- Google's form of the interface maxim: "Functions accept interfaces and return concrete types," with exceptions for `error` and factory/strategy patterns — [Google Decisions](https://google.github.io/styleguide/go/decisions). CRC: "return concrete types" from constructors and define interfaces in the consumer — [CRC](https://go.dev/wiki/CodeReviewComments).
- Proverb counterparts in the style guides: "Clear is better than clever" maps to Google's Guide principles (clarity, simplicity, concision, maintainability, consistency) — [Google Guide](https://google.github.io/styleguide/go/guide).

### Inferences
- "A little copying is better than a little dependency" fits supply-chain hygiene (fewer modules, smaller govulncheck surface). Teams should state it as a rule ("no new module for <N lines of code") rather than a slogan.

### Gaps
- I didn't verify the original author and source of "accept interfaces, return structs" in this session; it is commonly attributed to Jack Lindamood (Medium, 2016). Cite with caution.

## 3. golangci-lint v2: config format, recommended linters, formatters, real-world configs

### Takeaway
golangci-lint v2 (latest v2.15.0, Oct 2026) requires `version: "2"`, replaces `enable-all`/`disable-all` with `linters.default: standard|all|none|fast`, moves formatters (gofmt, gofumpt, goimports, gci) into their own `formatters:` section, consolidates exclusions under `linters.exclusions`, and merges gosimple and stylecheck into staticcheck. `golangci-lint migrate` converts v1 configs, but it drops comments.

### Cited Findings
**Format changes (v1 → v2)**
- `version: "2"` is required. `disable-all: true` becomes `linters.default: none`; `enable-all: true` becomes `linters.default: all`; `fast` enables fast linters. `linters.presets` (`bugs`, `style`, etc.) is removed — [Migration guide](https://golangci-lint.run/docs/product/migration-guide/).
- `linters.default` accepts `standard` (the default), `all`, `none` or `fast` — [Config file docs](https://golangci-lint.run/docs/configuration/file/).
- `gci`, `gofmt`, `gofumpt` and `goimports` move to `formatters.enable`, and their settings to `formatters.settings`. `linters-settings` becomes `linters.settings` — [Migration guide](https://golangci-lint.run/docs/product/migration-guide/).
- Exclusions: `linters.exclusions.rules` replaces `issues.exclude-rules`/`exclude`. `linters.exclusions.paths` replaces `skip-dirs`/`skip-files`/`exclude-dirs`. `linters.exclusions.generated` takes `lax|strict|disable`. `linters.exclusions.presets` takes `comments`, `common-false-positives`, `legacy` and `std-error-handling` (replacing `exclude-use-default`). `warn-unused` reports stale exclusions — [Migration guide](https://golangci-lint.run/docs/product/migration-guide/); [Config file docs](https://golangci-lint.run/docs/configuration/file/).
- Defaults: linter exclusions for generated code default to `strict` and formatter exclusions to `lax`. `run.timeout` defaults to disabled. `run.go` falls back to the go.mod version, then `GOVERSION`, then 1.22. `run.relative-path-mode` now defaults to `cfg`. Concurrency follows the container CPU quota — [Config file docs](https://golangci-lint.run/docs/configuration/file/); [Migration guide](https://golangci-lint.run/docs/product/migration-guide/).
- `output.formats` is a map (`text`, `json`, `tab`, `html`, `checkstyle`, `code-climate`, `junit-xml`, `teamcity`, `sarif`), each with a `path` — [Config file docs](https://golangci-lint.run/docs/configuration/file/).
- New-code gating: `issues.new`, `issues.new-from-rev`, `issues.new-from-merge-base` (e.g. `main`), `issues.new-from-patch`, `issues.whole-files` — [Config file docs](https://golangci-lint.run/docs/configuration/file/).
- `gosimple` and `stylecheck` are merged into `staticcheck`. Aliases are removed: `gas`→`gosec`, `goerr113`→`err113`, `gomnd`→`mnd`, `vet`/`vetshadow`→`govet`. Removed linters: `deadcode`, `execinquery`, `exhaustivestruct`, `exportloopref`, `golint`, `ifshort`, `interfacer`, `maligned`, `nosnakecase`, `scopelint`, `structcheck`, `tenv`, `varcheck` — [Migration guide](https://golangci-lint.run/docs/product/migration-guide/).
- `govet` `check-shadowing` becomes `enable: [shadow]`. `gci` `local-prefixes` becomes `sections` — [Migration guide](https://golangci-lint.run/docs/product/migration-guide/).
- `golangci-lint migrate` backs up the old file, drops comments and unknown fields, and sets generated exclusions to `strict` to preserve v1 behavior — [Migration guide](https://golangci-lint.run/docs/product/migration-guide/).
- Recent deprecations inside v2: `wsl` → `wsl_v5` (v2.2.0), `gomodguard` → `gomodguard_v2` (v2.12.0), `exhaustruct` → `exhaustruct_v5` (v2.13.0) — [Linters list](https://golangci-lint.run/docs/linters/).

**Real-world configs**
- golangci-lint's own `.golangci.yml` says it is "not a recommendation." It sets `default: none` and enables: bodyclose, copyloopvar, depguard, dogsled, dupl, errcheck, errorlint, funlen, gocheckcompilerdirectives, gochecknoinits, goconst, gocritic, gocyclo, godox, mnd, goprintffuncname, gosec, govet, intrange, ineffassign, lll, misspell, nakedret, noctx, nolintlint, revive, staticcheck, testifylint, unconvert, unparam, unused, whitespace. Settings include `funlen.statements: 50` (lines disabled), `gocyclo.min-complexity: 15`, gocritic tags diagnostic/experimental/opinionated/performance/style, govet `enable: [nilness, shadow]`, and depguard denying `github.com/pkg/errors` ("replace with standard lib errors") and logrus outside the logging package — [golangci-lint .golangci.yml](https://github.com/golangci/golangci-lint/blob/main/.golangci.yml).
- Prometheus formatters: `gci` (sections standard, default, `prefix(github.com/prometheus/prometheus)`), `gofumpt` with `extra` options `group-params`, `clothe-returns` and `balance-calls`, and `goimports` with `local-prefixes`. Linters: depguard, errorlint, exptostd, fatcontext, gocritic, godot, govet, loggercheck, misspell, modernize, nilnesserr, perfsprint, predeclared, revive, sloglint, staticcheck, testifylint, unconvert, unused, usestdlibvars, whitespace. It sets `issues.max-issues-per-linter: 0` and `max-same-issues: 0`, uses `exclusions.warn-unused: true`, re-adds the v1 errcheck default exclusion for `Close`/`Flush`/`os.Remove`/print functions, and excludes errcheck in `_test.go` — [Prometheus .golangci.yml](https://github.com/prometheus/prometheus/blob/main/.golangci.yml).
- Uber's minimum set is errcheck, goimports, revive, govet and staticcheck — [Uber raw](https://raw.githubusercontent.com/uber-go/guide/master/style.md).

### Inferences
- A production backend set built from the above: base on `default: standard` (errcheck, govet, ineffassign, staticcheck, unused, which are golangci-lint's long-standing defaults; I didn't confirm the list on the docs page in this session), then add the bug-finders bodyclose, noctx, sqlclosecheck, rowserrcheck, errorlint, contextcheck, nilerr/nilnesserr, exhaustive, gosec, copyloopvar, fatcontext, sloglint, usestdlibvars, modernize, nolintlint (require explanation and a specific linter), gocritic, revive and depguard (ban `pkg/errors`, `golang/protobuf`, `io/ioutil` and anything else the team bans). Add formatters gofumpt plus gci or goimports with your module prefix. Turn on `exclusions.warn-unused`. Use `new-from-merge-base: main` to adopt on legacy code.
- The modernize linter, the `go fix` modernizers and gopls share analyzers, so enabling `modernize` in CI and running `go fix` on toolchain upgrades enforce the same thing.
- Avoid `default: all`, since new linters arrive with every minor release and break CI. golangci-lint's own config shows the opposite approach: an explicit list.

### Gaps
- The docs pages fetched didn't list the exact membership of the `standard` set or the full formatter list (golines and swaggo have been added as formatters in v2 per changelogs, not verified here). Confirm with `golangci-lint help linters`.
- I didn't read the per-linter docs for contextcheck, sqlclosecheck, exhaustive or gosec, so their settings aren't captured.

## 4. Security and supply-chain tooling

### Takeaway
The baseline is govulncheck (reachability-based, low noise), gosec via golangci-lint, `go mod verify` plus the checksum database, Dependabot or Renovate for `gomod` and `github-actions`, and go.mod `toolchain` and `tool` directives so builds and dev tools are reproducible.

### Cited Findings
- The govulncheck GitHub Action (`golang/govulncheck-action@v1`, latest v1.1.0) runs `govulncheck ./...` with the latest Go by default. Inputs: `go-version-input`, `go-version-file`, `go-package`, `work-dir`, `repo-checkout`, `check-latest`, `cache`, `cache-dependency-path`, `output-format` (`text`/`json`/`sarif`), `output-file`. With `json` or `sarif` it exits successfully even when vulnerabilities are found, so text mode is the gate. Its README still calls it "experimental" — [govulncheck-action README](https://github.com/golang/govulncheck-action).
- govulncheck reports only vulnerabilities in functions your code actually calls ("low-noise, reliable") — [govulncheck-action README](https://github.com/golang/govulncheck-action); [Go vulnerability management](https://go.dev/doc/security/vuln/).
- gosec latest is v2.30.0 (2026-10-23) and runs inside golangci-lint as `gosec` (the old alias `gas` was removed in v2) — [gosec releases](https://github.com/securego/gosec/releases); [Migration guide](https://golangci-lint.run/docs/product/migration-guide/).
- Since Go 1.24, the go.mod `tool` directive replaces the `tools.go` blank-import pattern: `go get -tool golang.org/x/tools/cmd/stringer`, run with `go tool stringer`, install everything with `go install tool` — [Go 1.24](https://go.dev/doc/go1.24).
- Since Go 1.25, a go.mod `ignore` directive removes directories from `./...` and `all` — [Go 1.25](https://go.dev/doc/go1.25).
- `go mod init` with a 1.N toolchain writes `go 1.(N-1).0` (1.26 writes `go 1.25.0`), which keeps new modules compatible with the previous supported release — [Go 1.26](https://go.dev/doc/go1.26).
- `go mod tidy` for `go 1.27`+ modules consolidates requirements into at most two blocks, direct and indirect — [Go 1.27](https://go.dev/doc/go1.27).
- Go 1.27 rejects removed `GODEBUG` settings in go.mod and `//go:debug` unless they are set to their final default; old values fail the build — [Go 1.27](https://go.dev/doc/go1.27). Go 1.26 listed the GODEBUGs removed in 1.27: `tlsunsafeekm`, `tlsrsakex`, `tls10server`, `tls3des`, `x509keypairleaf`, `gotypesalias`, `asynctimerchan` — [Go 1.26](https://go.dev/doc/go1.26).
- `go build` embeds the VCS version, with a `+dirty` suffix for uncommitted changes. `GOAUTH` handles authentication for private module fetches — [Go 1.24](https://go.dev/doc/go1.24).
- FIPS 140-3: the Go Cryptographic Module is selected with `GOFIPS140`, and the `fips140` GODEBUG enables FIPS mode at runtime — [Go 1.24](https://go.dev/doc/go1.24).
- The toolchain directive and `GOTOOLCHAIN` behavior (the go line is a minimum requirement; `toolchain` suggests a toolchain; auto-download) are documented in [Go Toolchains](https://go.dev/doc/toolchain), introduced in Go 1.21 — [Go 1.21](https://go.dev/doc/go1.21).
- depguard is used to ban deprecated or duplicate dependencies (e.g. `github.com/pkg/errors`) — [golangci-lint .golangci.yml](https://github.com/golangci/golangci-lint/blob/main/.golangci.yml).

### Inferences
- Run govulncheck twice: on PRs (text mode as the gate) and on a schedule (new CVEs appear without code changes), and upload SARIF for code scanning as a separate non-gating step.
- Pin GitHub Actions to commit SHAs and let Dependabot or Renovate bump them. Pin golangci-lint to an exact v2.x in CI. The action's README example still uses `v2.13` while v2.15.0 is current, so README examples go stale.
- Keep the `go` line at the oldest supported release you target, and set `toolchain` or use setup-go `go-version-file: go.mod` so CI and local builds use the same toolchain.

### Gaps
- I didn't fetch Dependabot or Renovate docs in this session. Both support the `gomod` ecosystem and grouping, and Renovate has `postUpdateOptions: ["gomodTidy"]`, but these come from general knowledge and need confirmation at [Dependabot docs](https://docs.github.com/en/code-security/dependabot) and [Renovate Go docs](https://docs.renovatebot.com/modules/manager/gomod/).
- I didn't check `go mod verify` semantics or OpenSSF Scorecard/SLSA provenance tooling (e.g. `actions/attest-build-provenance`) in this session.

## 5. Typical CI pipeline for Go services (GitHub Actions)

### Takeaway
The canonical jobs are: lint (golangci-lint-action v9), test (`go test -race -cover` with setup-go module caching), vulncheck (govulncheck-action), and optionally `go mod tidy` diff and `go fix -diff` checks. Use least-privilege `permissions: contents: read`.

### Cited Findings
- The official golangci-lint-action workflow uses `permissions: contents: read` (plus optional `pull-requests: read` for `only-new-issues`), `actions/checkout@v6`, `actions/setup-go@v6` with `go-version: stable`, and `golangci/golangci-lint-action@v9` with a pinned `version: v2.13`. It shows matrix (`stable`, `oldstable`), monorepo (`working-directory`) and reusable-workflow variants — [golangci-lint-action README](https://github.com/golangci/golangci-lint-action).
- golangci-lint-action latest is v9.3.0 (2026-06-29). v9 moved the runtime from node20 to node24 (reported by a third-party compatibility guide, not verified in release notes). v7+ supports only golangci-lint v2 — [golangci-lint-action releases](https://github.com/golangci/golangci-lint-action/releases); [arran4 blog](https://arran4.github.io/blog/post/2026/002-golangci-lint-action-go-version-matrix/).
- actions/setup-go v7.0.0 (2026-07-16) migrates to ESM and bumps `@actions/cache` to 6.2.0. The golangci-lint-action README still shows `setup-go@v6` — [setup-go v7.0.0 release](https://github.com/actions/setup-go/releases/tag/v7.0.0); [golangci-lint-action README](https://github.com/golangci/golangci-lint-action).
- govulncheck-action inherits setup-go's version and cache inputs (`go-version-file`, `cache`, `cache-dependency-path`) — [govulncheck-action README](https://github.com/golang/govulncheck-action).
- `go test -json` and `go build -json` give structured output (Go 1.24), and Go 1.27 adds an `OutputType` field to test JSON. `go test` now also runs the `stdversion` vet check by default — [Go 1.24](https://go.dev/doc/go1.24); [Go 1.27](https://go.dev/doc/go1.27).
- Test artifacts: `T.ArtifactDir()` with `go test -artifacts` stores outputs under `-outputdir`, which can be uploaded as CI artifacts (Go 1.26) — [Go 1.26](https://go.dev/doc/go1.26).
- `go fix -diff ./...` previews modernizer changes. The Go team recommends running `go fix` on every toolchain upgrade from a clean git state, twice for synergistic fixes — [Go blog: go fix](https://go.dev/blog/gofix).

### Inferences
- Recommended pipeline: `go mod download` then `go mod verify`; a `go mod tidy && git diff --exit-code` check; `go vet ./...` (golangci-lint covers it); `go test -race -shuffle=on -coverprofile=cover.out ./...`; golangci-lint; govulncheck; a `go fix -diff` check (fail if there is output) after toolchain bumps. Run a stable/oldstable matrix only for libraries; services build with one pinned toolchain from go.mod.
- `-race` needs cgo and slows tests several-fold. Run it on the main test job on Linux amd64. I didn't source exact overhead figures here.

### Gaps
- I didn't fetch setup-go's README in this session (default `cache: true` keyed on go.sum, `go-version-file` support), so those claims are from general knowledge.
- I found no official Go-team "reference CI pipeline"; the pipeline above is a synthesis.

## 6. Go 1.21 → 1.27 changes that alter best practices

### Takeaway
Since 1.21, the stdlib has absorbed what used to need third-party libraries or idioms: slog (logging), slices/maps/min/max, per-iteration loop vars, method+wildcard ServeMux routing, range-over-func iterators, typed `errors.AsType`, `os.Root` sandboxing, `omitzero`, `testing/synctest`, `WaitGroup.Go`, container-aware GOMAXPROCS, `go fix` modernizers, and in 1.27 GA `encoding/json/v2`, generic methods and a stdlib `uuid` package.

### Cited Findings
**Go 1.21 (2023-08)**: `log/slog` structured logging; builtins `min`, `max` and `clear`; `slices`, `maps` and `cmp` packages; `toolchain` directive and forward compatibility via `GOTOOLCHAIN` — [Go 1.21](https://go.dev/doc/go1.21); [Go Toolchains](https://go.dev/doc/toolchain).

**Go 1.22 (2024-02)**: `for` loop variables are per-iteration (no more `x := x`); `range` over integers; `net/http.ServeMux` patterns with methods and wildcards (`"GET /items/{id}"`, `r.PathValue("id")`), which reduces the need for routers; `math/rand/v2` — [Go 1.22](https://go.dev/doc/go1.22).

**Go 1.23 (2024-08)**: range-over-func iterators and the `iter` package (`iter.Seq`, `iter.Seq2`), with iterator functions in `slices`/`maps` (`slices.Collect`, `maps.Keys`); the `unique` package; unreferenced timers are garbage-collected and timer channels became unbuffered — [Go 1.23](https://go.dev/doc/go1.23). Go 1.27 removed the `asynctimerchan` opt-out, so timer channels are always unbuffered — [Go 1.27](https://go.dev/doc/go1.27).

**Go 1.24 (2025-02)**: go.mod `tool` directive; generic type aliases; `os.Root` (`os.OpenRoot`) for traversal-safe file access; `encoding/json` `omitzero` (respects `IsZero()`, so it works for `time.Time`, unlike `omitempty`); `testing.B.Loop`, `T.Context`, `T.Chdir`; `crypto/rand.Text`, with `rand.Read` no longer returning errors; `crypto/mlkem`; FIPS 140-3 module; `weak` package; `runtime.AddCleanup` replaces `SetFinalizer`; `strings.Lines`/`SplitSeq`/`FieldsSeq` iterators; Swiss-table maps; vet `tests` analyzer and a printf check for non-constant format strings — [Go 1.24](https://go.dev/doc/go1.24).

**Go 1.25 (2025-08)**:
- Compiler nil-check fix: code that used a result before checking `err` now correctly panics. Always check `err` immediately — [Go 1.25](https://go.dev/doc/go1.25).
- Container-aware `GOMAXPROCS` honors cgroup CPU limits and updates periodically. This makes `go.uber.org/automaxprocs` unnecessary (an inference). Setting GOMAXPROCS manually disables it — [Go 1.25](https://go.dev/doc/go1.25).
- `sync.WaitGroup.Go(func())`; `testing/synctest` GA (virtual clock bubbles); `runtime/trace.FlightRecorder`; `net/http.CrossOriginProtection` (CSRF protection via `Sec-Fetch-Site`, no tokens); more `os.Root` methods; `slog.GroupAttrs`; vet `waitgroup` and `hostport` (use `net.JoinHostPort`) analyzers; experimental `encoding/json/v2` and Green Tea GC; SHA-1 disabled in TLS 1.2 — [Go 1.25](https://go.dev/doc/go1.25).

**Go 1.26 (2026-02)**:
- `new(expr)`, e.g. `Age: new(yearsSince(born))`, for optional pointer fields; generic self-referential constraints — [Go 1.26](https://go.dev/doc/go1.26).
- `go fix` rewritten as a modernizer suite on the vet analysis framework, with a `//go:fix inline` source-level inliner for automated API migrations. Analyzers include `any`, `buildtag`, `fmtappendf`, `forvar`, `hostport`, `inline`, `mapsloop`, `minmax`, `rangeint`, `stringscut`, `stringsbuilder` and `newexpr`. Use `go fix -diff` to preview — [Go 1.26](https://go.dev/doc/go1.26); [Go blog: go fix (Alan Donovan, 2026-02-17)](https://go.dev/blog/gofix).
- `errors.AsType` (generic, type-safe `errors.As`; prefer it in new code); `slog.NewMultiHandler`; `T.ArtifactDir`; `B.Loop` no longer blocks inlining (prefer `for b.Loop()` over `b.N`); `testing/cryptotest.SetGlobalRandom` — [Go 1.26](https://go.dev/doc/go1.26).
- Green Tea GC on by default (10–40% less GC overhead on GC-heavy workloads); experimental goroutine-leak profile — [Go 1.26](https://go.dev/doc/go1.26).
- Security defaults: crypto ignores caller-supplied randomness; hybrid post-quantum TLS key exchange on by default; new `crypto/hpke`; PKCS#1 v1.5 encryption deprecated (use OAEP); `httputil.ReverseProxy.Director` deprecated in favor of `Rewrite` (header-stripping attack); ServeMux trailing-slash redirects use 307; `url.Parse` rejects extra colons in hosts — [Go 1.26](https://go.dev/doc/go1.26).
- `go mod init` writes the previous minor's go line; `go tool doc` removed — [Go 1.26](https://go.dev/doc/go1.26).

**Go 1.27 (2026-08)**:
- Generic methods: methods can declare their own type parameters. They can't appear on interface methods or implement interface methods. Struct literal keys may be any field selector — [Go 1.27](https://go.dev/doc/go1.27).
- `encoding/json/v2` and `encoding/json/jsontext` are GA. v1 `encoding/json` is now implemented on v2: marshaling is preserved, error text may differ, Unmarshal is significantly faster. v2 rejects invalid UTF-8 and duplicate names by default. Opt out with `GOEXPERIMENT=nojsonv2` (temporary) — [Go 1.27](https://go.dev/doc/go1.27).
- New stdlib `uuid` package (an inference: it reduces the need for `github.com/google/uuid`); `crypto/mldsa` (FIPS 204); `bytes`/`strings.CutLast`; `url.URL.Clone`; `database/sql` `driver.RowsColumnScanner` — [Go 1.27](https://go.dev/doc/go1.27).
- `net/http`: `Response.Body.Close` drains unread content up to a limit (better connection reuse); HTTP/2 server honors RFC 9218 priorities; `Server.MaxHeaderValueCount`; `httptest.NewTestServer` runs on an in-memory network for `synctest`; `synctest.Sleep` — [Go 1.27](https://go.dev/doc/go1.27).
- `goroutineleak` pprof profile GA (`/debug/pprof/goroutineleak`); goroutine labels in tracebacks — [Go 1.27](https://go.dev/doc/go1.27).
- `go fix` adds `atomictypes`, `embedlit`, `slicesbackward` and `unsafefuncs`; `waitgroup` renamed to `waitgroupgo`; `fmtappendf` removed. Vet printf flags `%w` applied to a pointer-to-error-type value, which breaks `errors.Is`. `go doc pkg@version` — [Go 1.27](https://go.dev/doc/go1.27).
- `crypto/tls` `Config.Rand` deprecated (use `cryptotest.SetGlobalRandom` in tests) — [Go 1.27](https://go.dev/doc/go1.27).

### Inferences: rules that changed
- Logging: use `log/slog` (and `sloglint`) instead of logrus or zap for new services unless you need zap's performance. Prometheus enables `sloglint` — [Prometheus .golangci.yml](https://github.com/prometheus/prometheus/blob/main/.golangci.yml).
- Errors: drop `github.com/pkg/errors` (golangci-lint's own depguard bans it), use `fmt.Errorf("...: %w")` and `errors.Join`, and use `errors.AsType` from 1.26 on.
- Routing: stdlib `ServeMux` is enough for many services since 1.22; a third-party router is now a choice that needs a reason.
- JSON: use `omitzero` instead of `omitempty` for struct and time fields. Target json/v2 semantics on 1.27+ (strict UTF-8 and duplicate keys) and test error-message assertions, since v1 error text changed.
- Concurrency: use `wg.Go`, test with `testing/synctest`, check leaks with the `goroutineleak` profile or goleak, and drop `automaxprocs`.
- Files: use `os.Root` for any path derived from user input.
- Tests: use `t.Context()`, `b.Loop()`, `T.ArtifactDir`; no `tt := tt`.
- Toolchain upgrades: run `go fix ./...` and review the diff. Libraries can annotate deprecated APIs with `//go:fix inline`.

### Gaps
- I didn't fetch the 1.21, 1.22 and 1.23 release notes in this session; their claims come from well-known release contents with links to the official notes.
- The Go 1.27 notes page I fetched didn't mention changes to slog, iterators or `os.Root` in 1.27.
- I didn't check the exact `errors.AsType` signature (pkg.go.dev/errors).
- No Go 1.28 information was available; its release would be expected around February 2027 under the six-month cadence.
