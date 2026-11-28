# 0006: Events are an outbox on the job queue

Date: 2026-11-28. Status: accepted.

## Context

Some facts have several consumers. In Butter, a published post triggers seven reactions written as chained calls inside one handler, and a deleted subscription seven more, most of whose failures are logged and swallowed: one failing reaction either aborts the rest or is lost. Its design (ADR 0005 there) routes every such fan-out through a transactional outbox, and module code cannot take its final shape without it.

What it needs: an event published inside the writer's transaction; a delivery per subscriber with its own retries; at least once, with an event ID to tell a repeat; subscribers declared in code; and no error for an event nobody subscribes to.

## Decision

`postgres/events` delivers events as jobs of `postgres/queue`. There is no outbox table and no dispatcher process of its own.

- **Publishing enqueues one job**, of the kind `medusa.event`, through the caller's transaction. It carries the event's name, a generated ID, the time and the data.
- **A worker turns that job into one job per subscriber.** The subscribers are the ones registered in the worker's process. Each subscriber's job is named after the subscriber and has its own attempts, so its retries, its metrics and its failed jobs are its own.
- **The fan-out is safe to repeat.** Each subscriber's job has a unique key made of the event's ID and the subscriber's name, so a fan-out that ran twice, or failed halfway, adds only what is missing.
- **An event without subscribers ends there**, as a job that did nothing.
- **The subscriber sees the same event ID on every delivery**, and is expected to be safe to run twice.

## Alternatives considered

- **One job per subscriber enqueued by the publisher.** One step fewer, and Butter would accept it. But the publisher, usually the API, would have to know every subscriber, and during a rolling deploy an API of the old version would silently skip a subscriber added in the new one for every event it published. With the fan-out in the worker the same gap exists only for the events a worker of the old version happens to pick up.
- **An outbox table with its own dispatcher.** The queue already is a transactional table with leases, retries and a reaper; a second one would repeat it.
- **A broker (NATS, Kafka, Redis streams).** A system to operate, and no publishing in the writer's transaction without an outbox in front of it anyway. The events can feed a broker later if a consumer outside the codebase needs them.
- **Postgres `LISTEN/NOTIFY`.** A consumer that is down misses the notification.

## Consequences

- Reactions are eventually consistent, normally within the worker's poll interval, twice: once for the event and once for the subscriber. What a user must see in the same response stays a direct call.
- **Deploy order matters for a new subscriber.** A worker of the old version that picks up an event does not know the new subscriber, which then never sees that event. Release a subscriber before the release that starts publishing its event, or deploy workers before the processes that publish.
- Events are not kept as a log: the job is deleted with the queue's retention. Replaying history to a new subscriber is not supported.
- Ordering between events is not guaranteed.
