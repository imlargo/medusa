# AGENTS.md

How to work in this repository. Read it before changing anything. Product context and status live in [context.md](context.md), what comes next in [docs/roadmap.md](docs/roadmap.md), how the system behaves at runtime in [docs/architecture.md](docs/architecture.md), and the reasoning behind the layout in [docs/adr/](docs/adr/) (0001: Medusa apart from the app, 0002: batteries as optional modules, 0003: application errors and business rules, 0004: the job queue on Postgres, 0005: the caller as a Principal in the context, 0006: events as an outbox on the job queue, 0007: each feature owns its tables, 0008: the API contract on Huma).

## The mental model

There are two things in this repo, Medusa and an app built on it, and the whole layout follows from keeping them apart. Medusa itself has two layers: a core and optional batteries.

| | Medusa core | Medusa batteries | The app |
|---|---|---|---|
| What | A toolkit for HTTP APIs on Gin | Ready-made pieces most APIs need, opt-in | One concrete API built on Medusa |
| Where | Repository root (`/`) | `api/`, `auth/`, `env/`, `postgres/`, `storage/` | `app/` |
| Module | `github.com/imlargo/medusa` | `github.com/imlargo/medusa/<battery>`, one each | `github.com/imlargo/medusa/app` |
| Knows about | HTTP, errors, lifecycle, health, metrics | One integration each (Huma, golang-jwt, godotenv, pgx + goose, the AWS SDK) | Users, its schema, Redis, config, routes |
| Imports | Never a battery or the app | The core at most, never the app | Medusa, through its public packages only |

The direction is one way: **app -> batteries -> core**. If a change to Medusa needs an app type, the change belongs in the app.

`go.work` at the root ties every module to this checkout. `app/go.mod` also `replace`s the Medusa modules with their local paths, so the example always builds against this checkout. A battery requires the released core (`auth` requires `github.com/imlargo/medusa v0.1.0`), so CI checks it the way a consumer gets it: a battery that needs an unreleased core change waits for the core's release.

## Where does my code go?

| I want to... | Put it in |
|---|---|
| Add an endpoint, a business rule, a table, a query | The app, in the feature's package (see the recipes below) |
| Change config or environment variables | `app/internal/config` (+ `app/.env.example`); how variables are read is Medusa's `env` |
| Change how tokens are signed, parsed or required | Medusa: `auth/` |
| Change registration, login or token expiry | `app/internal/account` |
| Change how routes or middleware are wired | `app/cmd/api/bootstrap/routes.go` |
| Change how routes are bound, validated or documented | Medusa: `api/` |
| Add a dependency to the running app (a client, a pool) | `app/cmd/api/bootstrap/container.go` |
| Change the error model or how errors are rendered | Medusa: `apperr/`, `httperr/`, `response.go` |
| Change the pool, migrations runner or test databases | Medusa: `postgres/` |
| Add a generic middleware or helper any API could use | Medusa, if it passes the checklist below |

When in doubt, put it in the app. Moving code into Medusa later is easy; taking an API out of a library is not.

### Medusa map

Core:

| Package | Holds |
|---|---|
| `medusa` (root) | `Context`, `Handle`/`HandleCreate`/`HandleUpdate`/`HandleGet`/`HandleDelete`, response writers, `RequestIDKey`, `RequestIDFrom(ctx)`, `LogHandler`, the request errors (`ErrValidation`, `ErrMissingCredentials`...) |
| `apperr` | `Error`, `Kind`, declaring constructors (`NotFound`, `Conflict`, `FailedPrecondition`...), `From`, `Declared` (every declared error, for the catalog) |
| `httperr` | `Status(kind)`, `Problem` (RFC 9457), `NewProblem`, `Write` for plain net/http |
| `middleware` | `RequestID`, `SecureHeaders`, `ReportErrors`, `Recover`, `Timeout`, `CORS`, `BodyLimit`, `Metrics`, `RateLimit`, `APIKey`, `BearerAPIKey` |
| `health` | `Service.Register(name, fn)`, `Service.Drain(ctx, delay)`, `Handler.Live`, `Handler.Ready` |
| `httpserver` | `New(handler, logger, opts...)`: serves any `http.Handler` with graceful shutdown |
| `job` | `Every(name, interval, fn, opts...)`: periodic background work, run and stopped by `lifecycle` |
| `lifecycle` | `New(opts...).Run(ctx)`: runs servers and hooks, stops on SIGINT/SIGTERM |
| `metrics` | `NewHTTP(registry)`: Prometheus request metrics; `NewJobs(registry, stats)`: job wait, duration, outcomes and queue depth |
| `webhook` | Standard Webhooks: `NewSigner` (`Headers`, `NewRequest`), `NewVerifier` (`Verify`, secrets for rotation, tolerance); `HMAC` for a provider's own header scheme |
| `httpclient` | `New(opts...)`, `Do`, `JSON`: outbound calls with a timeout, a body cap, safe retries, the request ID sent along; `StatusError`, `ErrUnavailable` |

