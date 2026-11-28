# 0007: Each feature owns its tables

Date: 2026-11-28. Status: accepted. Changes the app layout of ADR 0003.

## Context

The app kept one directory of migrations, one of queries and one generated sqlc package for every feature, and a `store.Store` that held every repository so they could share a transaction. With three features it was simple. Butter's backend will have about fourteen, and that layout fails there in three ways:

- One sqlc package holding every query and every model is one package everyone imports: any feature can call any other's queries, and nothing says which feature owns a table.
- `store` imports every feature that has a repository, so no such feature can import `store` back to open a transaction. Transactional use cases had to live in features without repositories.
- In Butter's current code, the tables written by three or more parts of the system are where its bugs and its coupling are.

## Decision

**A table has one owner: the feature whose migrations create it.** Only the owner writes it.

- A feature that owns tables keeps, inside its package, its `migrations/`, its `queries/` and the sqlc package generated from them, in `internal/db`. Go's `internal` rule makes that package unimportable from any other feature, so the compiler enforces that only the owner runs its queries.
- Other features use the owner's Go API (its repository or service, its declared errors, its events).
- **Reading another feature's table is allowed when declared.** A feature lists the owner's migrations in its `schema` in `sqlc.yaml`, which lets its own queries join or select from that table. It is how a foreign key is expressed too. Listing a schema grants reading, never writing.
- **A test enforces the writes.** `TestOnlyATablesOwnerWritesIt` reads every feature's migrations and queries and fails when a query inserts into, updates or deletes from a table another feature created, and when two features create the same table.

**One migration history, files in their features.** `postgres.MergeMigrations` gathers every feature's migrations into one file system for goose, which orders them by the version in their names. New migrations are named by timestamp (`20261128153000_create_orders.sql`), so two features never choose the same version and the order is the order they were written in. A test fails when a feature's migrations are not in the history.

**A migration that arrives out of order is refused.** Two branches adding migrations produce versions in an order that can differ from the order they merge in. `postgres.Migrate` fails on a pending migration older than one already applied, and applies nothing: allowing it would give production a history in an order no test database ever had, which matters as soon as two migrations touch the same table. The fix is to rename the file. A CI job compares a branch's new migrations with the latest one on `main`, so the rename happens before the merge and not in the deploy step. That job sees `main` as it was when it ran; a branch that waited while another migration merged is caught by the same job on `main` after the merge, and by `Migrate` itself.

**A transaction travels in the context.** `postgres.Transact` (in the app, `store.WithTx`) puts the transaction in the context it gives its function, and every repository takes its connection from `postgres.ConnFrom(ctx, pool)`. Repositories of different features join the same transaction without knowing each other, and `store` imports no feature. A `Transact` inside another is a savepoint.

**Tests migrate the whole database.** A feature's test uses `pgtest.New(t, migrations.FS)`, which clones a template migrated once, so the whole schema costs a copy.

## Alternatives considered

- **A database or a schema per feature.** It breaks the joins every feature makes on users and tenants, and makes one transaction across features a distributed one.
- **Postgres roles with grants per feature.** Features share a process, so they share a role.
- **Passing the transaction as an argument** to every repository method and every cross-feature call. Explicit, and it puts a `postgres.Conn` in the signature of every service method that might ever be part of a transaction. The context is how the caller, the deadline and the request ID already travel.
- **A linter rule (depguard) instead of `internal/db`.** The compiler is stricter, needs no configuration and cannot be disabled by a comment.
- **Forbidding all reads across features.** The owner would grow a method for every report another feature needs, and joins would become loops of calls. Declared reads keep the join in SQL and the dependency visible in `sqlc.yaml`.
- **Goose with one directory per feature and separate version tables.** Foreign keys between features would depend on the order features are migrated in, which nothing would record.

## Consequences

- The transaction is implicit. A call made with a context other than the one `WithTx` handed out runs outside the transaction and is not rolled back with it. Code inside `WithTx` uses the inner `ctx` only.
- Two features that read each other's tables through `sqlc.yaml` are coupled at the schema: renaming a column breaks the reader's generated code, at `make sqlc` and in CI, not in production.
- The ownership test reads SQL with regular expressions. It sees the queries sqlc compiles; SQL built at run time would escape it, and the app has none.
- Existing migrations keep their numbers (`00001` to `00003`); timestamps sort after them.
- `store.Store` no longer has a field per repository. Services receive the repositories they use.
