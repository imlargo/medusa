# Data persistence, migrations, background jobs and messaging for enterprise Go backends

Research date: 2026-11-01. About 19 searches and fetches. Primary sources (pkg.go.dev, GitHub READMEs, threedots.tech, brandur.org, watermill.io) are preferred. Blog or aggregator claims are marked as such. Some items in the brief (Jet, squirrel/goqu, tern, NATS JetStream, amqp091-go, ristretto status, singleflight docs) were not fetched in this pass. They are listed under Gaps rather than asserted.

## DB access: database/sql vs pgx, sqlc, sqlx, ent, GORM, Bun, Jet, query builders; ORM vs SQL-first consensus

### Takeaway
For Postgres, the current SQL-first default is pgx v5 (native interface plus pgxpool) with sqlc generating type-safe code from handwritten SQL. Choose ent for schema-as-code or graph-heavy domains, and GORM when ecosystem and familiarity outweigh compile-time safety and performance. Bun sits between the two as a lightweight, SQL-oriented ORM and query builder.

### Cited Findings
- pgx v5 is actively released: v5.12.0 is shown as published Oct 31, 2026 on pkg.go.dev. The page itself warns it may not be the latest version in the module. — [pkg.go.dev pgx/v5](https://pkg.go.dev/github.com/jackc/pgx/v5)
- sqlc is not an ORM. It generates Go methods from SQL you write. Ent is schema-as-code with generated APIs, aimed at larger codebases. Bun is a lightweight, SQL-oriented query builder and ORM. — [glukhov.org comparison (blog)](https://glukhov.org/post/2025/09/comparing-go-orms-gorm-ent-bun-sqlc)
- Migration handling differs. GORM has AutoMigrate, which the source cautions against in production. Ent generates diff-based SQL (through Atlas). Bun ships a separate migrate package. sqlc relies on external tools. — [glukhov.org (blog)](https://glukhov.org/post/2025/09/comparing-go-orms-gorm-ent-bun-sqlc)
- Type safety: sqlc and Ent catch query and schema errors at compile time, while GORM's errors are mostly found at runtime. — [danilchenko.dev (blog)](https://www.danilchenko.dev/posts/sqlc-vs-gorm-vs-ent/)
- Performance: one comparison reports GORM about 2x slower than sqlc or raw database/sql on a 15,000-row fetch. That benchmark is from 2023. — [danilchenko.dev (blog)](https://www.danilchenko.dev/posts/sqlc-vs-gorm-vs-ent/)
- Another 2026 comparison ranks sqlc fastest and lowest-overhead, with GORM slower, but notes that database round-trip time dominates for most apps. — [tech-insider.org (low-authority blog)](https://tech-insider.org/gorm-vs-sqlc-vs-ent-2026/)
- Adoption, from a single less-established source as of October 2026: GORM about 40k GitHub stars, sqlc about 18k, Ent about 17k. GORM has the largest community. — [tech-insider.org](https://tech-insider.org/gorm-vs-sqlc-vs-ent-2026/)
- I found no authoritative, recent benchmark covering all of GORM, Ent, Bun and sqlc. The search summary itself flagged this. — [glukhov.org](https://glukhov.org/post/2025/09/comparing-go-orms-gorm-ent-bun-sqlc); [danilchenko.dev](https://www.danilchenko.dev/posts/sqlc-vs-gorm-vs-ent/)

### Inferences
- The cited sources support this split. sqlc on pgx gives compile-time safety and near-raw performance at the cost of dynamic queries. Dynamic filters need a query builder alongside it (squirrel, goqu or Jet) or sqlc's nullable-parameter tricks.
- GORM's runtime errors and AutoMigrate are its main enterprise risks. Teams that pick GORM should still run versioned migrations with a dedicated tool.
- The benchmarks are old or low-authority. Performance arguments should be framed as "ORM overhead is usually dwarfed by DB round trips," with type safety and reviewability as the stronger differentiators.

### Gaps
- Not verified this pass: the current status of sqlx (jmoiron/sqlx, historically slow-moving), go-jet/jet, Masterminds/squirrel (historically labelled feature-complete, not actively developed) and doug-martin/goqu. Their GitHub pages should be checked before citing.
- I found no primary-source survey (for example the Go Developer Survey) quantifying ORM vs SQL-first preference in 2026.

## Connection pool tuning, prepared statements with PgBouncer, context cancellation

### Takeaway
database/sql defaults are unsafe for production: unlimited open connections, only 2 idle and no lifetime limit. Set the limits explicitly, sized against Postgres `max_connections` divided by the number of instances. pgxpool has saner defaults (max conns is the greater of 4 or NumCPU, 1h lifetime, 30m idle). For PgBouncer in transaction mode, use PgBouncer 1.21+ with `max_prepared_statements > 0` and keep pgx's default statement cache. Otherwise switch to `QueryExecModeExec`, which pgx prefers over `SimpleProtocol`.

### Cited Findings
- database/sql: max open connections defaults to 0 (unlimited). Microsoft frames tuning values as a starting point, not a universal default. — [Microsoft go-mssqldb docs](https://learn.microsoft.com/en-us/sql/connect/golang/connection-pooling?view=sql-server-ver17)
- sql.DB retains at most 2 idle connections by default, and connections live forever unless SetConnMaxLifetime is set. — [Alex Edwards, Configuring sql.DB](https://www.alexedwards.net/blog/configuring-sqldb)
- A typical starting point is SetMaxOpenConns(25), SetMaxIdleConns at or below MaxOpen, and SetConnMaxLifetime of about 5m, kept below the firewall or load balancer idle timeout. Another source sizes per instance as `max_connections` / instances (for example 100 / 4 → 20). — [Alex Edwards](https://www.alexedwards.net/blog/configuring-sqldb); [oneuptime blog](https://oneuptime.com/blog/post/2026-01-07-go-postgresql-connection-pooling/view)
- pgxpool defaults, quoted from the docs:
  - `pool_max_conns`: "default is the greater of 4 or runtime.NumCPU()"
  - `pool_min_conns`: default 0
  - `pool_max_conn_lifetime`: default 1 hour
  - `pool_max_conn_idle_time`: default 30 minutes
  - `pool_health_check_period`: default 1 minute
  - `pool_max_conn_lifetime_jitter`: default 0

  — [pkg.go.dev pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool)
- pgx v5 QueryExecMode. The default `CacheStatement` uses named prepared statements, which PgBouncer 1.21.0+ supports in transaction and statement pooling when `max_prepared_statements > 0`. For older PgBouncer, or with prepared statements disabled, set `DefaultQueryExecMode` to `QueryExecModeExec`. `SimpleProtocol` is compatible, but the docs say to prefer `Exec`. "Do not use QueryExecModeDescribeExec with transaction pooling", because PgBouncer may assign a different server connection between describe and execute. SQL-level `PREPARE` is unsupported under transaction pooling. — [pkg.go.dev pgx/v5](https://pkg.go.dev/github.com/jackc/pgx/v5)
- Real-world example: GitLab's LabKit v2 Postgres package defaults to `QueryExecModeSimpleProtocol` for PgBouncer transaction mode. The motivation is `ERROR: prepared statement "..." already exists` failures in production. GitLab Container Registry made the same change. — [GitLab LabKit MR 363](https://gitlab.com/gitlab-org/labkit/-/merge_requests/363); [LabKit commit](https://gitlab.com/gitlab-org/labkit/commit/3f701cc73476c2a7f3eb762e53e3beb0efa9e758); [Container Registry issue 268](https://gitlab.com/gitlab-org/container-registry/-/issues/268)

### Inferences
- GitLab chose SimpleProtocol and the pgx docs prefer Exec. Both avoid server-side prepared statements, but Exec keeps the extended protocol with binary parameters. The strongest recommendation is Exec when the pooler cannot handle prepared statements, or the default cache with PgBouncer ≥1.21.
- Context cancellation: pgx and database/sql both take `ctx` on every query method, so per-request deadlines propagate into queries. I did not fetch a primary source detailing pgx's cancel-request behavior, so this stays an inference.

### Gaps
- No primary source fetched on how pgx handles context cancellation (whether it sends a PG cancel request and closes the connection). Check the pgconn `BuildContextWatcherHandler` / `CancelRequestContextWatcherHandler` docs.
- The pgxpool `MaxConnLifetimeJitter` rationale (avoiding a thundering herd of reconnects) was not sourced.

## Transaction patterns in layered Go apps: unit of work, WithTx closures, repository debate, tx in context

### Takeaway
Three Dots Labs, the most-cited Go source on this, recommends the repository `Update(ctx, id, updateFn)` closure pattern as the default. Use a transaction provider (unit of work) sparingly, for cross-repository atomicity. Avoid managing transactions in the application layer, and avoid hiding a tx in `context.Context`.

### Cited Findings
- The UpdateFn closure is the Three Dots Labs "go-to solution". The repository opens the transaction, loads the aggregate, calls `updateFn` with the business logic, and saves. "The repository has no logic at all." This keeps domain logic unit-testable without a DB. — [threedots.tech: Database Transactions in Go with Layered Architecture](https://threedots.tech/post/database-transactions-in-go/)
- A transaction provider (unit-of-work style) suits edge cases such as an audit log next to a user update. Repositories receive the tx in their constructor. The article warns "be careful not to over-use it": it "can quickly get out of control" in fast-moving teams and is risky with `FOR UPDATE`. — [threedots.tech](https://threedots.tech/post/database-transactions-in-go/)
- On transactions in the logic layer, the section is titled "avoid if you can": "Avoid mixing transactions with your application logic." — [threedots.tech](https://threedots.tech/post/database-transactions-in-go/)
- On tx in context, an abstract `Transaction` object or passing the tx via `Context` "only masks the root issue". — [threedots.tech](https://threedots.tech/post/database-transactions-in-go/)
- On repository design: one repository per aggregate, not per table. Group data that must be consistent together (for example user points and discounts) into one aggregate. — [threedots.tech](https://threedots.tech/post/database-transactions-in-go/)
- Counterpoint: Qonto lets the domain decide atomicity, with the transaction behind a Go interface using database/sql. — [Qonto: Transactions in Go Hexagonal Architecture](https://medium.com/qonto-way/transactions-in-go-hexagonal-architecture-f12c7a817a61)
- Another practitioner notes that starting a tx in business code and passing it down is simple but breaks layering. Some teams pass it via context despite the trade-off. — [William Ong](https://williamong.netlify.app/posts/seperate-application-and-database-code-in-hexagonal-architecture)
- In r/golang discussion, starting transactions in the service layer is common and considered acceptable if maintainable. This is community opinion, not consensus. — [r/golang thread (mirror)](https://axebps-redlib.hf.space/r/golang/comments/1j403if/the_repository_pattern_in_go/mg7lltm/?context=3)

### Inferences
- A pragmatic enterprise default: a `WithTx(ctx, func(q *Queries) error)` helper at the store layer (sqlc's `Queries.WithTx(tx)` fits this), aggregate-scoped repositories with UpdateFn, and an explicit transaction provider only where several repositories must commit together. This avoids context-smuggled transactions.
- River's transactional enqueue (next section) adds a reason why the tx must be reachable at the store layer: the job insert has to share the business write's tx.

### Gaps
- No quantitative data on how widespread each pattern is in Go codebases.

## Migrations: golang-migrate, goose, Atlas, tern; separate step vs startup; expand/contract

### Takeaway
golang-migrate (about 19k stars, v4 line) and goose are the mainstream versioned-SQL tools. goose adds Go-function migrations. Atlas adds declarative diffing, a lock, an integrity sum file and linting, and it is what Ent uses. Zero-downtime is a property of the migration discipline (expand/contract over several deploys), not of the tool.

### Cited Findings
- golang-migrate: 19.0k stars and 1.6k forks. The v4 line is supported and v3 is marked "DO NOT USE". It has 317 open issues and 184 open PRs, and its README lists supported Go versions 1.25 and 1.26. It supports a graceful stop via `GracefulStop chan bool` to "help prevent database corruptions". — [GitHub golang-migrate/migrate](https://github.com/golang-migrate/migrate)
- golang-migrate leaves transactions to the user and records failed runs as a "dirty" version that needs a manual `force` to fix. goose supports Go-code migrations, useful for logic-heavy backfills, and wraps migrations in transactions by default. Atlas supports declarative schemas with computed diffs, takes a lock so only one migration runs at a time, and keeps an integrity (atlas.sum) file that detects edited applied files. — [schemadrawer.com comparison (blog)](https://schemadrawer.com/blog/go-migration-tools-compared/); [oneuptime (blog)](https://oneuptime.com/blog/post/2026-02-01-go-database-migrations/view)
- Atlas's own comparison of Go migration tools is vendor-authored and should be weighed accordingly. — [atlasgo.io blog (vendor)](https://atlasgo.io/blog/2022/12/01/picking-database-migration-tool)
- Zero downtime requires multiple deployments and migrations (expand/contract) and does not depend on the tool. — [schemadrawer.com (blog)](https://schemadrawer.com/blog/go-migration-tools-compared/)

### Inferences
- In multi-replica deployments, running migrations as a separate pipeline step (a Kubernetes Job or init step) rather than at app startup avoids racing replicas, and avoids a failed or dirty migration crash-looping every pod. A tool with locking (Atlas, or goose and golang-migrate with their DB-level locks) reduces but does not remove that risk. This is a standard practice inference. I did not fetch a primary source for it.
- Expand/contract sequence:
  1. Add the nullable column or new table.
  2. Deploy code that dual-writes.
  3. Backfill in batches.
  4. Switch reads.
  5. Drop the old column in a later release.

  For Postgres, also create indexes with `CREATE INDEX CONCURRENTLY` (which cannot run inside a transaction) and set `lock_timeout` on DDL. These are well-known practices but were not sourced this pass.

### Gaps
- tern (jackc/tern) status not fetched.
- goose's current version and maintainer status (pressly/goose) not fetched.
- I found no primary source comparing the locking implementations of golang-migrate and goose.

## Transactional outbox and inbox in Go, idempotency, exactly-once illusions

### Takeaway
Write the domain change and the outbox row (or a River job) in the same DB transaction. Relay rows to the broker with at-least-once semantics, and make consumers idempotent with an inbox or processed-messages table keyed by message ID. In Go, Watermill provides this through the SQL Pub/Sub and its Forwarder component. Its "exactly once" wording refers only to the SQL subscriber's offset tracking, not end-to-end delivery.

### Cited Findings
- Brandur's argument: enqueueing to a fast external queue mid-transaction lets the job run before commit, or run for data that is later rolled back. Enqueueing after commit risks losing the job if the process crashes between commit and enqueue. — [brandur.org: Transactionally Staged Job Drains](https://brandur.org/job-drain)
- The fix is a `staged_jobs` table written in the same transaction, which is invisible until commit and discarded on rollback. An enqueuer drains it to the queue and deletes rows only after transmission, so "At least once delivery semantics are guaranteed." — [brandur.org/job-drain](https://brandur.org/job-drain)
- Watermill Forwarder: a database-backed subscriber republishes messages to a broker publisher. This is the outbox relay. — [watermill.io Forwarder](https://watermill.io/advanced/forwarder/); [watermill.io SQL Pub/Sub](https://watermill.io/pubsubs/sql/)
- The Watermill SQL publisher accepts a `*sql.Tx`, so the event is written in the caller's business transaction. The Postgres and MySQL offsets adapters are "designed to support multiple subscribers with exactly once delivery and guaranteed order". That claim is about the SQL subscriber only. — [watermill.io SQL](https://watermill.io/pubsubs/sql/); [pkg.go.dev mirror of watermill sql docs](https://pkg.go.dev/github.com/wfusion/gofusion/common/infra/watermill/pubsub/sql)
- Watermill 1.3 fixed "a low chance of losing messages in watermill-sql due to how transactions work" and recommended new tables for outbox use. I saw this in search output and did not verify it on the release page. — [Watermill GitHub](https://github.com/ThreeDotsLabs/watermill)
- Practitioner advice: treat the outbox as the default for anything money-adjacent, and design for at-least-once delivery. Make consumers idempotent by storing processed message UUIDs in a table with a unique index (an inbox). — [DEV: Watermill pitfalls (blog)](https://dev.to/dvorlandi/watermill-pitfalls-i-wish-i-knew-before-shipping-eda-in-go-15lf)
- Three Dots Labs has a dedicated outbox explainer. — [Three Dots Labs Academy: Outbox](https://academy.threedots.tech/knowledge/outbox/)

### Inferences
- "Exactly once" in practice means at-least-once delivery plus idempotent effects. The inbox insert and the side effect should share one DB transaction, which turns duplicates into unique-constraint no-ops.
- With a Postgres-only stack, River's transactional insert removes the need for a separate outbox and relay for internal jobs. An outbox and relay remain necessary for publishing to external brokers (Kafka, NATS).

### Gaps
- I did not verify the current Watermill major version or release date.
- No source found on Debezium/CDC-based outbox usage from Go specifically.

## Background jobs and queues: River, Asynq, Temporal, NATS/JetStream, Kafka clients, RabbitMQ, Watermill

### Takeaway
- **River (Postgres):** the default when Postgres is already the system of record. It enqueues transactionally and is actively developed.
- **Asynq (Redis):** stable but pre-1.0, with slowing development.
- **Temporal:** use for durable multi-step workflows and sagas, not plain jobs.
- **Kafka:** franz-go is the strongest pure-Go client. confluent-kafka-go is actively maintained but needs cgo. segmentio/kafka-go is popular but shows signs of lagging behind Kafka (tested only up to Kafka 2.7.1).
- **Watermill:** an abstraction over these brokers that also provides the outbox.

### Cited Findings
- River is "a fast, transactional job queue backed by Postgres or SQLite". "Jobs are guaranteed to be enqueued if their transaction commits", and they are discarded on rollback. It offers retries, scheduling, unique jobs, periodic jobs and a web UI. It has about 5.8k stars. Its Go driver is `riverpgxv5`, and clients exist for Go, Java, JS/TS, Ruby and Rust. — [GitHub riverqueue/river](https://github.com/riverqueue/river)
- River features listed by aggregators include COPY-based batch insert, snoozing and cancellation, step-based resumable jobs, pluggable retry policies, subscriptions and River UI. Versions in 2026 reached at least v0.30 (January 2026) and later v0.49.0 (the directory lists it as "vv0.49.0"). I could not confirm exact dates. — [ecosyste.ms](https://awesome.ecosyste.ms/projects/github.com%2Friverqueue%2Friver); [openapps.pro](https://openapps.pro/packages/river)
- River's open core focuses on background jobs, and workflows are gated behind the paid River Pro (per a Hacker News commenter). — [Hacker News thread](https://news.ycombinator.com/item?id=48628338)
- Asynq README: "relatively stable and undergoing moderate development". It is still v0.x. v0.26.0 was published Feb 3, 2026. — [GitHub hibiken/asynq](https://github.com/hibiken/asynq); [pkg.go.dev asynq](https://pkg.go.dev/github.com/hibiken/asynq)
- A Hacker News commenter says Asynq development is "not as active anymore", though a new contributor has joined. — [Hacker News](https://news.ycombinator.com/item?id=48628338)
- Migration example: a Go project replaced Asynq with River. It notes that Asynq and River do not coordinate, so jobs can run twice during overlapping deploys. — [go-torrent-trader PR 290](https://github.com/williamokano/go-torrent-trader/pull/290)
- Brandur notes that in-database queues where workers lock jobs directly historically scaled poorly under heavy lock contention. That critique was written about older Ruby queues (delayed_job, que, queue_classic). — [brandur.org/job-drain](https://brandur.org/job-drain)
- Temporal is a distributed orchestration engine for long-running business logic. The Go SDK is how you write workflows and activities. — [temporalio Go SDK (GitHub mirror)](https://github.com/temporalio/temporal-go-client)
- franz-go is a pure-Go client covering Kafka features from 0.8.0 onward, with latest v1.21.0 on pkg.go.dev. Its speed claims come from its own README. — [pkg.go.dev franz-go](https://pkg.go.dev/github.com/twmb/franz-go)
- confluent-kafka-go is a cgo wrapper over librdkafka, maintained by Confluent. One post counted 55 commits and 29 merged PRs in the 90 days to late September 2026. Its default partitioner is CRC32, not the Murmur2 used by the Java client and franz-go, which matters for mixed-language producers. — [pkg.go.dev confluent-kafka-go](https://pkg.go.dev/github.com/confluentinc/confluent-kafka-go); [DEV: confluent-kafka-go in 2026 (blog)](https://dev.to/getkafma/confluent-kafka-go-in-2026-what-still-works-and-what-to-watch-g3j)
- segmentio/kafka-go: 8.6k stars, 183 open issues and 92 open PRs. It is "tested with Kafka versions 0.10.1.0 to 2.7.1". `NewWriter` and `WriterConfig` are deprecated. Consumer-group readers cannot `SetOffset`, and `Offset` and `Lag` return -1. — [GitHub segmentio/kafka-go](https://github.com/segmentio/kafka-go)
- One comparison names franz-go the recommended pure-Go option. Another calls kafka-go the de facto pure-Go client. The sources disagree. — [kafma.app (vendor blog)](https://kafma.app/blog/kafka-golang-clients); [pistack.xyz (blog)](https://www.pistack.xyz/posts/2026-08-23-kafka-client-libraries-kafka-go-kafka-python-confluent-comparison/); [AxonOps Go driver docs](https://axonops.com/docs/data-platforms/kafka/application-development/drivers/go/)
- Watermill is described as "Building event-driven applications the easy way in Go", with pluggable Pub/Subs including SQL. — [GitHub ThreeDotsLabs/watermill](https://github.com/ThreeDotsLabs/watermill)

### Inferences
- Decision guide drawn from the findings:
  - **River:** jobs tied to DB writes, with Postgres already present.
  - **Asynq:** Redis is already central and you accept pre-1.0 and slower development.
  - **Temporal:** long-running, multi-step, compensating workflows (sagas, human-in-the-loop), with the extra ops cost of a Temporal cluster or Temporal Cloud.
  - **Kafka with franz-go:** high-throughput event streaming or retention for many consumers.
  - **NATS JetStream:** lightweight streaming or request-reply.
  - **RabbitMQ with amqp091-go:** classic routing and work queues.
  - **Watermill:** when you want broker portability and outbox and middleware out of the box.
- River's growth (v0.3x to v0.4x within 2026) and transactional semantics make it the strongest new default for Go plus Postgres SaaS. Prefer franz-go over segmentio/kafka-go for new Kafka work, given kafka-go's tested-version ceiling and open backlog.

### Gaps
- Not fetched: NATS JetStream Go client (nats.go `jetstream` package) status. The legacy JetStream API was superseded by the new `jetstream` package, but this is not verified here.
- Not fetched: rabbitmq/amqp091-go, the maintained fork of the archived streadway/amqp (not verified this pass).
- Not fetched: the Temporal Go SDK latest version.
- Not fetched: River's exact current version and River Pro feature list (riverqueue.com).
- IBM/sarama status not covered.

## Caching: go-redis, rueidis, ristretto, otter, singleflight

### Takeaway
Both main Redis clients now sit under the `redis` GitHub organization. rueidis offers auto-pipelining and server-assisted client-side caching, with large self-reported throughput gains over go-redis v9. For in-process caching, otter v2 (W-TinyLFU, loaders that dedupe concurrent misses, async refresh) is the modern choice. singleflight is the standard-library-adjacent tool for stampede protection.

### Cited Findings
- rueidis lives under github.com/redis/rueidis (about 3k stars). It auto-pipelines concurrent commands by default (`DisableAutoPipelining` switches to a pool model) and supports server-assisted client-side caching via `DoCache` / `DoMultiCache` with invalidation notifications. It also has cache-aside helpers, distributed locks, rate limiting, OpenTelemetry, Cluster and Sentinel support, and a go-redis-style adapter. — [GitHub redis/rueidis](https://github.com/redis/rueidis)
- rueidis claims higher throughput than go-redis v9 at parallelism 1, 8 and 64, and about 14x at parallelism 64 on an M1 Pro. These are self-reported benchmarks. — [GitHub redis/rueidis](https://github.com/redis/rueidis)
- otter v2 is the latest stable major (`github.com/maypok86/otter/v2`) and requires Go 1.24+. It uses adaptive W-TinyLFU and supports size and time eviction, automatic loading where "concurrent Gets would deduplicate loader calls" (stampede protection), async refresh that returns the stale value, stats and persistence. Its design principles come from Java's Caffeine. — [GitHub maypok86/otter](https://github.com/maypok86/otter)

### Inferences
- With otter v2, built-in loader dedup replaces hand-rolled `singleflight` for the local tier. `golang.org/x/sync/singleflight` is still useful in front of Redis or DB loads for the remote tier.
- go-redis remains the most familiar API. rueidis is worth it at high concurrency or when client-side caching cuts Redis round trips.

### Gaps
- Not fetched: go-redis (redis/go-redis) v9 latest release, dgraph-io/ristretto maintenance status (Dgraph ownership changed hands, with v2 released in 2024, unverified here), and the `golang.org/x/sync/singleflight` docs. No independent benchmark comparing otter and ristretto hit ratios was retrieved.

## Multi-tenancy in Go SaaS: Postgres RLS, schema per tenant

### Takeaway
The prevailing pattern is a shared schema with a `tenant_id` column and Postgres RLS policies. A per-transaction GUC is set via `set_config('app.tenant_id', $1, true)` inside a `RunInTx` helper that reads the tenant from context. `FORCE ROW LEVEL SECURITY` and `WITH CHECK` are mandatory details, and session-level `SET` is unsafe behind transaction poolers. Schema per tenant trades stronger isolation for migration and connection overhead.

### Cited Findings
- Use `set_config(name, value, true)` (equivalent to `SET LOCAL`), which resets at commit. This is what makes RLS compatible with transaction pooling. A session-level `SET` persists on the server connection and leaks into another tenant's transaction. — [DEV: Postgres multi-tenancy: RLS, tenant_id filters, or schema per tenant](https://dev.to/libme/postgres-multi-tenancy-row-level-security-tenantid-filters-or-a-schema-per-tenant-11j9)
- Go and pgx pattern: a `RunInTx` helper takes the tenant ID from context, fails if it is missing, and runs `set_config('app.current_tenant', $1, true)` inside the tx. Table owners (often the app user) bypass RLS unless `FORCE ROW LEVEL SECURITY` is set. — [DEV: PostgreSQL RLS in Go (blog)](https://dev.to/__8fa66572/postgresql-rls-in-go-architecting-secure-multi-tenancy-4ifm)
- Policies compare `tenant_id` to `current_setting(...)`, with `WITH CHECK` to prevent cross-tenant inserts. — [DEV: Multi-tenant SaaS with RLS](https://dev.to/macrogenltd/multi-tenant-saas-with-postgres-row-level-security-a-practical-guide-3n3i); [oneuptime: RLS in PostgreSQL](https://oneuptime.com/blog/post/2026-01-25-row-level-security-postgresql/view)
- ClickHouse's engineering guide covers the architecture trade-offs between shared-schema, schema-per-tenant and database-per-tenant designs on Postgres. — [ClickHouse: How to architect multi-tenant SaaS on Postgres](https://clickhouse.com/resources/engineering/multi-tenant-saas-postgres-architecture)

### Inferences
- RLS fits the WithTx and UpdateFn transaction patterns above. Every DB access goes through a tx helper that sets the tenant GUC, which makes forgetting the tenant filter a policy failure (zero rows) rather than a data leak.
- The app should connect as a non-owner role without `BYPASSRLS`, with migrations run by a separate owner role. This follows from the FORCE RLS finding.
- Schema per tenant multiplies migration runs (N schemas) and pgx statement caches (search_path changes). It suits a small number of large tenants with compliance isolation needs.

### Gaps
- No official PostgreSQL or pgx documentation was fetched for this section. The sources are blogs. The PostgreSQL docs on Row Security Policies (postgresql.org/docs/current/ddl-rowsecurity.html) should be cited in the final report.
- No benchmark on RLS overhead or planner impact was retrieved.
