-- The queue's schema. Every statement is idempotent, so Install can run on
-- every deploy: a new version of the queue adds what is missing.

CREATE TABLE IF NOT EXISTS medusa_jobs (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    queue          text        NOT NULL,
    kind           text        NOT NULL,
    payload        jsonb       NOT NULL,
    status         text        NOT NULL DEFAULT 'queued'
                   CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'superseded')),
    priority       integer     NOT NULL DEFAULT 0,
    attempts       integer     NOT NULL DEFAULT 0,
    max_attempts   integer     NOT NULL,
    scheduled_for  timestamptz NOT NULL DEFAULT now(),
    unique_key     text,
    active_key     text,
    correlation_id text        NOT NULL DEFAULT '',
    lease_token    uuid,
    lease_owner    text        NOT NULL DEFAULT '',
    lease_expires  timestamptz,
    last_error     text        NOT NULL DEFAULT '',
    enqueued_at    timestamptz NOT NULL DEFAULT now(),
    started_at     timestamptz,
    finished_at    timestamptz
);

-- What a claim reads: the next due job of a queue, best priority first.
CREATE INDEX IF NOT EXISTS medusa_jobs_claim
    ON medusa_jobs (queue, priority DESC, scheduled_for, id)
    WHERE status = 'queued';

-- What the reaper reads: running jobs by lease expiry.
CREATE INDEX IF NOT EXISTS medusa_jobs_lease
    ON medusa_jobs (lease_expires)
    WHERE status = 'running';

-- A unique key is taken for as long as its row exists.
CREATE UNIQUE INDEX IF NOT EXISTS medusa_jobs_unique_key
    ON medusa_jobs (unique_key)
    WHERE unique_key IS NOT NULL;

-- An active key is cleared when its job ends, so it allows one job at a time.
CREATE UNIQUE INDEX IF NOT EXISTS medusa_jobs_active_key
    ON medusa_jobs (active_key)
    WHERE active_key IS NOT NULL;

-- What retention deletes: finished jobs by age.
CREATE INDEX IF NOT EXISTS medusa_jobs_finished
    ON medusa_jobs (finished_at)
    WHERE finished_at IS NOT NULL;