Batteries:

| Module | Holds |
|---|---|
| `postgres` | `Open(ctx, url, logger, opts...)`, `Migrate(ctx, url, migrations, logger)`, `Conn`, `IsNoRows`, `IsUniqueViolation` |
| `postgres/pgtest` | `New(t, migrations)`: a fresh database per test, cloned from a template migrated once per set of migrations |
| `postgres/events` | A fact to several consumers, on the queue: `PublishTx`, `NewSubscribers`, `Subscribe` (one job per subscriber, retried on its own), `Attach(worker)` |
| `postgres/queue` | Jobs on Postgres: `Install`, `New(conn)`, `Enqueue`/`EnqueueTx`, `Worker(logger, queues)`, `Register`, `Schedule` (cron), `Periodic`, `Stats`, `Failed`, `Retry` |
| `auth` | `Principal` in the context: `Middleware(authenticators...)`, `Enrich`, `Require(scopes...)`, `PrincipalFrom(ctx)`, `SubjectFrom[S](ctx)`; authenticators `NewJWT[S]` (`Sign`, `Parse`, `WithScopes`, `WithTenant`) and `APIKeys(lookup)`; `NewRefresher(store, ttl)` for opaque refresh tokens that rotate (`Issue`, `Rotate`, `Revoke`) over a `RefreshStore` the app implements |
| `storage` | S3/R2 object storage: `NewFileStorage`, upload (sized, stream, presigned PUT), download, `Head`, `Copy`, `List`, bulk delete, key validation |
| `api` | The API contract on Huma: `New`, `Group` (`Secured`, `Tags`), `Get`/`Post`/`Put`/`Patch`/`Delete` for `func(ctx, *In) (*Resource, error)`, `Errors`, `Status`, `Scopes`, `Empty`, `Optional[T]`, `Page`/`List[T]`/`NewList`, `Document`, `Request` |
| `env` | `LoadDotEnv()`, `New()`: typed getters (`Required`, `Int`, `Duration`, `List`...) that collect every problem into `Err()` |

### App map

```
app/
  cmd/api/main.go               entry point, calls bootstrap
  cmd/api/bootstrap/            wiring: container.go (dependencies), routes.go (every route), app.go
  cmd/migrate/                  applies migrations and installs the job queue, run as its own deploy step
  cmd/worker/                   runs queued jobs; wired in bootstrap/worker.go
  internal/config/              loads and validates env vars
  internal/database/migrations/ the one migration history, assembled from every feature's (migrations.FS)
  internal/user/                feature, owns users: User, NormalizeEmail, errors, Repository
  internal/account/             feature: register, login, refresh, logout, me (Service, Handler, errors, jobs and events)
  internal/session/             feature, owns refresh_tokens: Repository, the auth.RefreshStore of the app
  internal/events/              feature: SSE stream and publish (Handler, errors)
  internal/store/               what features share about the database: WithTx, Enqueue, Publish. Imports no feature
  sqlc.yaml                     one block per feature that owns tables
  api/errors.md                 GENERATED by TestErrorCatalog: every error code the API answers with
```

Code is grouped by feature, not by layer. Inside a feature a request flows handler -> service -> repository -> sqlc -> Postgres, and the feature's errors flow back up unchanged until `medusa.WriteError` renders them.

A feature package holds, as files:

| File | Holds |
|---|---|
| `<feature>.go` | The domain types and the rules on them (`user.NormalizeEmail`) |
| `errors.go` | Every error the feature can return, declared with `apperr` |
| `repository.go` | Storage: sqlc rows to domain types, driver errors to the feature's errors |
| `migrations/*.sql`, `migrations.go` | The tables the feature owns, and the `Migrations` embed that puts them in the history |
| `queries/*.sql` | Its sqlc queries |
| `internal/db/` | GENERATED by `make sqlc` from the two above. Only this feature can import it |
| `service.go` | Use cases: rules that need storage or other features |
| `handler.go` | Input types (where each value comes from and what is valid, as tags) and handler methods |

