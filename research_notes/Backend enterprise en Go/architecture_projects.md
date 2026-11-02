# Architecture and project layout for large Go backends, and exemplary open-source Go projects

Research date: 2026-11-01. Repository directory listings were read live through the GitHub API on that date, so they show each repo's default branch as of today. Wherever a claim is someone's opinion rather than official guidance or a broad consensus, the text says so.

## Official guidance: go.dev module layout, the golang-standards/project-layout debate, and cmd/, internal/, pkg/

### Takeaway
The only official guidance is go.dev/doc/modules/layout. For servers it says to put the logic in `internal/` and the binaries in `cmd/`, and it never mentions `pkg/`. golang-standards/project-layout is a community repo. Russ Cox (Go tech lead, posting as `rsc`) publicly objected to it calling itself a "standard". `pkg/` is a matter of taste and has no official backing. Big projects such as Kubernetes, CockroachDB, Grafana and etcd use it, but mostly for historical reasons.

### Cited findings
- Official: "It's recommended to keep packages in `internal` as much as possible." Code in `internal/` cannot be imported by other modules, so "we're free to refactor its API and generally move things around without breaking external users." — [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout)
- Official, servers: "Server projects typically won't have packages for export, since a server is usually a self-contained binary... it's recommended to keep the Go packages implementing the server's logic in the `internal` directory." It also says "it's a good idea to keep all Go commands together in a `cmd` directory" because servers usually have many non-Go directories. The example layout is `internal/{auth,metrics,model}` plus `cmd/{api-server,metrics-analyzer}/main.go`. — [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout)
- Official: when packages inside a server turn out to be useful to others, "it's best to split these off to separate modules." — [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout)
- Official: `cmd/` is "a common convention... while this isn't strictly necessary in a repository that consists only of commands, it's very useful in a mixed repository." The page does not mention `pkg/` at all. — [go.dev/doc/modules/layout](https://go.dev/doc/modules/layout)
- Russ Cox in golang-standards/project-layout issue #117: "the vast majority of packages in the Go ecosystem do not put the importable packages in a pkg subdirectory"; "what is described here is just very complex, and Go repos tend to be much simpler"; "It is unfortunate that this is being put forth as 'golang-standards' when it really is not." He suggested the README describe itself as "a set of common historical and emerging project layout patterns." — [github.com/golang-standards/project-layout/issues/117](https://github.com/golang-standards/project-layout/issues/117)
- Opinion, Peter Bourgon (2016): "Put library code under a pkg/ subdirectory. Put binaries under a cmd/ subdirectory." This is one of the main places the `pkg/` habit came from, and it predates modules and `internal/` guidance. — [peter.bourgon.org/go-best-practices-2016](https://peter.bourgon.org/go-best-practices-2016/)
- Opinion, Kat Zień: her GopherCon 2018 talk suggested `pkg/` for large repos with many non-Go assets ([Sourcegraph liveblog](https://about.sourcegraph.com/go/gophercon-2018-how-do-you-structure-your-go-apps)). In a later JetBrains interview she said `cmd` "doesn't really add any value, so you shouldn't add it by default" (quote as reported by search results; the JetBrains page itself was not fetched). — [blog.jetbrains.com/?p=335971](https://blog.jetbrains.com/?p=335971)
- In practice (directory listings, 2026-11-01):
  - Kubernetes: `cmd/ pkg/ plugin/ staging/ test/ vendor/ api/` — [github.com/kubernetes/kubernetes](https://github.com/kubernetes/kubernetes)
  - CockroachDB: everything under `pkg/` — [github.com/cockroachdb/cockroach](https://github.com/cockroachdb/cockroach)
  - Grafana: `pkg/` plus `apps/` — [github.com/grafana/grafana](https://github.com/grafana/grafana)
  - Terraform: only `internal/` at the top (no `cmd/`, no `pkg/`) — [github.com/hashicorp/terraform](https://github.com/hashicorp/terraform)
  - Prometheus: `cmd/` and `internal/` next to top-level domain packages (`promql/ tsdb/ scrape/ storage/ rules/ discovery/`) — [github.com/prometheus/prometheus](https://github.com/prometheus/prometheus)
  - Traefik: `cmd/ internal/ pkg/` — [github.com/traefik/traefik](https://github.com/traefik/traefik)
  - Caddy: `cmd/ internal/ modules/ caddyconfig/` — [github.com/caddyserver/caddy](https://github.com/caddyserver/caddy)
  - Gitea: top-level `cmd/ models/ modules/ routers/ services/` with no `internal/` or `pkg/` — [github.com/go-gitea/gitea](https://github.com/go-gitea/gitea)

### Inferences
- Consensus: put binaries under `cmd/<name>/main.go` when a repo has more than one binary or many non-Go directories, and use `internal/` for anything you don't want imported. Using `pkg/` is neither wrong nor endorsed. In a server-only module it adds nothing, because `internal/` already gives you the guarantee that matters.
- `pkg/` in Kubernetes and CockroachDB dates from before `internal/` was enforced (Go 1.5) and before modules existed. Copying it today is cargo-culting rather than following a best practice.
- Terraform moved almost everything into `internal/`, and that matches the current official advice best for a product that is not a library.

### Gaps
- I did not confirm when or why Terraform moved its code into `internal/`. I also did not check the current wording of the project-layout README disclaimer.
- I did not fetch the original JetBrains interview with Kat Zień. The quote comes from a search snippet.

## Architectural styles in Go and what each author recommends concretely

### Takeaway
Respected Go authors converge on a few points: the domain or business code must not depend on infrastructure, dependencies are passed in explicitly through constructors, wiring lives in `main`, there is no global state, and interfaces are small and defined by their consumer. They disagree about ceremony. Ben Johnson and Grafana organize packages by dependency or feature around domain root packages. Three Dots Labs uses explicit clean/hexagonal layers (`ports/app/adapters/domain`) and admits the boilerplate costs. Mat Ryer optimizes for readable, flat HTTP services with a `run` function and one big `NewServer` constructor. Every one of them rejects Rails-style package-by-layer (`controllers/`, `models/`).

### Cited findings
**Ben Johnson, "Standard Package Layout" (opinion, highly influential)**
- The four tenets are: 1) the "Root package is for domain types" (for example a `User` struct and a `UserService` interface), and "The root package should not depend on any other package in your application!"; 2) "Group subpackages by dependency" (`postgres/`, `http/`, each an adapter that implements the domain interfaces, and layers such as caches can wrap them); 3) "Use a shared mock subpackage," with hand-written mocks; 4) "Main package ties together dependencies," and it lives under `cmd/`. — [gobeyond.dev/standard-package-layout](https://www.gobeyond.dev/standard-package-layout/)
- He criticizes three alternatives. A monolithic package gets hard past ~10K SLOC. Rails-style grouping by type gives names like `controller.UserController` and leads to circular dependencies. Grouping by module gives stutter like `users.User` and cycles between modules such as `accounts` and `users`. — [gobeyond.dev/standard-package-layout](https://www.gobeyond.dev/standard-package-layout/)

**Mat Ryer, "How I write HTTP services in Go after 13 years" (Grafana blog, 2024; opinion)**
- `main` is tiny and calls `run(ctx, args, getenv, stdin, stdout, stderr) error`: "Operating system fundamentals are passed into run as arguments", which makes the whole program testable. — [grafana.com blog](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/)
- "NewServer is a big constructor that takes in all dependencies as arguments" and returns an `http.Handler`. Global middleware is applied there. — same source
- A single `routes.go` is "the one place in your service where all routes are listed," including per-route middleware. — same source
- Handler makers are functions that take their dependencies and return `http.Handler`, with the setup done in the closure. "If a handler function wants a dependency, it can bloody well ask for it as an argument." "I optimize for reading code, not writing it." — same source
- Generic encode/decode helpers, plus a one-method `Validator` interface (`Valid(ctx) map[string]string`) that is checked during decode. — same source
- He prefers end-to-end tests that start the real program through `run` with `t.Cleanup` and poll `/healthz` or `/readyz`, and he deletes unit tests that duplicate that coverage. — same source

**Three Dots Labs (Wild Workouts, "Go with the Domain"; opinion and teaching material)**
- DDD Lite has three rules: reflect business logic literally (types with behavior, not getter/setter bags), "always keep a valid state in memory" (private fields, validation in constructors), and keep the domain database-agnostic. — [threedots.tech/post/ddd-lite-in-go-introduction](https://threedots.tech/post/ddd-lite-in-go-introduction/)
- Their caveats: "not every project is complex enough to warrant advanced techniques like DDD", "Without Strategic patterns, I'd say you'll only get 30% of the advantages that DDD can offer", and "There is no quality vs. speed tradeoff in the long run." — same source
- Their clean-architecture layout per service is `ports/` (inbound HTTP, gRPC, Pub/Sub, CLI, i.e. hexagonal "primary adapters"), `app/` (use cases), `adapters/` (DB and outbound clients, i.e. "secondary adapters"), an optional `domain/`, and `main.go` for manual wiring. The dependency rule: "outer layers (implementation details) can refer to inner layers (abstractions), but not the other way around." — [threedots.tech/post/introducing-clean-architecture](https://threedots.tech/post/introducing-clean-architecture/)
- Go-specific choice: "Thanks to Go's implicit interfaces, we see no value in keeping a dedicated layer for them." Interfaces live next to the consumer (for example the app defines `trainingRepository`). — same source
- Costs they admit: the code "did grow in lines of code", which they call "an investment in loose coupling". One big service struct forced fat mocks, which they later fixed with CQRS command and query handlers. Their naming collides with the original hexagonal terms. They recommend the `go-cleanarch` linter to enforce boundaries. — same source
- Follow-ups: [Repository pattern in Go](https://threedots.tech/post/repository-pattern-in-go), [Basic CQRS in Go](https://threedots.tech/post/basic-cqrs-in-go/), [DDD + CQRS + Clean Architecture combined](https://threedots.tech/post/ddd-cqrs-clean-architecture-combined/), and the [series index](https://threedots.tech/series/modern-business-software-in-go/). The repo's top level today is `api/ docker/ internal/ sql/ terraform/ web/`. — [github.com/ThreeDotsLabs/wild-workouts-go-ddd-example](https://github.com/ThreeDotsLabs/wild-workouts-go-ddd-example)

**Peter Bourgon, "Go best practices, six years in" (2016; opinion, widely cited)**
- "Make dependencies explicit!" Use constructor injection and "many small interfaces to model dependencies" (for example a `Doer` instead of `*http.Client`). — [peter.bourgon.org/go-best-practices-2016](https://peter.bourgon.org/go-best-practices-2016/)
- No global state. Package globals like `log.Printf`, `http.Get` and `init` side effects are "awkward in the large". "Loggers are dependencies!" — same source
- "Only func main has the right to decide which flags are available to the user." Env vars are fine "provided each var is also defined as a flag." Use struct-literal initialization, useful zero values, and no-op defaults instead of nil checks. — same source

**Dave Cheney, "SOLID Go Design" (2016; opinion, widely cited)**
- Name packages for what they provide, and avoid `server`, `private`, `common` and `utils`. — [dave.cheney.net/2016/08/20/solid-go-design](https://dave.cheney.net/2016/08/20/solid-go-design)
- He calls "Accept interfaces, return structs" a good rule of thumb. Narrow parameters to the smallest interface the function needs (`*os.File` becomes `io.Writer`). — same source
- The import graph should be "wide, and relatively flat," and concrete decisions should move "as high as possible up the import graph" (toward `main`). — same source

**Kat Zień, GopherCon 2018 "How do you structure your Go apps?" (opinion)**
- She surveyed flat, layered, modular and DDD/hexagonal structures. Her conclusion was to group by context (DDD), not by function, and to stay consistent and simple. Flat suits small apps and has no risk of cycles. — [Sourcegraph liveblog](https://about.sourcegraph.com/go/gophercon-2018-how-do-you-structure-your-go-apps); [Enova writeup](https://innovation.enova.com/gophercon-2018-how-do-you-structure-your-go-apps/); talk materials at github.com/katzien/talks

### Inferences
- Consensus: package by feature or domain, with a dependency rule pointing inward and wiring in `main`. Package-by-layer is broadly rejected.
- Contested: how many layers, and whether to formalize hexagonal or clean terminology. Go's implicit interfaces remove the need for a separate "ports" package of interfaces, and Three Dots Labs and Ben Johnson both take advantage of that.
- "Vertical slices" in Go usually means one package per feature (Grafana's `pkg/services/<feature>`, Gitea's `services/<feature>`) more than the .NET-style one-file-per-use-case. Three Dots Labs' CQRS handlers (one struct per command or query) come closest to true vertical slices.

### Gaps
- I did not fetch the full "Go with the Domain" book or its current status (free ebook, possibly updated).
- I found no source that states a single canonical definition of "vertical slice architecture" in Go.

## Dependency injection: manual wiring vs google/wire vs uber-go/fx/dig

### Takeaway
Manual constructor injection wired in `main` (or in `run`/`NewServer`) is the community default and what Bourgon, Ryer, Johnson and Three Dots Labs teach. google/wire is now archived (25 Aug 2025) and unmaintained, but Grafana still depends on it heavily. uber-go/fx is actively maintained, powers Uber, and is used by Temporal server. It earns its keep when you have dozens of components with start/stop lifecycles shared across teams. The usual criticism of it is runtime reflection and errors at startup instead of compile time.

### Cited findings
- google/wire: the repo was "archived by the owner on Aug 25, 2025" and is read-only. The README says "This project is no longer maintained", "If you wish to update or extend wire, please do so in a fork", and that Wire is "considered feature complete". — [github.com/google/wire](https://github.com/google/wire)
- Wire is compile-time code generation (`wire_gen.go`) with no runtime container. — [Leapcell comparison (secondary source)](https://www.leapcell.io/blog/go-dependency-injection-approaches-wire-vs-fx-and-manual-best-practices)
- Grafana uses Wire. Its docs say "Wire represents dependencies between components as function parameters, which encourages explicit initialization instead of global variables." Each service exposes `ProvideService(deps...) (*Service, error)` and is registered in a `wire.NewSet` in `pkg/server/wire.go`. Background services implement `Run(ctx) error`, and services can implement `IsDisabled()` (the `registry.CanBeDisabled` interface). — [grafana/contribute/backend/services.md](https://github.com/grafana/grafana/blob/main/contribute/backend/services.md)
- uber-go/fx: "Fx is the backbone of nearly all Go services at Uber". It is pitched as removing global state and `init()` and as letting teams "build loosely-coupled and well-integrated shareable components". It is v1 and "follows SemVer strictly." — [github.com/uber-go/fx](https://github.com/uber-go/fx)
- Temporal server wires its services with fx: `service/fx.go` imports `go.uber.org/fx`, and the service packages are `frontend/ history/ matching/ worker/`. — [github.com/temporalio/temporal/tree/main/service](https://github.com/temporalio/temporal/tree/main/service)
- Manual DI is the approach in Three Dots Labs' clean-architecture post ("Dependencies are wired manually in main.go", with Wire deferred to later) — [threedots.tech](https://threedots.tech/post/introducing-clean-architecture/). Mat Ryer's `NewServer` takes all dependencies as arguments — [grafana.com blog](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/). Bourgon: "Make dependencies explicit!" — [peter.bourgon.org](https://peter.bourgon.org/go-best-practices-2016/)

### Inferences
- Manual wiring is justified almost always. It is compile-checked, greppable and has no dependency. Its cost is a long `main`/`run`, which you can split into per-module `New...` constructors.
- Wire made sense for very large graphs (Grafana has hundreds of services) because it keeps compile-time safety. Adopting it for a new project in 2026 is hard to justify now that it is archived. Existing users keep working, since the generated code is plain Go.
- fx/dig fits platform-style codebases with many teams, pluggable modules and lifecycle hooks (Uber, Temporal). The trade-off is reflection-based, startup-time errors and wiring that is harder to trace by reading code.

### Gaps
- I did not fetch the fx docs on `fx.Lifecycle` and `fx.Module`, or any primary critique of fx from a well-known author. Criticism of reflection and runtime errors is widely repeated, but I don't have a primary citation for it here.
- I did not verify whether Grafana has a plan to migrate off Wire.

## How real projects are structured and what to learn from them

### Takeaway
Exemplary large Go codebases share a few patterns. The product is one module, sometimes with separately published API/SDK modules. Services are packaged by feature. There is a central registry or wiring point. Storage is decorated in layers. Plugins are compiled in, registered at import time, or run out-of-process. Code generation is used heavily. None of them follows golang-standards/project-layout literally.

### Cited findings
- **Grafana**: "The Go packages in Grafana should be packaged by feature... retaining a clear sole ownership of a single domain", explicitly inspired by Ben Johnson's layout. The root package `pkg/services/<svc>` holds the interfaces, types and errors. The implementation goes in a sub-package named for uniqueness (for example `teaimpl`). Test doubles go in `<svc>test` (e.g. `teapottest`), and widely used services should ship an in-memory fake. Store and logic are separated, and the store interface is "not directly called from outside the service". Legacy exceptions: migrations live in `pkg/services/sqlstore/migrations` and API endpoints stay in `pkg/api`. — [grafana/contribute/backend/package-hierarchy.md](https://github.com/grafana/grafana/blob/main/contribute/backend/package-hierarchy.md). Today's `pkg/` contains `api apis apiserver bus infra registry server services setting storage ...`, which shows a migration toward Kubernetes-style API servers (`apis`, `apiserver`, `apimachinery`). — [github.com/grafana/grafana/tree/main/pkg](https://github.com/grafana/grafana/tree/main/pkg)
- **Kubernetes**: a monorepo with `staging/`, described as "the staging area for packages that have been split to their own repository... periodically published to respective top-level k8s.io repositories" (`k8s.io/api`, `apimachinery`, `client-go`, `apiserver`, `kubectl`, `kubelet`, and others). — [kubernetes/staging/README.md](https://github.com/kubernetes/kubernetes/blob/master/staging/README.md)
- **Mattermost**: the server lives in `server/` next to `webapp/` in one repo. `server/channels/` has `api4 app store web wsapi jobs`, which is layered (API, then app, then store). The store is decorated with generated layers: `localcachelayer retrylayer searchlayer timerlayer sqlstore` plus `layer_generators`. Enterprise code sits behind `einterfaces/`, with an `enterprise/` directory. Mattermost's AGENTS.md says not to use `context.Context` in store signatures and to use their own `request.CTX` instead. — [github.com/mattermost/mattermost/tree/master/server](https://github.com/mattermost/mattermost/tree/master/server); [server/AGENTS.md](https://github.com/mattermost/mattermost/blob/master/server/AGENTS.md)
- **Gitea**: the top level splits `models/` (DB entities), `modules/` (reusable infrastructure), `services/` (business logic per feature: `issue pull repository webhook mailer actions ...`) and `routers/` (HTTP). This is layered at the top but organized by feature inside `services/`. — [github.com/go-gitea/gitea](https://github.com/go-gitea/gitea)
- **Temporal server**: one repo builds several roles (`service/frontend`, `history`, `matching`, `worker`) that can run in one process or separately, wired with fx. `common/` holds shared infrastructure and `proto/` and `api/` hold the generated contracts. — [github.com/temporalio/temporal](https://github.com/temporalio/temporal)
- **Caddy**: a module system. "By importing the package, the module registers itself with the Caddy core." Module IDs are namespaced (`http.handlers.reverse_proxy`), and a namespace shares an interface. The lifecycle is load, then provision/validate, then use, then cleanup. Config reloads are atomic: new modules are provisioned and old ones are cleaned up only if all succeed. "Plugins can be added without modifying the Caddy code base at all" (they are compiled in). — [caddyserver.com/docs/architecture](https://caddyserver.com/docs/architecture)
- **HashiCorp**: Vault's top level has `api/ sdk/ builtin/ plugins/ physical/ command/ http/ vault/ internal/ serviceregistration/` ([github.com/hashicorp/vault](https://github.com/hashicorp/vault)). Consul has `agent/ api/ sdk/ internal/ proto/ proto-public/` ([github.com/hashicorp/consul](https://github.com/hashicorp/consul)). Nomad has `api/ client/ nomad/ scheduler/ drivers/ plugins/ internal/` ([github.com/hashicorp/nomad](https://github.com/hashicorp/nomad)). Terraform has only `internal/` at the top ([github.com/hashicorp/terraform](https://github.com/hashicorp/terraform)). The pattern: `api/` and `sdk/` are the public surfaces, and everything else is private.
- **Prometheus**: top-level domain packages (`promql tsdb scrape storage rules discovery notifier web`) plus `cmd/` and `internal/`. This is package-by-subsystem with no `pkg/`. — [github.com/prometheus/prometheus](https://github.com/prometheus/prometheus)
- **etcd**: `api/ client/ server/ pkg/ etcdctl/ etcdutl/ tests/`. — [github.com/etcd-io/etcd](https://github.com/etcd-io/etcd)
- **Traefik**: `cmd/ internal/ pkg/ integration/ webui/`. — [github.com/traefik/traefik](https://github.com/traefik/traefik)
- **CockroachDB**: everything under `pkg/` (`pkg/sql`, `pkg/kv`, and others), plus `c-deps/`. — [github.com/cockroachdb/cockroach](https://github.com/cockroachdb/cockroach)
- **Watermill** (Three Dots Labs library): `message/ pubsub/ components/ internal/ _examples/`. — [github.com/ThreeDotsLabs/watermill](https://github.com/ThreeDotsLabs/watermill)
- **Ent**: code generation from schema (`entc/`, `schema/`, `dialect/`, `privacy/`). — [github.com/ent/ent](https://github.com/ent/ent)
- **PocketBase**: the GitHub repo is now **archived**, and its README says "The repo has moved to https://ganigeorgiev.com/pocketbase/pocketbase." It is still "under active development" with no backward-compatibility guarantee before v1.0.0. Its layout is `apis/ core/ forms/ mails/ migrations/ plugins/ cmd/`, and it is designed to be embedded as a Go framework. — [github.com/pocketbase/pocketbase](https://github.com/pocketbase/pocketbase)
- **Encore**: "Encore recognizes this as a service, and uses the package name as the service name". "Building a microservices architecture is as simple as creating multiple Go packages within your application". "Encore automatically generates a `main` function." — [encore.dev/docs/go/primitives/services](https://encore.dev/docs/go/primitives/services)

### Inferences
- Patterns worth copying:
  - Grafana's root-package-plus-`impl`-plus-`test` convention.
  - Mattermost's generated store decorators for cache, retry, timing and search. They keep cross-cutting concerns out of business logic.
  - Caddy's interface-per-namespace module registry with provision/validate/cleanup, for anything pluggable.
  - The Kubernetes and HashiCorp habit of publishing only `api/` and `sdk/` and keeping the rest internal.
  - Temporal's single binary with several roles, which is a monolith deployable as several services.
- Nearly all of these projects lean on code generation: protobuf (Temporal, Consul), Wire (Grafana), store layers (Mattermost), Ent schemas, and Kubernetes code-generator.
- Most have accumulated legacy (Grafana's documented exceptions, Kubernetes' `pkg/`). Their layout records how they evolved and should not be copied as a template.

### Gaps
- I could not fetch Gitea's backend guidelines doc (the docs-repo path did not resolve). The description of Gitea's layering comes from the directory listing alone.
- I did not check whether Vault's and Consul's `api/` and `sdk/` are separate Go modules (I believe they are, but didn't verify).
- I did not research Forgejo separately, or HashiCorp go-plugin (out-of-process plugins over gRPC), with primary sources.
- I did not check whether PocketBase's new host is reliable or why the project moved.

## Monolith vs microservices, and modular monoliths in Go

### Takeaway
The Go community's leading voices treat microservices as a deployment choice that does not by itself bring good structure. They favor well-bounded modules inside one deployable, which can be split later. Temporal and Encore build this split-ability into the architecture itself.

### Cited findings
- Three Dots Labs: "Using microservices architecture and splitting services into small pieces is not enough" to keep code maintainable. — [threedots.tech/post/ddd-lite-in-go-introduction](https://threedots.tech/post/ddd-lite-in-go-introduction/)
- go-kit describes itself as a toolkit "for building microservices (or elegant monoliths) in Go." — [github.com/go-kit/kit](https://github.com/go-kit/kit)
- Encore: services are packages in one app. Splitting into microservices is "as simple as creating multiple Go packages within your application." — [encore.dev/docs](https://encore.dev/docs/go/primitives/services)
- Temporal runs `frontend/history/matching/worker` roles from one codebase. — [github.com/temporalio/temporal](https://github.com/temporalio/temporal)
- Grafana is a large modular monolith. Feature services in `pkg/services` have sole ownership and communicate through root-package interfaces, and the store is private to each service. — [grafana package-hierarchy.md](https://github.com/grafana/grafana/blob/main/contribute/backend/package-hierarchy.md)

### Inferences
- The concrete recipe for a Go modular monolith, drawn from Grafana plus Three Dots Labs:
  - one module per feature, with a public root package (interfaces, types, errors)
  - a private implementation and a private store
  - no cross-module imports of internals
  - wiring in `main`
  - optionally `internal/` per module, so the compiler enforces the boundaries
- Splitting into separate services is then mostly a wiring and transport change.

### Gaps
- I did not fetch a dedicated Three Dots Labs article on microservices vs monolith (they have one on "distributed monolith" anti-patterns), or a primary GopherCon talk on modular monoliths in Go.

## Frameworks and toolkits for whole platforms, and how the community sees full frameworks vs stdlib plus libraries

### Takeaway
The community default is the standard library (net/http with the Go 1.22+ pattern router) or a light router plus hand-picked libraries. Full-stack frameworks are a minority choice for platform backends. Gin is the most-used HTTP framework but is not a platform. Kratos and go-zero are popular, mostly in China. go-kit has effectively stagnated (no pushes since July 2024). Encore and Goa take a code-generation and design-first approach.

### Cited findings
- JetBrains Developer Ecosystem 2025: net/http is the most common routing choice, and since gorilla/mux was archived in 2023 many projects moved to chi or the stdlib, "especially after Go 1.22 introduced pattern routing in http.ServeMux". Gin is used by ~48% (41% in 2020), followed by Echo and Fiber. (Secondary write-up of JetBrains survey data.) — [blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025](https://blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025/)
- The official Go Developer Survey 2025 (5,379 responses, published January 2026) found 91% satisfaction. Respondents asked for help making the most of the standard library and for best practices. I found no framework breakdown in it. — [go.dev/blog/survey2025](https://go.dev/blog/survey2025)
- go-kit calls itself a "programming toolkit" (a set of packages and best practices), not a framework. As of 2026-11-01 it had ~27.4k stars, was last pushed 2024-07-19, and is not archived. — [github.com/go-kit/kit](https://github.com/go-kit/kit) (metadata via GitHub API)
- Kratos (go-kratos): its layout template uses `internal/{biz,data,service,server,conf}`, a DDD-ish layering where biz is the domain/use cases and data is the repositories. It has ~26k stars and is active. — [github.com/go-kratos/kratos-layout](https://github.com/go-kratos/kratos-layout)
- Repository activity as of 2026-11-01 (GitHub API metadata):
  - go-zero: ~33.4k stars, active — [github.com/zeromicro/go-zero](https://github.com/zeromicro/go-zero)
  - go-micro: ~23.1k stars, active — [github.com/micro/go-micro](https://github.com/micro/go-micro)
  - Beego: ~32.4k stars, active — [github.com/beego/beego](https://github.com/beego/beego)
  - Goa: ~6.1k stars, active — [github.com/goadesign/goa](https://github.com/goadesign/goa)
  - Encore: ~12.7k stars, active — [github.com/encoredev/encore](https://github.com/encoredev/encore)
  - Buffalo: ~8.4k stars, last push 2026-03-21 — [github.com/gobuffalo/buffalo](https://github.com/gobuffalo/buffalo)
- Buffalo pitches itself as "isn't just a framework; it's a holistic web development environment and project structure", in the Rails style. — [github.com/gobuffalo/buffalo](https://github.com/gobuffalo/buffalo)
- Encore generates `main` and the infrastructure wiring from annotated packages. — [encore.dev/docs](https://encore.dev/docs/go/primitives/services)

### Inferences
- Opinion that is close to consensus among well-known English-language Go authors (Ryer, Bourgon, Cheney, Johnson, Three Dots Labs): prefer the stdlib plus small libraries, with explicit wiring. None of the exemplary projects in this file (Kubernetes, Grafana, Mattermost, Gitea, Temporal, HashiCorp, Prometheus, etcd, Caddy, Traefik) is built on a full web framework.
- Platforms like go-zero and Kratos bring scaffolding (goctl, protoc plugins) and conventions, which helps large teams standardize. The costs are lock-in, codegen layouts and documentation centered on Chinese. Encore buys infrastructure automation at the cost of tying the app to its runtime and compiler model.
- go-kit's stagnation and Wire's archival both show that relying on heavyweight Go architecture tooling carries maintenance risk.

### Gaps
- I did not fetch a primary community sentiment piece (Reddit or HN threads, or a Go team statement) on frameworks. The sentiment stated above is inferred from what respected authors teach and what exemplary projects do.
- I did not verify GitHub star counts against other sources. They are API metadata, and their only purpose here is to show relative scale.
- I did not research the docs of go-micro, Beego or Goa in depth.
