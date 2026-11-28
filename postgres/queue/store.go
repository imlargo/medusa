package queue

import (
	"context"
	"fmt"
	"time"
)

// Every write that ends or extends a claim names the lease token, and
// reports whether it matched a row. A worker whose lease expired and was
// taken over therefore cannot overwrite the new owner's job: its writes match
// nothing. Times are the database's, so workers with skewed clocks agree on
// when a lease ends.

// claim takes up to limit due jobs of queue for owner.
func (c *Client) claim(ctx context.Context, queue string, limit int, owner string, lease time.Duration) ([]*Job, error) {
	rows, err := c.conn.Query(ctx, `
		UPDATE medusa_jobs
		SET status = 'running',
		    attempts = attempts + 1,
		    lease_token = gen_random_uuid(),
		    lease_owner = $3,
		    lease_expires = now() + make_interval(secs => $4::double precision),
		    started_at = now()
		WHERE id IN (
		    SELECT id FROM medusa_jobs
		    WHERE queue = $1 AND status = 'queued' AND scheduled_for <= now()
		    ORDER BY priority DESC, scheduled_for, id
		    LIMIT $2
		    FOR UPDATE SKIP LOCKED
		)
		RETURNING id, queue, kind, payload, attempts, max_attempts, correlation_id, lease_token::text, enqueued_at,
		          EXTRACT(EPOCH FROM now() - scheduled_for)::double precision`,
		queue, limit, owner, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim from %s: %w", queue, err)
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		var (
			job    Job
			waited float64
		)
		if err := rows.Scan(&job.ID, &job.Queue, &job.Kind, &job.Payload, &job.Attempt, &job.MaxAttempts, &job.CorrelationID, &job.leaseToken, &job.EnqueuedAt, &waited); err != nil {
			return nil, fmt.Errorf("claim from %s: %w", queue, err)
		}
		job.Waited = time.Duration(waited * float64(time.Second))
		jobs = append(jobs, &job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim from %s: %w", queue, err)
	}
	return jobs, nil
}

// renew extends the job's lease.
func (c *Client) renew(ctx context.Context, job *Job, lease time.Duration) (bool, error) {
	return c.fenced(ctx, job, `
		UPDATE medusa_jobs
		SET lease_expires = now() + make_interval(secs => $3::double precision)
		WHERE id = $1 AND lease_token = $2::uuid AND status = 'running'`,
		lease.Seconds())
}

// succeed ends the job.
func (c *Client) succeed(ctx context.Context, job *Job) (bool, error) {
	return c.fenced(ctx, job, `
		UPDATE medusa_jobs
		SET status = 'succeeded', finished_at = now(), active_key = NULL,
		    lease_token = NULL, lease_owner = '', lease_expires = NULL
		WHERE id = $1 AND lease_token = $2::uuid AND status = 'running'`)
}

// fail records a failed run: back to the queue after retryIn while attempts
// remain and the failure is not permanent, failed for good otherwise.
func (c *Client) fail(ctx context.Context, job *Job, cause error, permanent bool, retryIn time.Duration) (bool, error) {
	return c.fenced(ctx, job, `
		UPDATE medusa_jobs
		SET status        = CASE WHEN attempts < max_attempts AND NOT $4 THEN 'queued' ELSE 'failed' END,
		    scheduled_for = CASE WHEN attempts < max_attempts AND NOT $4 THEN now() + make_interval(secs => $5::double precision) ELSE scheduled_for END,
		    finished_at   = CASE WHEN attempts < max_attempts AND NOT $4 THEN NULL ELSE now() END,
		    active_key    = CASE WHEN attempts < max_attempts AND NOT $4 THEN active_key ELSE NULL END,
		    last_error = $3,
		    lease_token = NULL, lease_owner = '', lease_expires = NULL
		WHERE id = $1 AND lease_token = $2::uuid AND status = 'running'`,
		cause.Error(), permanent, retryIn.Seconds())
}

// release hands the job back as if it had not been claimed: the attempt is
// not spent. It is due again after delay.
func (c *Client) release(ctx context.Context, job *Job, delay time.Duration) (bool, error) {
	return c.fenced(ctx, job, `
		UPDATE medusa_jobs
		SET status = 'queued',
		    attempts = GREATEST(attempts - 1, 0),
		    scheduled_for = now() + make_interval(secs => $3::double precision),
		    lease_token = NULL, lease_owner = '', lease_expires = NULL
		WHERE id = $1 AND lease_token = $2::uuid AND status = 'running'`,
		delay.Seconds())
}

// fenced runs a write that only the holder of the job's lease may make, and
// reports whether this worker still is that holder.
func (c *Client) fenced(ctx context.Context, job *Job, sql string, args ...any) (bool, error) {
	tag, err := c.conn.Exec(ctx, sql, append([]any{job.ID, job.leaseToken}, args...)...)
	if err != nil {
		return false, fmt.Errorf("job %d: %w", job.ID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// supersede retires the other due jobs of the claimed job's kind.
func (c *Client) supersede(ctx context.Context, job *Job) error {
	_, err := c.conn.Exec(ctx, `
		UPDATE medusa_jobs
		SET status = 'superseded', finished_at = now(), active_key = NULL
		WHERE queue = $1 AND kind = $2 AND status = 'queued' AND scheduled_for <= now() AND id <> $3`,
		job.Queue, job.Kind, job.ID)
	if err != nil {
		return fmt.Errorf("job %d: %w", job.ID, err)
	}
	return nil
}

// reap returns running jobs whose lease expired: their worker died or lost
// the database. The interrupted run counts as an attempt, so a job that must
// not repeat (MaxAttempts(1)) is failed instead of run again.
func (c *Client) reap(ctx context.Context) (int64, error) {
	tag, err := c.conn.Exec(ctx, `
		UPDATE medusa_jobs
		SET status      = CASE WHEN attempts < max_attempts THEN 'queued' ELSE 'failed' END,
		    finished_at = CASE WHEN attempts < max_attempts THEN NULL ELSE now() END,
		    active_key  = CASE WHEN attempts < max_attempts THEN active_key ELSE NULL END,
		    last_error = 'the worker stopped renewing the lease before the job ended',
		    lease_token = NULL, lease_owner = '', lease_expires = NULL
		WHERE id IN (
		    SELECT id FROM medusa_jobs
		    WHERE status = 'running' AND lease_expires < now()
		    FOR UPDATE SKIP LOCKED
		)`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// deleteFinished removes succeeded and superseded jobs older than retention.
// Failed jobs stay: they are the record of work that did not happen.
func (c *Client) deleteFinished(ctx context.Context, retention time.Duration) error {
	_, err := c.conn.Exec(ctx, `
		DELETE FROM medusa_jobs
		WHERE status IN ('succeeded', 'superseded')
		  AND finished_at < now() - make_interval(secs => $1::double precision)`,
		retention.Seconds())
	return err
}