Only the files the feature needs exist: `user` has no service or handler, `events` no repository.

**A table has one owner, the feature whose migrations create it, and only the owner writes it** (ADR 0007). Another feature that needs the data changed calls the owner's Go API or reacts to its events. One that needs to read it, or to reference it with a foreign key, lists the owner's migrations in its own `schema` in `sqlc.yaml`. CI fails on a query that writes a table of another feature.

## Recipes

### Add an endpoint

1. In the feature's package (a new `app/internal/<feature>` if none fits), in `handler.go`, the input type. Its tags say where each value comes from and what is valid (ADR 0008):
   ```go
   type UpdateInput struct {
       ID   int64 `path:"id" minimum:"1"`
       Body struct {
           Name string `json:"name" minLength:"1" maxLength:"80"`
       }
   }
   ```
   `path`, `query` and `header` go on fields; the JSON body is the field `Body`. A body field the type does not have is refused.
2. The logic as a method on the feature's `Service`. Business rules return the feature's declared errors (next recipe); any other error becomes a 500.
3. A handler method `func(ctx context.Context, in *UpdateInput) (*Thing, error)` that returns the resource itself. For no body, return `*api.Empty`. Read the signed-in user with `auth.SubjectFrom[int64](ctx)`.
4. A new service or handler is constructed in `buildServices` / `buildHandlers` in `container.go`.
5. The route in `routes.go`, with the errors it can answer: `api.Patch(g.protected, "/things/{id}", h.Update, api.Errors(thing.ErrNotFound))`. Use `g.protected` when it needs a logged-in user. `api.Status(http.StatusCreated)` for a route that creates, `api.Scopes(auth.Require, "things:write")` for a scope.

The route is in the OpenAPI document as soon as it is registered: there is nothing to generate. Other cases:

| Need | Use |
|---|---|
| A partial update, where "not sent" differs from null | `api.Optional[T]` fields, tagged `json:"x,omitzero"` |
| A list | Embed `api.Page` in the input, return `*api.List[T]` built with `api.NewList` |
| A redirect or a response header | An output struct with `Status int` or `header:"..."` fields |
| The exact bytes of the body (a signed webhook) | `RawBody []byte` in the input, and `api.Request(ctx)` for the headers |
| A stream or a websocket | A Gin handler on `g.public.Router()`, listed with `api.Document` |
| A route left out of the document | `api.Hidden()` |

### Add a business rule and its error

1. The rule goes in one place: a function or method on the domain type in `<feature>.go` when it needs no storage, a method on the `Service` when it does. Never in a handler or a repository, except the storage invariants a repository enforces (`NormalizeEmail`).
2. Its error in the feature's `errors.go`, declared once:
   `var ErrOrderShipped = apperr.FailedPrecondition("order.already_shipped", "the order has already shipped")`.
   - The code is `<feature>.<rule>`, lowercase with underscores. It is part of the API contract: never rename one that shipped.
   - The message is safe to show to anyone. Never build it from an error or from user input; put specifics in `WithFields` or `WithMeta`.
   - Pick the kind by what the caller can do, using the table in [ADR 0003](docs/adr/0003-application-errors-and-business-rules.md). Ask "can the same request succeed later?" and "is the input itself wrong?".
3. Return it as is, or with the cause for the log: `return ErrOrderShipped.WithCause(err)`. Callers match it with `errors.Is`.
4. A test that asserts the rule returns exactly that error.
5. Regenerate the catalog of codes, `app/api/errors.md`: `go test ./cmd/api/bootstrap -run TestErrorCatalog -update` from `app/`. CI fails when it is out of date or a code is declared twice.

### Add a table or a query

Everything goes inside the feature that owns the table, `app/internal/<feature>/`.

