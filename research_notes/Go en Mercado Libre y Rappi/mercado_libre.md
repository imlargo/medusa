# Go at Mercado Libre (Meli / Mercado Pago): backend platform, Fury, and Go practices

Research date: 2026-11-01. Each claim is dated by its source's publication date. "Official" means Mercado Libre's own engineering blog (medium.com/mercadolibre-tech, "Tecnología de Mercado Libre") or the company's GitHub org. "Vendor" means a case study written by a vendor (Google/Go team, AWS, Datadog, Google Cloud), usually with Meli staff quoted; these are semi-official and promotional. "Third-party" means anything else.

Access note: medium.com returned 403 to direct fetches. I read the Medium articles in full through a reader proxy (r.jina.ai). Citations point to the original Medium URLs.

---

## 1. Tech stack, how much Go, and the migration history

### Takeaway
Meli runs a polyglot platform. Java and Go are the two main backend languages, with Node.js, Python and Kotlin also officially supported. Go has carried about half of production traffic since at least 2019, and the company said in 2024 that the share was still growing. The move to Go began around 2015, when the Groovy/Grails API framework hit its limits. The rewrite reported roughly 88% fewer servers and a test suite 24x faster. That figure is from 2019 and has not been updated publicly since.

### Cited Findings
- **History (official, Feb 2024):** In 1999 the marketplace was a monolith: a few hundred physical servers, a single Oracle database and one repo shared by 200+ developers, with weekly deploys and code freezes. Around 2010 Meli moved to microservices on "MeliCloud", an internal IaaS (17,500 instances, 1,200+ traffic pools, 1,400+ instances deployed per day). Fury was born in 2015. — [The technological evolution at Mercado Libre (Feb 22, 2024)](https://medium.com/mercadolibre-tech/the-technological-evolution-at-mercado-libre-fb269776a4e8)
- The pre-microservices monolith was Java (third-party summary of a PlatformCon talk by Meli engineers Lucia Brizuela and Juliano Martins, c. 2022-2023). — [platformengineering.org talk page](https://platformengineering.org/talks-library/scaling-from-2k-engineers-to-12k-engineers-and-10k-deploys-per-day)
- **Why Go (vendor: Go team case study, Nov 10, 2019):** In 2015 the API framework, built on Groovy and Grails, was "reaching its limits". It required many developer decisions, Groovy's dynamic typing made scaling slower, and test, build and deploy times were slow. Meli moved from a heavy multi-layer Grails/Groovy stack on relational databases to "a thin Go framework" for APIs. — [MercadoLibre Grows with Go (go.dev, 2019)](https://go.dev/solutions/mercadolibre)
- **Results (same source, 2019, now old):**
  - The core API team's user APIs averaged 8-10M requests per minute (RPM) at under 10 ms per request.
  - Mission-critical Go APIs handled more than 20M RPM.
  - One large Go service handled 70,000 requests per machine using 20 MB of RAM.
  - Server count dropped from 32 to 4, and cores per server from 4 to 2: about 88% of servers eliminated, and CPU halved on the rest.
  - Builds ran 3x faster and the test suite 24x faster (90 s to 3 s).
  - About half of traffic was served by Go.
  - Entry-level engineers built APIs handling about 10M RPM.
  - Developers coming from Java, .NET or Ruby reached production level in Go within a few weeks.
  - "Go and Java" were described as the lingua francas of the infrastructure.

  — [go.dev/solutions/mercadolibre](https://go.dev/solutions/mercadolibre)
- **Go share (official, Aug 28, 2024):** "Approximately half of our productive traffic flows through Go applications, and this proportion is increasing." — [For devs, by devs: The story of the Mercado Libre Go toolkit (Aug 28, 2024)](https://medium.com/mercadolibre-tech/for-devs-by-devs-67a395a03c3f)
- **Official stacks (official, Jul 2022):** The Code Ecosystem team supports "each official Mercado Libre stack (Java, Go, Python, NodeJS, Android and iOS)". — [Code Ecosystem: Improving the development experience (Jul 12, 2022)](https://medium.com/mercadolibre-tech/code-ecosystem-improving-the-development-experience-30f7416bc4da)
- **Fury language support (official, 2024-2025):**
  - Apr 2024: Fury supported 10 application types and 11 programming languages. — [Safe but flexible: the DevEx-based code management culture (Apr 11, 2024)](https://medium.com/mercadolibre-tech/safe-but-flexible-the-devex-based-code-management-culture-at-mercado-libre-9b1dfde6f1b6)
  - Oct 2025: "Applications can be developed in Java, Node.js, Python, Golang, and other programming language supported by Fury", on both x86 and ARM (Graviton). — [30.000 deployments per day? (Oct 21, 2025)](https://medium.com/mercadolibre-tech/30-000-deployments-per-day-heres-how-we-operate-without-losing-our-minds-0eddc0480fb9)
- **Business-unit stacks (official, Jan 2026):** Mercado Envíos (logistics) teams work across "Java, Kotlin, TypeScript/JavaScript, Go". — [Mutation testing at Mercado Libre (Jan 14, 2026)](https://medium.com/mercadolibre-tech/mutation-testing-at-mercado-libre-from-pilot-to-everyday-guardrail-2ffe0a273f0e)
- **Profiling coverage (official, Mar 2026):** Continuous profiling covers "about 90% of the Java and Go microservices running in Fury", which indicates Java and Go are the two dominant backend runtimes. — [Continuous profiling at Mercado Libre (Mar 11, 2026)](https://medium.com/mercadolibre-tech/continuous-profiling-at-mercado-libre-turning-flamegraphs-into-fixes-2ee371c32bfd)
- **Re:Invent 2024 templates (third-party summary):** A summary of the talk lists Fury app templates as Node.js, Java, Kotlin and Python and does not mention Go. That is probably an omission by the summarizer, given the official sources above. — [zenn.dev summary of AWS re:Invent 2024 talk by Marcos Pinheiro (Meli)](https://zenn.dev/kiiwami/articles/2449391f7fbaedd2?locale=en)

### Inferences
- The migration did not replace Java everywhere. Go took over the high-throughput, I/O-bound API layer, especially the core APIs and Mercado Pago, while Java (and now Kotlin) stayed strong. Streams, for example, has a 2025 post about migrating to Java virtual threads; I saw only its title, linked from the KVS article.
- The "half of traffic in Go" figure was stable from 2019 to 2024. Go's share probably grew in absolute terms more than in relative terms.

### Gaps
- No public count of Go services or Go repos versus Java ones.
- No source documents a Node.js-to-Go migration. Node.js appears only as a supported stack.
- The 2019 performance numbers (32 to 4 servers, and so on) have no updated equivalent.

---

## 2. Fury, the internal developer platform

### Takeaway
Fury (2015-present) is a self-service internal developer platform (IDP). A web UI and the Fury CLI create a GitHub repo, CI/CD pipeline, scaffolding, container image, monitoring and alerts in a few clicks. Apps run in "Scopes" with selectable deploy strategies on a multicloud compute layer, now mostly EKS/Kubernetes. Teams consume managed platform services (KVS, SQL, document DB, search, messaging/streams, secrets, object storage, audits, event-sourcing) through official SDKs. Nobody is forced to use the SDKs. Apps only have to comply with platform rules.

### Cited Findings
- **Definition and origin (official, Feb 2024):** Fury is an "Internal Developer Platform created by Mercado Libre that provides developers with a unified and self-service environment to efficiently create, deploy, manage, and monitor applications". It was born in 2015 with the goal of an app "up and running in just three clicks". It offers a defined stack, containerization, cloud-provider abstraction (multicloud: AWS, GCP), cost insights and integrated monitoring. All business units use it: marketplace, fintech, shops/ERP, logistics and ads. — [Technological evolution (Feb 2024)](https://medium.com/mercadolibre-tech/the-technological-evolution-at-mercado-libre-fb269776a4e8)
- **Scaffolding (official, Feb 2024):** Creating an app means picking an application type and technology in the Fury UI. The platform then "automatically generates a GitHub code repository with the necessary team permissions, sets up the CI/CD pipeline, and provides a basic scaffolding with a preconfigured containerized image". The Fury CLI lets developers download, run and test the app locally and create versions. — [same](https://medium.com/mercadolibre-tech/the-technological-evolution-at-mercado-libre-fb269776a4e8)
- **2019 description (vendor):** Fury is a PaaS "for building, deploying, monitoring, and managing services in a cloud-agnostic way", providing "templates, GitHub starter repositories, Docker images, and deployment pipelines". — [go.dev (2019)](https://go.dev/solutions/mercadolibre)
- **Scopes (official, 2024-2025):**
  - Scopes are flexible environments (several test scopes, demo, production). Creating one provisions the load balancer, autoscaling and instances, plus logs, metrics and alerts. Critical apps are split into separate scopes for blast-radius isolation. — [Technological evolution (2024)](https://medium.com/mercadolibre-tech/the-technological-evolution-at-mercado-libre-fb269776a4e8)
  - Scope app types: web services, scheduled jobs, workqueues, messaging services, streams. Criticality levels: test, low, medium, high, company critical, BU critical. — [30k deployments (Oct 2025)](https://medium.com/mercadolibre-tech/30-000-deployments-per-day-heres-how-we-operate-without-losing-our-minds-0eddc0480fb9)
- **Release Process (official):**
  - Proprietary CI/CD since 2018. It ran 7 quality checks (dependencies, branching model, CI, code coverage, hardcoded credentials, and others) across 26,000+ repos as of 2024, and a failed check stops the flow. — [Technological evolution (2024)](https://medium.com/mercadolibre-tech/the-technological-evolution-at-mercado-libre-fb269776a4e8)
  - Stable-branch criteria: high coverage, passing tests, only secure and stable libraries, no hard-coded secrets, minimal code smells. This applies whether the stack is Java or Go and the branching model is gitflow or trunk-based. — [Safe but flexible (Apr 2024)](https://medium.com/mercadolibre-tech/safe-but-flexible-the-devex-based-code-management-culture-at-mercado-libre-9b1dfde6f1b6)
- **Build is separate from deploy (official, Oct 2025):** The CI build runs unit tests, dependency validation, vulnerability scanning and linters, and produces an immutable Docker image in an internal registry. Deploys reuse that artifact across scopes and configurations without rebuilding.
  - Strategies: Blue-Green, Canary / Canary-Safe, Rolling Update, All-In, and Initial (used for new scopes).
  - Config and env-var changes count as deploys.
  - Automatic redeploys roll out sidecar security updates and resource right-sizing.
  - Every deploy step is idempotent and retryable.
  - Alerts are tiered by scope criticality: test scopes get a 1-2 day SLA, while critical production pages the on-call rotation, with automatic escalation.

  — [30k deployments (Oct 2025)](https://medium.com/mercadolibre-tech/30-000-deployments-per-day-heres-how-we-operate-without-losing-our-minds-0eddc0480fb9)
- **Compute internals (third-party summary of AWS re:Invent 2024, speaker Marcos Pinheiro):**
  - 130+ EKS clusters. A "Compute API" per cluster translates Fury objects into Kubernetes objects. Fury uses ReplicaSets rather than Deployments.
  - Compute shards are grouped by criticality and scope type.
  - Karpenter handles nodes, with Spot ratios per cluster. FluxCD manages cluster config. Clusters are replaced rather than upgraded in place.
  - The earlier model ran one replica per EC2 instance with Docker Compose sidecars. The migration to EKS was transparent to developers, and Lambda and Batch were rejected because 35,000 services could not be changed.
  - Scaling: HPA (CPU, predictive CPU, RPS), VPA recommendations through "Sizer", scale-to-zero, and a "Right-sizer".
  - "Safe" deploy compares metrics and rolls back automatically. Over 30 self-service services, 99.95% uptime, more than 30% lower compute cost.
  - Developers write no YAML.

  — [zenn.dev re:Invent 2024 summary](https://zenn.dev/kiiwami/articles/2449391f7fbaedd2?locale=en)
- **Kubernetes and Karpenter (official, Oct 2025):**
  - Kubernetes is "the foundation of our multi-cloud strategy, powering our internal compute engine".
  - Average pod scheduling time is about 50 s.
  - NodePools are named by convention, for example `user_application-spot-amd64-cpu_optimized`.
  - Spot policy: up to 10% for critical workloads, 70% for medium, 100% for test.
  - GitOps with FluxCD, plus an internal governance platform called "Automatrix".

  — [Scaling Kubernetes at Mercado Libre with Karpenter and GitOps (Oct 28, 2025)](https://medium.com/mercadolibre-tech/scaling-kubernetes-at-mercado-libre-with-karpenter-and-gitops-2c792a7403c5)
- **Platform services (official, Aug 2024):** "SQL databases, key-value stores, document databases, free text search, messaging, secrets storage, object storage, audits, and more… We even have design pattern implementations as services, like event-sourcing." — [For devs, by devs (2024)](https://medium.com/mercadolibre-tech/for-devs-by-devs-67a395a03c3f)
- **Common tooling (official, Apr 2024):** Fury CLI, Artifact Repositories, Official SDKs for Fury services, Release Process, Building System, Compute Service, Deployment Service, Logging Service. Almost 1,000 developers (7% of engineering) work on Fury itself. — [Safe but flexible (2024)](https://medium.com/mercadolibre-tech/safe-but-flexible-the-devex-based-code-management-culture-at-mercado-libre-9b1dfde6f1b6)
- **Voluntary adoption:**
  - Third-party summary of the PlatformCon talk: Fury's goals were saving money, governance and control, and security. Adoption was promoted as voluntary. — [platformengineering.org](https://platformengineering.org/talks-library/scaling-from-2k-engineers-to-12k-engineers-and-10k-deploys-per-day)
  - Official: "Fury is a platform flexible enough that no one is forced to use any particular library… The only requirement is that your app complies with certain rules." — [For devs, by devs](https://medium.com/mercadolibre-tech/for-devs-by-devs-67a395a03c3f)
- **Off-Fury share (third-party academic article, Oct 19, 2026):** About 20% of infrastructure remains "Off-Fury" (managed with IaC). It also lists five deploy strategies. — [E&S Journal: High availability in microservices: the case of Mercado Livre's Fury platform (2026)](https://revistaes.com.br/artigos/alta-disponibilidade-em-microsservicos-o-caso-da-plataforma-fury-do-mercado-livre?lang=en). The authors used secondary sources plus a survey of 20 respondents, so treat it as weak evidence.

### Inferences
- Fury follows the "paved road" model. It gives defaults (templates, SDKs, pipelines, observability) and escape hatches, and enforces only security and compliance gates in the Release Process and the traffic sidecar.
- For a Go engineer, the "framework" at Meli is mostly the platform: lifecycle, health, auth, metrics and traces are provided by Fury, sidecars and toolkit bootstrap code. Application code stays thin.

### Gaps
- No public documentation of the exact contents of the Go service template (directory layout, Makefile, Dockerfile).

---

## 3. Go-specific: open-source repos, internal toolkit, standards, testing

### Takeaway
Meli's important Go code is internal. The public GitHub org has only a few small or legacy Go repos. Internally, a Mercado Pago team built a versioned, stdlib-idiomatic toolkit split into **go-core** (generic: web server, HTTP clients with retries and backoff, circuit breaker, round-trippers, logging, metrics and tracing) and **go-platform** (Fury-specific bootstrap and observability). It replaced older unversioned Fury SDKs and a Gin-based router, and spread through voluntary adoption. A new version was in the works in 2024, using generics and OpenTelemetry. The company's Go guidance favors the standard library (`database/sql`) and is skeptical of ORMs.

### Cited Findings
- **Public Go repos (official GitHub, checked Nov 2026):**
  - `golang-restclient` (154 stars, last updated May 2022)
  - `flamepool` (28)
  - `piper` ("minimal pipeline library", 12)
  - `iso8583` (ISO 8583 card-message marshaling, 2023)
  - `golang-sdk` (archived 2021)
  - `pla` (load testing tool, archived)
  - `structs` (archived)
  - Forks of `istio-api` and `istio-istio` (updated Aug 2026)

  — [github.com/mercadolibre (Go repos)](https://github.com/orgs/mercadolibre/repositories?language=go&type=all)
- **golang-restclient (official, old, about 2016-2022):** A REST client with response caching driven by cache-control, etag and expires headers (TTL + LRU, max byte size), built-in mockups for tests, fork-join and async request patterns, retries, gzip, and automatic JSON/XML marshaling. Its README is partly in Spanish ("clone en tu gopath"), a sign of its pre-modules age. — [golang-restclient README](https://github.com/mercadolibre/golang-restclient)
- **golang-sdk (official):** The public marketplace API SDK. It is outdated, and its README says SDK maintenance stopped in April 2021 (from a search snippet; I did not open the README). — [github.com/mercadolibre/golang-sdk](https://github.com/mercadolibre/golang-sdk)
- **Mercado Pago public SDK:** `mercadopago/sdk-go` requires Go 1.18+ (from a search snippet). — [github.com/mercadopago/sdk-go](https://github.com/mercadopago/sdk-go)
- **"go-meli-toolkit" (vendor, 2019):** In 2019 Meli was developing "Go-meli-toolkit" to interface with Fury services. — [go.dev (2019)](https://go.dev/solutions/mercadolibre). I found no public repo named go-meli-toolkit or fury_go-core. Both are internal.
- **Internal toolkit story (official, Aug 28, 2024, author Sebastián Orfino):**
  - **Before:** "We weren't native to Go, with only some SDKs that allowed us to use Fury's services and a somewhat opinionated router based on Gin." Those SDKs were not versioned.
  - **The team:** Development started with 4 people at Mercado Pago, later 5-6 at most.
  - **Server requirements:**
    - register endpoints and handlers
    - remove repetitive code (marshaling, unmarshaling, error encoding, parameter parsing)
    - support custom middleware
    - graceful shutdown on OS signals
    - liveness signal
  - **Client requirements:** HTTP clients with retry policies, backoff and timeouts, "perhaps a circuit breaker", and metrics and traces at all levels.
  - **Fintech constraint:** Mercado Pago runs some computation in a segmented network that is not Fury, so the toolkit had to work there too.
  - **The split:**
    - **go-core:** "circuit breakers, HTTP clients with retries, round-trippers to customize request/response, tracing and metrics, a web server with some opinions, logging, etc."
    - **go-platform:** "bootstrapping the web server and initializing observability components according to the platform."
  - **Principles:**
    - libraries must be idiomatic: "a custom library shouldn't differ much from the standard library… developers shouldn't even notice"
    - strict semver
    - Hyrum's Law awareness
    - design the public API first, which takes the most iterations
  - **Workload:** Typical apps are I/O-bound (receive requests, make requests, transform or aggregate, respond).
  - **Adoption:** "No one… was forced to adopt… There was no directive." Adoption spread beyond Mercado Pago because of API ergonomics and stability, and the toolkit became a community asset.
  - **Next version (as of Go 1.22.2):** Use generics (absent at the start), fix design mistakes, and drop the observability abstraction because the platform is moving to OpenTelemetry. Also support non-web apps such as sidecars.

  — [For devs, by devs (Aug 28, 2024)](https://medium.com/mercadolibre-tech/for-devs-by-devs-67a395a03c3f)
- **Database access guidance (official, Jul 2022, Code Ecosystem team, Fernando Eugenio Correa):**
  - Meli scanned all production Go repos for the most-used DB packages. The Oct 20, 2021 "top 3" was shown as a chart that I could not read.
  - Packages reviewed: database/sql, dbr, goqu, sqlx, dbq, rel, ent, GORM.
  - Raw SQL pros: runtime performance, SQL is a standard skill, `database/sql` is "a standard in the Go community". Cons: query management and CRUD boilerplate.
  - ORM pro: development speed. Cons: slower, "too invasive and 'magical' (Golang community doesn't like it)".
  - The author chose raw `database/sql` for his own ramp-up project. The conclusion was a team-by-team choice rather than a mandate.

  — [Using Go at Meli: ORM or !ORM (Jul 12, 2022)](https://medium.com/mercadolibre-tech/using-go-at-meli-orm-or-orm-ba528b0f6f7f); [Go Language, Relational Databases and ORMs (Jul 12, 2022)](https://medium.com/mercadolibre-tech/go-language-relational-databases-and-orms-682a5fd3bbb6)
- **Onboarding (official, 2022):** New engineers do a "ramp-up": building a simple app that touches the day-to-day tooling. — [same](https://medium.com/mercadolibre-tech/go-language-relational-databases-and-orms-682a5fd3bbb6)
- **Code Ecosystem team (official, 2022):** The team maintains Docker images for the Python, Java and Go stacks. It builds "compliant toolkits w/RFC cross company", such as the KVSClient and Restclient for Python, and runs a dependency catalog, open-source license analysis, and "Sync Tech" (a knowledge-sharing livestream every two weeks). — [Code Ecosystem (2022)](https://medium.com/mercadolibre-tech/code-ecosystem-improving-the-development-experience-30f7416bc4da)
- **Testing practices (official):**
  - CI requires unit tests, coverage thresholds, linters and dependency/vulnerability scans. — [30k deployments (2025)](https://medium.com/mercadolibre-tech/30-000-deployments-per-day-heres-how-we-operate-without-losing-our-minds-0eddc0480fb9); [Technological evolution (2024)](https://medium.com/mercadolibre-tech/the-technological-evolution-at-mercado-libre-fb269776a4e8)
  - Mercado Envíos added mutation testing through one multi-stack GitHub Action embedded in Fury pipeline templates. The source says Java/Kotlin/Go map to PITest and TS/JS to Stryker (PITest is a JVM tool, so the Go mapping looks odd). The rollout ran delta-only on changed files, started in dry-run mode with PR comments, and turned on team-by-team gates with a labeled bypass for hotfixes: "indicator first, gate later". — [Mutation testing (Jan 14, 2026)](https://medium.com/mercadolibre-tech/mutation-testing-at-mercado-libre-from-pilot-to-everyday-guardrail-2ffe0a273f0e)
- **Go performance tooling (official, Mar 2026):**
  - Continuous profiling copies Go's pattern of exposing pprof profiles on internal HTTP routes and scraping them from a sidecar.
  - pprof is the normalized format for all languages.
  - Only one pod per HA cluster is profiled, by leader election, for a few seconds every few minutes. That yields 4.0-4.5M profiles per day, viewed in Grafana with differential flamegraphs.
  - An AI assistant explains the profiles, and auto-PRs fix known patterns such as missing GC tuning.
  - Production profiles feed Go PGO (profile-guided optimization) builds.

  — [Continuous profiling (Mar 11, 2026)](https://medium.com/mercadolibre-tech/continuous-profiling-at-mercado-libre-turning-flamegraphs-into-fixes-2ee371c32bfd)
- **Go in the traffic layer (official, May 2024):** Traffic authorization was first implemented as Nginx sidecar validations "with Lua and Golang customizations", later moved to Envoy RBAC filters. — [Internal Traffic: What a crazy airport! (May 9, 2024)](https://medium.com/mercadolibre-tech/internal-traffic-what-a-crazy-airport-3c9281831df9)

### Inferences
- Splitting the toolkit into a portable core and a platform-specific layer is the most reusable architectural lesson for a Go shop. Keep platform opinions (bootstrap, env, observability wiring) out of the generic library.
- Meli's toolkit shape (stdlib-like HTTP server, resilient clients built from round-trippers, circuit breaker, semver) suggests they moved away from Gin-style frameworks toward a thinner `net/http`-compatible core. The article implies this but never states which router the new toolkit uses.

### Gaps
- No public Go style guide, linter configuration (for example golangci-lint rules) or project-layout standard.
- No public talks found from GopherCon Latam or Golang Argentina meetups. A search returned nothing specific. The 2019 Go case study says Meli sends engineering leaders to GopherCon yearly and runs Go workshops for external developers. — [go.dev (2019)](https://go.dev/solutions/mercadolibre)
- Whether the internal toolkit's web server is built on `net/http` ServeMux, chi or something custom is not stated.

---

## 4. Architecture: traffic, mesh, data, messaging, resilience, observability

### Takeaway
External traffic enters through a central Traffic Layer. Service-to-service traffic now goes through a per-app sidecar (Nginx, later Envoy) with mTLS identity, declarative authorization ("Traffic Authorization") and decentralized routing ("Traffic Routes"), on an Istio control plane. Data lives in managed Fury services: KVS (DynamoDB-backed since 2018), MySQL, Spanner, document DB, search, Streams/queues. Observability is standardizing on OpenTelemetry, alongside Datadog, New Relic, Grafana and Opsgenie.

### Cited Findings
- **Traffic (official, May 2024):**
  - Internal traffic alone peaks above **400M RPM**, across 30k+ applications in 75k+ instance groups.
  - The old design was a centralized Traffic Layer for all hops, including internal ones, with 100K+ routes in one central configuration. Many apps had no authentication.
  - Identity went through three versions: a signed custom token, then a JWT, then mTLS in the sidecar, with an internal certificate authority (CA) that issues certificates at scope creation and renews them on each deploy.
  - Traffic Authorization: developers declare the endpoints, methods and domains they expose and consume. Enforcement moved from Nginx+Lua/Go to Envoy RBAC filters.
  - Traffic Routes: each team owns its domain's path-based routing, with header, query and method conditions, and can test routes before production.
  - The service mesh uses an **Istio** control plane, chosen over building one in-house.

  — [Internal Traffic (May 2024)](https://medium.com/mercadolibre-tech/internal-traffic-what-a-crazy-airport-3c9281831df9)
- **KVS (official, Oct 1, 2025):**
  - Over 642M operations per minute (572M reads, 70M writes), used by 7,600+ apps (about 25% of all apps), storing over 9 PB.
  - Built on "a fully managed NoSQL solution".
  - Variants:
    - Database: strong consistency, TTL, optimistic locking
    - Cache: eventual consistency
    - Counter: atomic counters
    - Vault: encrypted and audited storage
    - QKVS: queryable, with indexes
  - SDKs for Go, Node and Python.
  - Integrations: Streams, Schemas validation, Audits, Cold Storage tiering.
  - Usage limits on item size, key distribution and TTL.

  — [KVS: The key-value storage engine that powers the Fury ecosystem (Oct 1, 2025)](https://medium.com/mercadolibre-tech/kvs-the-key-value-storage-engine-that-powers-the-fury-ecosystem-473829d2318e)
- **KVS backend (third-party summary):** It moved from Cassandra to DynamoDB in 2018 without SDK changes. — [zenn.dev re:Invent 2024 summary](https://zenn.dev/kiiwami/articles/2449391f7fbaedd2?locale=en)
- **Database scale (vendor, AWS, undated, probably 2022-2023):** "17,000 databases and 56,000 Data Services that generate 900 million requests per minute between all communications from the approximately 24 thousand microservices". The page also cites peaks over 2.2M transactions per second. — [AWS Innovators: Mercado Libre](https://aws.amazon.com/solutions/case-studies/innovators/mercado-libre/)
- **Spanner (vendor, Google Cloud, Nov 4, 2025):**
  - Fury is called "our in-house middleware platform". Spanner is one database choice within Fury, used for orders, payments and inventory, plus some KV-style workloads.
  - Scale: about 214K QPS and 30K TPS, on 400+ nodes.
  - Data Boost with BigQuery Federation for analytics, and Reverse ETL back into Spanner.
  - Spanner Change Streams with Dataflow feed BigQuery and "Fury Stream" for event-driven integrations.

  — [Inside Mercado Libre's multi-faceted Spanner foundation (Nov 4, 2025)](https://cloud.google.com/blog/topics/retail/inside-mercado-libres-multi-faceted-spanner-foundation-for-scale-and-ai)
- **Streams (official, Jun 2024):**
  - The Fury Stream Service carries about 30M messages per minute to queues, KVS and BigQuery, including database CDC.
  - It had about 600 lag alerts in late 2023.
  - The team built a "worker-balancer" that hands out rate permits to workers based on reported load, using an async in-memory buffer flushed every minute through a queue. Lag alerts dropped 95%.

  — [Workload Distribution Dynamics (Jun 24, 2024)](https://medium.com/mercadolibre-tech/workload-distribution-dynamics-dc947eade032)
- **Kafka (third-party, weak):** A LinkedIn post claims Saga, outbox and event sourcing over Kafka at Meli. I could not verify it. I found no official source naming Kafka or "BigQueue" as a Fury service. The official messaging terms are "messaging", "workqueues", "Streams" and event-sourcing-as-a-service.
- **Cell-based architecture (official, Dec 29, 2025):**
  - Mercado Envíos' stock system sat on Meli's second-largest MySQL instance, serving all of Latin America. It had hit vertical-scaling limits by early 2024 (no larger instance sizes, CPU and I/O maxed out).
  - The team kept an "intentional monolith" core for latency and consistency, and split it into 3 cells by country group with read/write separation.
  - Legacy entities without a routing key are resolved by a "broadcast gateway" that queries all cells; if every cell returns 404, the entity does not exist. New entities get deterministic IDs that act as routing hints.
  - Response time dropped about 70%. Fury templates made the cells uniform.

  — [From a single point of failure to a cell-based architecture (Dec 29, 2025)](https://medium.com/mercadolibre-tech/from-a-single-point-of-failure-to-a-cell-based-architecture-how-we-scaled-mercado-env%C3%ADos-stock-528f581fb71b)
- **Observability (official, Jul 5, 2024):**
  - A dedicated Observability platform team.
  - Volumes: 40 TB of logs per day, 200M+ spans per minute, 250M+ metrics per minute.
  - The team **adopted OpenTelemetry** for vendor neutrality and shared semantics, and added distributed tracing and profiling as Fury signals.
  - "First Pane of Glass" view; the logging pipeline was redesigned.
  - Quotas, sampling and retention tiers control cost.
  - In payments alone, 100+ microservices and 1,000+ operations can be involved in a single flow.

  — [Building a large-scale Observability Ecosystem (Jul 5, 2024)](https://medium.com/mercadolibre-tech/building-a-large-scale-observability-ecosystem-1edf654b249e)
- **Observability tools:**
  - Fury exposes DataDog, NewRelic and Opsgenie with standard monitors per service. — [Technological evolution (2024)](https://medium.com/mercadolibre-tech/the-technological-evolution-at-mercado-libre-fb269776a4e8)
  - Grafana hosts logs, metrics, traces and profiles. — [Continuous profiling (2026)](https://medium.com/mercadolibre-tech/continuous-profiling-at-mercado-libre-turning-flamegraphs-into-fixes-2ee371c32bfd)
  - Datadog is embedded in Fury so every new app has monitoring from the start (vendor, Nov 14, 2025). — [Datadog case study](https://www.datadoghq.com/case-studies/mercado-libre/)
- **Resilience patterns:**
  - Retries, backoff, timeouts and circuit breakers live in go-core. — [For devs, by devs](https://medium.com/mercadolibre-tech/for-devs-by-devs-67a395a03c3f)
  - Blast-radius isolation through scopes and cells; idempotent deploy steps. — [30k deployments](https://medium.com/mercadolibre-tech/30-000-deployments-per-day-heres-how-we-operate-without-losing-our-minds-0eddc0480fb9)
  - Spreading across availability zones with topologySpreadConstraints. — [Karpenter (2025)](https://medium.com/mercadolibre-tech/scaling-kubernetes-at-mercado-libre-with-karpenter-and-gitops-2c792a7403c5)
  - The 2026 E&S survey lists circuit breakers, retry with jitter, bulkheads and rate limiting as primitives offered by the platform (third-party, n=20). — [E&S Journal](https://revistaes.com.br/artigos/alta-disponibilidade-em-microsservicos-o-caso-da-plataforma-fury-do-mercado-livre?lang=en)

### Inferences
- Cross-cutting concerns (authentication, authorization, mTLS, routing, retries at the mesh level) are pushed into sidecars. That is why the Go toolkit can stay small and why the 2024 toolkit rewrite drops its own observability abstraction in favor of OpenTelemetry.
- Event-driven work is platform-mediated (Streams with CDC, workqueues, event-sourcing service) rather than each team running Kafka clients directly. This is an inference; the Kafka details are unverified.

### Gaps
- Public API gateway product details beyond "Traffic Layer" are not documented.
- No official multi-region topology was found, only multi-AZ and multicloud (AWS, GCP, on-prem).
- The names and implementations of the queue and messaging services are not public. "BigQueue" could not be verified.

---

## 5. Scale numbers (dated)

### Takeaway
As of 2025-2026: about 35,000 microservices, about 30,000 deploys per day, 900M RPM total (400M+ internal), peaks of 15M requests per second, and 16,000-20,000+ people in tech. Figures vary by source and by how each one counts.

### Cited Findings
| Metric | Value | Date | Source |
|---|---|---|---|
| Developers | 2,800+ | Nov 2019 | [go.dev](https://go.dev/solutions/mercadolibre) (vendor) |
| Microservices / engineers / deploys | 26k / 15k / 19k per day; 25k PRs per week | Apr 2024 | [Safe but flexible](https://medium.com/mercadolibre-tech/safe-but-flexible-the-devex-based-code-management-culture-at-mercado-libre-9b1dfde6f1b6) (official) |
| Tech headcount growth | 200 to 15,000 (2011 to 2024) | Apr 2024 | same |
| Microservices / instances / engineers | 30k+ / 100k instances / 15k+ | Feb 2024 | [Technological evolution](https://medium.com/mercadolibre-tech/the-technological-evolution-at-mercado-libre-fb269776a4e8) (official) |
| Internal traffic peak | 400M+ RPM; 30k+ apps; 75k+ instance groups | May 2024 | [Internal Traffic](https://medium.com/mercadolibre-tech/internal-traffic-what-a-crazy-airport-3c9281831df9) (official) |
| Engineers / deploys / traffic | 2k to 12k engineers; 10k deploys/day; "50M requests per second" (inconsistent with other figures) | c. 2022-23 | [platformengineering.org](https://platformengineering.org/talks-library/scaling-from-2k-engineers-to-12k-engineers-and-10k-deploys-per-day) (third-party) |
| re:Invent 2024 | 35k microservices; 23k deploys/day; 100k+ PRs/day; 900M+ RPM; 16k IT out of 60k employees; 130+ EKS clusters | Dec 2024 | [zenn.dev](https://zenn.dev/kiiwami/articles/2449391f7fbaedd2?locale=en) (third-party summary) |
| Deploys | 30k/day; 750k+ in July 2025 | Oct 2025 | [30k deployments](https://medium.com/mercadolibre-tech/30-000-deployments-per-day-heres-how-we-operate-without-losing-our-minds-0eddc0480fb9) (official) |
| Kubernetes scale | 35k microservices; 20k+ tech staff; about 30k deploys/day; about 120k PRs/day; 15M requests/s peak | Oct 2025 | [Karpenter](https://medium.com/mercadolibre-tech/scaling-kubernetes-at-mercado-libre-with-karpenter-and-gitops-2c792a7403c5) (official) |
| Datadog | 900M RPM typical; 30k+ microservices; 200k+ pods; 36k deploys/day; Black Friday +20-40% | Nov 2025 | [Datadog](https://www.datadoghq.com/case-studies/mercado-libre/) (vendor) |
| KVS | 642M ops/min; 7,600 apps; 9 PB | Oct 2025 | [KVS](https://medium.com/mercadolibre-tech/kvs-the-key-value-storage-engine-that-powers-the-fury-ecosystem-473829d2318e) (official) |
| Streams | about 30M messages/min | Jun 2024 | [Workload Distribution](https://medium.com/mercadolibre-tech/workload-distribution-dynamics-dc947eade032) (official) |
| Observability | 40 TB logs/day; 200M spans/min; 250M metrics/min | Jul 2024 | [Observability](https://medium.com/mercadolibre-tech/building-a-large-scale-observability-ecosystem-1edf654b249e) (official) |
| Profiling | 4.0-4.5M profiles/day; 35k microservices; 16k+ IT staff | Mar 2026 | [Continuous profiling](https://medium.com/mercadolibre-tech/continuous-profiling-at-mercado-libre-turning-flamegraphs-into-fixes-2ee371c32bfd) (official) |
| Spanner | 214K QPS; 30K TPS; 400+ nodes | Nov 2025 | [Google Cloud](https://cloud.google.com/blog/topics/retail/inside-mercado-libres-multi-faceted-spanner-foundation-for-scale-and-ai) (vendor) |
| Databases | 17k databases; 56k data services; about 24k microservices | undated (likely 2022-23) | [AWS](https://aws.amazon.com/solutions/case-studies/innovators/mercado-libre/) (vendor) |

### Inferences
- Deploys per day rose from about 10k (2022) to about 19k (Apr 2024), about 23k (late 2024) and 30-36k (2025). Part of that growth comes from counting config changes and automatic redeploys as deploys.
- The "50M requests per second" figure from PlatformCon disagrees with the official 15M/s peak (Oct 2025) and with 900M RPM (about 15M/s). It is probably a transcription error.

### Gaps
- No official 2026 aggregate request-rate figure was found.

---

## 6. Engineering culture and lessons for a Go backend engineer

### Takeaway
The culture treats the platform as a product, works paved-road-first, grows adoption voluntarily, and enforces compliance through gates rather than mandates. Several rules follow directly from what Meli has published.

### Cited Findings
- Re:Invent 2024 lessons (third-party summary): treat the platform as an internal product and developers as customers; start small; provide choices and escape hatches; invest in docs, training and troubleshooting tools; define ownership and abstraction levels. — [zenn.dev](https://zenn.dev/kiiwami/articles/2449391f7fbaedd2?locale=en)
- Inner source and repo visibility tiers: public, internal, private and locked. GitHub teams mirror the org structure, with technology-specific teams for cross-repo work. — [Safe but flexible (2024)](https://medium.com/mercadolibre-tech/safe-but-flexible-the-devex-based-code-management-culture-at-mercado-libre-9b1dfde6f1b6)
- The Medium publication "Tecnología de Mercado Libre" (3.9K followers) last published on Mar 11, 2026 (as shown on the publication page when fetched). Articles appear in English, Spanish and Portuguese. — [medium.com/mercadolibre-tech](https://medium.com/mercadolibre-tech)
- Job listings from aggregators (third-party, indicative only) ask for Go in senior backend roles in Bogotá. Observability roles mention Datadog, New Relic, Kibana and Grafana and integrating with Fury. — [hw.glich.co: Senior Software Engineer Backend (Golang)](https://hw.glich.co/jobboard/jobs/senior-software-engineer-backend-golang-at-mercado-libre-uxdxvc); [Platform Observability](https://hw.glich.co/jobboard/jobs/software-engineer-platform-observability-it-at-mercado-libre-t1pwq3)

### Inferences: rules a Go backend engineer can take from Meli's practice
Each rule is based on the findings above.
1. **Split libraries into a portable core and a platform glue layer.** The core holds the HTTP server, resilient clients and circuit breaker with no environment assumptions. A thin platform package handles bootstrap and observability wiring (go-core / go-platform).
2. **Make libraries feel like the standard library**, version them strictly with semver, and spend the review time on the public API rather than the implementation.
3. **Treat backend services as I/O-bound pipelines**: receive, call out, aggregate, respond. Invest in client timeouts, retries with backoff, and circuit breakers before micro-optimizing CPU.
4. **Default to `database/sql` and plain SQL.** Adopt an ORM only when the team knows it deeply and accepts the cost in performance and "magic".
5. **Remove handler boilerplate** (decoding, error encoding, parameter parsing) with shared helpers, and include graceful shutdown on signals and liveness from day one.
6. **Standardize on OpenTelemetry** rather than building custom observability abstractions. Expose pprof on internal routes so profiles can be collected continuously and used for PGO.
7. **Separate build from deploy.** Build one immutable image and promote it across environments. Count config changes as deploys. Make every deploy step idempotent.
8. **Tier everything by criticality** (test, low, ..., company critical). Deploy strategy, Spot ratio, alert routing and SLAs all derive from that tier.
9. **Isolate the blast radius before scaling horizontally**: separate scopes for critical paths, and cells with deterministic routing IDs when a single database hits its vertical limit. A broadcast lookup is an acceptable bridge for legacy keys.
10. **Roll out quality gates as "indicator first, gate later"** (mutation score, coverage, lint), measure only the delta, and include a documented bypass.
11. **Win adoption through ergonomics, not mandates.** At Meli, both Fury and the Go toolkit spread voluntarily.

### Gaps
- No transcripts of Spanish or Portuguese Go meetup talks were found. Likely candidates are Golang Argentina, Gophers Latam and GopherCon Brasil, but I did not search them deeply within my tool budget.
- No InfoQ article on Meli and Go was found in this pass.
