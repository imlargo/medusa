# 0004: A job queue on Postgres, written in Medusa

Date: 2026-11-27. Status: accepted.

## Context

`job.Every` was Medusa's only background primitive. It runs on every replica, keeps nothing, and a crash loses the run in progress. Butter's backend, the first consumer, needs a `worker` process that runs work enqueued by the API, with these properties:

- a job is enqueued in the same transaction as the write that causes it;
- a job runs on one worker at a time, and survives that worker dying;
- failed jobs are retried, and jobs that ran out of attempts stay visible;
- each queue has its own concurrency;
- periodic jobs run once per cluster;
- depth and age can be read the same way for every queue.

Butter already runs two implementations of these semantics on Postgres, and its ADR 0004 keeps them over a library.

## Decision

Medusa ships the queue as `postgres/queue`, a package of the `postgres` battery, written on `FOR UPDATE SKIP LOCKED`. It adds no dependency.

- **One table**, `medusa_jobs`, installed and upgraded by `queue.Install` with idempotent statements. It runs in the migration step, next to the app's goose migrations and outside their version table.
- **A claim is a lease with a token.** Claiming sets a fresh token and an expiry. Renewing the lease, and every write that ends the job, names the token and reports whether it matched. A worker that stalls past its lease cannot overwrite the worker that took the job over. The handler of a lost job has its context canceled.
- **Times are the database's**, so workers with skewed clocks agree on when a lease ends.
- **An expired lease counts as an attempt.** The job goes back to the queue while attempts remain, and fails otherwise. `MaxAttempts(1)` is therefore how a job says "never repeat me": it fails instead of running twice.
- **Expired leases are recovered by a reaper** in each worker, on a partial index, not inside every claim.
- **Two kinds of unique key.** `Unique` holds for as long as the job's row exists, finished or not; `UniqueWhileActive` frees when the job ends. Both are unique across kinds, not per kind, so two kinds that must not run at once share an active key.
- **Clock work is enqueued once per slot.** `Schedule` takes a cron expression read in UTC and `Periodic` an interval aligned to the clock, not to process start. Every replica tries to enqueue each slot with one `Unique` key, and one succeeds. A minute that passes with no worker running is lost, unless the schedule has `CatchUp(window)`: a worker that starts then enqueues the schedule's most recent slot inside the window, and the slot's key, read from the table and not from memory, tells a missed run from one already done. It is opt-in because making up a run is only right for jobs that should still happen late. A worker that stalls makes up the minutes its own ticks skipped, up to an hour.
- **Failed jobs are an operator's queue.** `Failed` lists them by queue and kind and `Retry` puts one back with its attempts restored. A job that is unsafe to repeat takes one attempt and stops there until a person decides.
- **Stopping releases, it does not fail.** A worker told to stop claims nothing more, waits for running jobs, and hands back those that did not finish without spending their attempt.
- It lives in the `postgres` module because a battery never imports another, and it needs `postgres.Conn` to enqueue inside the caller's transaction.

## Alternatives considered

- **River.** A maintained library with these features and a UI. Rejected by imlargo (D7): Butter would migrate four queues to a new dependency instead of converging on semantics it has tested in production, and River's leases have no fencing token exposed to the handler's writes.
- **Redis (Asynq).** Loses the transactional enqueue, which is the reason to keep the queue next to the data.
- **A broker (NATS, SQS).** A stateful system to operate for volumes Postgres handles, and no transactional enqueue.
- **A separate `queue` module.** It could not import `postgres.Conn`, or the rule that batteries do not import each other would need an exception.
- **Goose migrations for the queue's table.** Two migration sets cannot share one version table, and a second table per package is more to explain than "run Install".

## Consequences

- At-least-once: a handler has to be safe to run again, or declare one attempt.
- **Two runs of one job can overlap.** Canceling a context only stops a handler that listens to it. A handler stuck in a subprocess or a blocking call keeps running after its lease expires, while the reaper hands the job to a second worker. The fence protects the queue's row, not the handler's own writes: a handler with side effects must be idempotent, or check that it still matters before its final write.
- Polling, every 2 seconds by default. `LISTEN/NOTIFY` can replace it later without changing callers.
- Medusa maintains the queue. Features River has and this does not: a UI, unique jobs by arguments, workflows. Cron expressions are five numeric fields: no names, no `@daily`, no time zones.
- Metrics are exported by the core's `metrics.NewJobs`, which knows no queue: the app adapts `Stats` and reports each run from `WithOnFinish`.
- `job.Every` stays for work that belongs to one process: refreshing a cache it holds, flushing a buffer.