1. A migration in `migrations/`, named by timestamp so it sorts after every other feature's: `20261128153000_create_orders.sql`, starting with `-- +goose Up`. Never edit a migration that has been applied. If CI's `migration order` job fails, another migration merged first with a later version: rename yours with a current timestamp. `migrate` refuses a migration older than one already applied.
2. The queries in `queries/x.sql`, each with `-- name: Name :one|:many|:exec`. They may read a table of another feature; they write only this feature's. All SQL goes here: a write passed as a string to `Exec` in Go code fails CI.
3. For the feature's first table:
   - `migrations.go` with `//go:embed migrations/*.sql` and `var Migrations embed.FS` (copy `user/migrations.go`);
   - the feature added to `migrations.FS` in `internal/database/migrations`;
   - a block in `sqlc.yaml` whose `schema` lists this feature's migrations and those of any feature whose tables it reads or references.
4. `make sqlc`. The code lands in the feature's `internal/db`.
5. A `Repository` in `repository.go` that takes the pool, runs each query on `db.New(postgres.ConnFrom(ctx, r.pool))`, maps the generated rows to the feature's types and turns driver errors into the feature's errors in `classify` (`postgres.IsNoRows` -> `ErrNotFound`, `postgres.IsUniqueViolation` -> the rule the constraint enforces). Never return generated types past the repository.
6. Construct it in `container.go` and pass it to the services that use it.
7. A test next to the repository using `pgtest.New(t, migrations.FS)`: the whole schema, cloned from a template (see `user/repository_test.go`).
8. `make migrate` against your database.

### Write to several repositories atomically

Wrap the writes in `store.WithTx` and use the `ctx` it hands you, never the outer one:

```go
err := s.store.WithTx(ctx, func(ctx context.Context) error {
    order, err := s.orders.Create(ctx, in)        // this feature's repository
    if err != nil {
        return err
    }
    if err := s.stock.Reserve(ctx, order); err != nil { // another feature's API
        return err
    }
    return s.store.Publish(ctx, EventPlaced, Placed{OrderID: order.ID})
})
```

The transaction travels in that `ctx`: every repository called with it joins the transaction, whichever feature it belongs to, and so do `Enqueue` and `Publish`. Returning an error, or panicking, rolls everything back. A `WithTx` inside another is a savepoint, so a method that needs atomicity asks for it without knowing whether its caller already did.

A call made with any other context, the outer `ctx` or `context.Background()`, runs outside the transaction and is not rolled back with it.

### Add a background job

Work caused by a request (send the receipt, process the upload) or by the clock (every hour) is a job on the queue. It survives a crash, is retried, and runs on one worker however many replicas there are (ADR 0004).

1. In the feature, a file `jobs.go`: the kind as a constant (`const KindWelcome = "account.welcome"`, `<feature>.<job>`), its payload as a struct, and the handler `func(ctx context.Context, payload T) error`. The handler has to be safe to run again: a job can run more than once.
2. Enqueue it where the cause is written, inside the same transaction:
   ```go
   err := s.store.WithTx(ctx, func(tx *store.Store) error {
       // ...the write...
       return tx.Enqueue(ctx, KindWelcome, WelcomeJob{UserID: id})
   })
   ```
   Options: `queue.In("mail")` for another queue, `queue.After(time.Hour)`, `queue.MaxAttempts(1)` for work that must never repeat, `queue.Unique(key)` to enqueue an event once.
3. Register the handler in `registerJobs` in `bootstrap/worker.go`: `queue.Register(worker, account.KindWelcome, ...)`. For the clock, also `worker.Schedule(kind, "35 6 * * *", payload)` (cron, UTC) or `worker.Periodic(kind, time.Hour, payload)`: either runs once per cluster. Give a schedule with few slots a day `queue.CatchUp(2*time.Hour)`, or a deploy that lands on its minute costs the run.
4. A test that runs the write and finds the job in `medusa_jobs`, after `queue.Install(ctx, pool)` on the test database (see `flow_test.go`).

A handler returns an error to be retried later, and `queue.Permanent(err)` when no retry can help. A job out of attempts stays in `medusa_jobs` with `status = 'failed'` and its `last_error`; `queue.Client.Failed` lists them and `Retry(id)` puts one back.

The worker serves `/metrics` and `/health` on `WORKER_PORT`: `jobs_ready`, `jobs_oldest_ready_seconds`, `jobs_failed`, `jobs_wait_seconds`, `jobs_run_duration_seconds` and `jobs_runs_total`, by queue and kind.

`job.Every` is only for work that belongs to one process, such as refreshing a cache it holds: every replica runs it, and nothing survives a crash. Never start a goroutine with `go` for long-lived work: nobody stops it, and a panic in it kills the process.

### Publish an event, or react to one

A fact that several features react to (an account was created, a post was published) is an event, not a chain of calls: the feature that produces it does not know who listens, and one reaction failing does not stop or repeat the others (ADR 0006).

1. The producer declares it in its `jobs.go`: the name as a constant (`const EventRegistered = "account.registered"`, `<feature>.<fact>` in the past tense) and its data as a struct.
2. It publishes where the fact is written, in the same transaction: `tx.Publish(ctx, EventRegistered, Registered{UserID: id})` inside `store.WithTx`.
3. A reaction is a function `func(ctx context.Context, event events.Event[Registered]) error` in the feature that reacts, with a name of its own (`const SubscriberWelcome = "account.welcome"`).
4. Subscribe it in `registerJobs` in `bootstrap/worker.go`: `events.Subscribe(subscribers, account.EventRegistered, account.SubscriberWelcome, ...)`.

A reaction runs at least once: make it safe to repeat, with `event.ID` to recognize a repeat. Release a new subscriber before the release that starts publishing its event, or deploy the workers first: a worker that does not know a subscriber does not deliver to it.

Work with one known consumer stays a job (`tx.Enqueue`). What the user must see in the same response stays a function call.

### Receive a webhook, or a callback from another service

1. Verify before anything else, on the raw body: `body, id, err := verifier.Verify(ctx.Request)` with a `webhook.Verifier` (Standard Webhooks: Svix, Resend, and Medusa's own `webhook.Signer`), or `webhook.HMAC{Header: ..., Prefix: ..., Secret: ...}.Verify` for a provider that signs the body into one header. Return the error as is: it answers 401 `webhook.invalid_signature`.
2. Store it before answering, once: enqueue a job with the raw body and `queue.Unique("<provider>:" + id)`. A redelivery finds the key taken and adds nothing, for as long as the finished job is kept (`queue.WithRetention`, 7 days by default); a provider that redelivers later than that runs again.
3. Answer 2xx. The work happens in the job's handler, which is retried from the stored body if it fails.

The route is public and outside `auth.Middleware`: the signature is the credential. Never process a webhook inline in its handler: the provider times out and redelivers, and a failure loses the event.

To send one: `signer.NewRequest(ctx, url, id, body)` and an `httpclient.Client`, from a job, with the same `id` on every retry.

### Report errors to a tracker

Medusa ships no tracker; it gives one function to call per source, each with the error, the request ID and the caller in its context.

- **Requests:** `middleware.ReportErrors(func(c *medusa.Context, err error) { ... })`, before `middleware.Recover`. It is called for every error behind a 5xx response, panics included (`*middleware.PanicError` has the stack), and never for a 4xx.
- **Jobs:** `queue.WithOnFinish(func(ctx, job, took, err) { ... })`, the same hook that feeds the metrics. A panic is a `*queue.PanicError`. Report when `job.Attempt == job.MaxAttempts` to hear of a job once, when it is given up.

Inside either, `medusa.RequestIDFrom(ctx)` and `auth.PrincipalFrom(ctx)` say which request and whose. One ID follows the work: the request's ID is stored with the jobs it enqueues, set again in their handlers, and sent as `X-Request-ID` by `httpclient` and by `webhook.Signer.NewRequest`, where the next service's `RequestID` middleware adopts it.

### Add something to Medusa

A new **battery** is a new module: its own directory with a `go.mod`, added to `go.work` and, if the app uses it, to the app's `require` and `replace` and the `COPY` lines in `app/Dockerfile`. It is for an integration with a heavy or opinionated dependency that many APIs need. Its API may use that dependency's types.

Code for the **core**, only if every answer is yes:

- Would a different API, with different users and data, use it unchanged?
- Does it avoid app concepts (users, IDs, config, a specific database)?
- Does its signature use standard library types (`context.Context`, `http.Handler`, `*slog.Logger`) or Gin at the edge?

Then: add it with a test next to it, keep the exported surface small, and run the Medusa checks. Medusa is versioned with semver; every exported name is a promise.

### Release a module

Every module (the core and each battery) is versioned and released on its own, with semver. While a module is `v0`, a breaking change bumps the minor version.

1. Each change lands with its entry under `[Unreleased]` in that module's `CHANGELOG.md` (`CHANGELOG.md` at the root for the core, `<battery>/CHANGELOG.md` for a battery). It is written for users, and a breaking change leads its line and says how to migrate.
2. To release, a PR renames `[Unreleased]` to the version with the date and adds a new empty `[Unreleased]` above it, then merges.
3. Check that `main`'s head is that merge commit (a tag on the wrong commit is permanent: the Go proxy caches it, and the only fix is a `retract` and a new version). Then tag it with an annotated tag: `v0.2.0` for the core, `<battery>/v0.2.0` for a battery. Push the tags one at a time: GitHub starts no workflow for a push of more than three tags. `release.yml` creates the GitHub release from the changelog section and publishes the version to the Go proxy.
4. The core goes first. A battery that requires the core (`auth`) then gets a PR raising its `require` to the new core version, and is tagged after that PR merges.

`main` only takes merges from PRs whose `ci` check passed.

## Commands

| Where | Command | Does |
|---|---|---|
| root | `go test ./... ./api/... ./auth/... ./env/... ./postgres/... ./storage/... ./app/...` | Tests every module (`./...` alone is only the core) |
| root | `go vet ./... ./api/... ./auth/... ./env/... ./postgres/... ./storage/... ./app/...` | Vets every module |
| `app/` | `TEST_DATABASE_URL=postgres://... go test ./...` | Also runs the database tests (skipped without the URL) |
| `app/` | `make run` / `make dev` | Runs the API / with hot reload |
| `app/` | `make worker` | Runs the worker that executes queued jobs |
| `app/` | `make migrate` | Applies pending migrations and installs the job queue's table |
| `app/` | `make sqlc` | Regenerates every feature's `internal/db` (Docker; the `sqlc` 1.30.0 release binary run in `app/` gives the same output) |
| root | `docker build -f app/Dockerfile .` | Builds the API image |

## Rules

- The core never imports a battery, the app, GORM, a database driver, a config loader or a logger other than `log/slog`.
- A battery imports at most the core and its own integration, never the app or another battery.
- Never run `go work sync`: it raises every module's dependencies to the highest version any of them uses.
- Generated code (each feature's `internal/db`, `app/api/errors.md`) is regenerated, never edited. The OpenAPI document is not generated into the repo: it is built from the routes when the API starts, and served at `/openapi.json` and `/docs` when `DOCS_ENABLED` is on.
- Migrations are additive and never run on API startup.
- Every error a client can see is an `apperr` error declared in an `errors.go`, with a `domain.rule` code. Causes go in `WithCause` and reach the log, never the response.
- Who may call a route is decided at the route: `auth.Require(scope)` for a scope, `auth.Enrich` for what needs a lookup per request (the tenant, a role). Whether the caller may touch one resource is a business rule in the service, with its declared error.
- Business code never imports `httperr` or chooses an HTTP status: it picks a `Kind`, and the edge maps it.
- Logging is `log/slog`, built once in `container.go` and passed down.
- Register global middleware before any route: Gin does not apply `Use` to routes that already exist.
- Repository methods take `ctx` first, and driver errors (`pgx.ErrNoRows`, `pgconn.PgError`) never leave the repositories: they become the feature's errors.
- A repository takes its connection from `postgres.ConnFrom(ctx, pool)` on every call, so it works inside and outside a transaction. Code that needs a transaction uses `store.WithTx` and passes its `ctx` on.
- Only a table's owner writes it. A feature never imports another feature's `internal/db`, which the compiler refuses, and never writes its tables in its own queries, which CI refuses.
- Never send `err.Error()` to a client. Declare an error for the case, or let it become `server.internal`.
- Features depend on each other in one direction (`account` imports `user`). When two features need each other, the shared part is a third feature, or one publishes an event the other subscribes to. Writes across features share a `store.WithTx`.
- Calls to other services go through `httpclient`, one client per service, never `http.DefaultClient`: it has no timeout. A dependency's rejection (`*httpclient.StatusError`) is handled where it is made or it becomes a 500; never forward a dependency's status or body to a client.
- Every goroutine has an owner that stops and waits for it: a `job`, a `lifecycle.Server`, or a function that waits before returning.
- A job is enqueued, and an event published, in the transaction of the write that causes it (`tx.Enqueue`, `tx.Publish`), never after the commit: a crash in between would lose it. Handlers and subscribers are safe to run twice.
- Split a package by feature before it reaches about 50 files.
