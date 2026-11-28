// Package queue is a job queue on Postgres: work that must survive a crash,
// run on one worker at a time, and be retried when it fails.
//
// A job is a row. It is enqueued in the same transaction as the change that
// causes it, so it exists exactly when that change is committed:
//
//	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
//	    if err := orders.MarkPaid(ctx, tx, id); err != nil {
//	        return err
//	    }
//	    _, _, err := jobs.EnqueueTx(ctx, tx, "orders.send_receipt", Receipt{OrderID: id})
//	    return err
//	})
//
// A [Worker] claims jobs with FOR UPDATE SKIP LOCKED, so any number of
// replicas share a queue without a job running twice at once:
//
//	worker := jobs.Worker(logger, queue.Queues{"default": 10, "mail": 2})
//	queue.Register(worker, "orders.send_receipt", receipts.Send)
//	// run it with lifecycle, next to or instead of the HTTP server
//
// # What a job is promised
//
// At least once. A claim is a lease with a token: the worker renews it while
// the handler runs, and every write that ends the job names the token. A
// worker that stalls past its lease loses the job to another one, its
// handler's context is canceled, and its late result is discarded. Handlers
// therefore have to be safe to run again; give a job MaxAttempts(1) when it is
// not, and it fails instead of repeating.
//
// A handler that returns an error is retried later, waiting longer each time,
// until its attempts run out. Then the job is failed, and stays in the table
// until someone looks at it.
//
// # Schema
//
// The queue owns one table, medusa_jobs. [Install] creates and upgrades it,
// and is safe to run on every deploy, next to the application's migrations.
package queue

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/imlargo/medusa/postgres"
)

// Defaults for a job enqueued without the matching option.
const (
	// DefaultQueue is the queue a job goes to without In.
	DefaultQueue = "default"

	// DefaultMaxAttempts is how many times a job runs before it is failed.
	DefaultMaxAttempts = 3
)

//go:embed schema.sql
var schema string

// installLock serializes Install across the replicas of a deploy.
const installLock = "medusa_jobs_install"

// Install creates the queue's table and indexes, or adds what a newer version
// needs. Every statement is idempotent, so run it on every deploy, as part of
// the migration step and never on API startup.
func Install(ctx context.Context, conn postgres.Conn) error {
	err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", installLock); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, schema)
		return err
	})
	if err != nil {
		return fmt.Errorf("queue: install schema: %w", err)
	}
	return nil
}

// Client enqueues jobs and builds workers. It is safe for concurrent use.
type Client struct {
	conn      postgres.Conn
	correlate func(context.Context) string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithCorrelationID sets how a job learns the ID of the request that
// enqueued it, usually medusa.RequestIDFrom. The worker hands it back to
// [WithJobContext], so one ID follows the work from the request into the job.
func WithCorrelationID(from func(context.Context) string) ClientOption {
	return func(c *Client) { c.correlate = from }
}

// New builds a client on conn, the pool.
func New(conn postgres.Conn, opts ...ClientOption) *Client {
	c := &Client{conn: conn, correlate: func(context.Context) string { return "" }}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type enqueueOptions struct {
	queue       string
	priority    int
	maxAttempts int
	at          *time.Time
	after       time.Duration
	uniqueKey   *string
	activeKey   *string
}

// EnqueueOption shapes one job.
type EnqueueOption func(*enqueueOptions)

// In sends the job to a queue other than [DefaultQueue]. Queues separate
// work that must not wait behind other work, each with its own concurrency.
func In(queue string) EnqueueOption {
	return func(o *enqueueOptions) { o.queue = queue }
}

// Priority runs the job before jobs of a lower priority in its queue. The
// default is 0; negative values wait behind everything else.
func Priority(priority int) EnqueueOption {
	return func(o *enqueueOptions) { o.priority = priority }
}

// MaxAttempts sets how many times the job runs before it is failed. Use 1
// for work that must never repeat: it then fails, instead of running again,
// even when the worker dies in the middle of it.
func MaxAttempts(n int) EnqueueOption {
	return func(o *enqueueOptions) { o.maxAttempts = n }
}

// At runs the job no earlier than t.
func At(t time.Time) EnqueueOption {
	return func(o *enqueueOptions) { o.at = &t }
}

// After runs the job no earlier than d from now, by the database's clock.
func After(d time.Duration) EnqueueOption {
	return func(o *enqueueOptions) { o.after = d }
}

// Unique enqueues the job only if no job with this key exists, finished ones
// included. It is how the same event, or the same slot of a schedule, is
// enqueued once however many times it is reported. The key frees when the
// finished job is deleted (see [WithRetention]).
func Unique(key string) EnqueueOption {
	return func(o *enqueueOptions) { o.uniqueKey = &key }
}

// UniqueWhileActive enqueues the job only if no queued or running job has
// this key: at most one at a time, and a new one as soon as it ends.
func UniqueWhileActive(key string) EnqueueOption {
	return func(o *enqueueOptions) { o.activeKey = &key }
}

// Enqueue adds a job outside a transaction. See [Client.EnqueueTx].
func (c *Client) Enqueue(ctx context.Context, kind string, payload any, opts ...EnqueueOption) (int64, bool, error) {
	return c.EnqueueTx(ctx, c.conn, kind, payload, opts...)
}

// EnqueueTx adds a job of kind with payload, encoded as JSON, through tx: the
// job is visible to workers when tx commits, and never if it rolls back.
//
// It returns the job's ID and true. When [Unique] or [UniqueWhileActive]
// finds its key taken, nothing is added and it returns 0 and false, which is
// not an error.
func (c *Client) EnqueueTx(ctx context.Context, tx postgres.Conn, kind string, payload any, opts ...EnqueueOption) (int64, bool, error) {
	if kind == "" {
		return 0, false, errors.New("queue: enqueue: kind is empty")
	}

	o := enqueueOptions{queue: DefaultQueue, maxAttempts: DefaultMaxAttempts}
	for _, opt := range opts {
		opt(&o)
	}
	if o.queue == "" || o.maxAttempts < 1 {
		return 0, false, fmt.Errorf("queue: enqueue %s: the queue must be named and the attempts at least 1", kind)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, false, fmt.Errorf("queue: enqueue %s: encode payload: %w", kind, err)
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO medusa_jobs (queue, kind, payload, priority, max_attempts, scheduled_for, unique_key, active_key, correlation_id)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6::timestamptz, now() + make_interval(secs => $7::double precision)), $8, $9, $10)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		o.queue, kind, encoded, o.priority, o.maxAttempts, o.at, o.after.Seconds(), o.uniqueKey, o.activeKey, c.correlate(ctx),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("queue: enqueue %s: %w", kind, err)
	}
	return id, true, nil
}

// FailedJob is a job that ran out of attempts, as an operator sees it.
type FailedJob struct {
	ID        int64
	Queue     string
	Kind      string
	Payload   json.RawMessage
	Attempts  int
	LastError string
	FailedAt  time.Time
}

// FailedFilter selects failed jobs. Empty fields match everything.
type FailedFilter struct {
	Queue string
	Kind  string

	// Limit is the most jobs returned, 100 when zero.
	Limit int

	// BeforeID returns jobs older than this one, to page through a long
	// list: pass the ID of the last job of the previous page.
	BeforeID int64
}

// Failed lists failed jobs, newest first. A failed job stays until it is
// retried: it is the record of work that did not happen, and for a job that
// must not repeat, the point where a person decides.
func (c *Client) Failed(ctx context.Context, filter FailedFilter) ([]FailedJob, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	rows, err := c.conn.Query(ctx, `
		SELECT id, queue, kind, payload, attempts, last_error, finished_at
		FROM medusa_jobs
		WHERE status = 'failed'
		  AND ($1 = '' OR queue = $1)
		  AND ($2 = '' OR kind = $2)
		  AND ($3 = 0 OR id < $3)
		ORDER BY id DESC
		LIMIT $4`,
		filter.Queue, filter.Kind, filter.BeforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("queue: list failed jobs: %w", err)
	}
	defer rows.Close()

	var jobs []FailedJob
	for rows.Next() {
		var job FailedJob
		if err := rows.Scan(&job.ID, &job.Queue, &job.Kind, &job.Payload, &job.Attempts, &job.LastError, &job.FailedAt); err != nil {
			return nil, fmt.Errorf("queue: list failed jobs: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("queue: list failed jobs: %w", err)
	}
	return jobs, nil
}

// Retry puts a failed job back in its queue, due now, with all its attempts
// again. It reports false when id is not a failed job: it does not exist, or
// someone already retried it.
func (c *Client) Retry(ctx context.Context, id int64) (bool, error) {
	tag, err := c.conn.Exec(ctx, `
		UPDATE medusa_jobs
		SET status = 'queued', attempts = 0, scheduled_for = now(), finished_at = NULL
		WHERE id = $1 AND status = 'failed'`, id)
	if err != nil {
		return false, fmt.Errorf("queue: retry job %d: %w", id, err)
	}
	return tag.RowsAffected() == 1, nil
}

// Stats is the state of one queue, for dashboards and alerts.
type Stats struct {
	Queue string

	// Ready jobs are due and waiting for a worker: the queue's depth.
	Ready int64

	// Scheduled jobs are waiting for their time: a later run, or a retry.
	Scheduled int64

	Running int64

	// Failed jobs ran out of attempts. They stay until someone handles them.
	Failed int64

	// OldestReady is how long the oldest ready job has waited. It grows when
	// workers cannot keep up, whatever the depth.
	OldestReady time.Duration
}

// Stats returns the state of every queue that holds an unfinished job.
func (c *Client) Stats(ctx context.Context) ([]Stats, error) {
	rows, err := c.conn.Query(ctx, `
		SELECT queue,
		       count(*) FILTER (WHERE status = 'queued' AND scheduled_for <= now()),
		       count(*) FILTER (WHERE status = 'queued' AND scheduled_for > now()),
		       count(*) FILTER (WHERE status = 'running'),
		       count(*) FILTER (WHERE status = 'failed'),
		       COALESCE(EXTRACT(EPOCH FROM now() - min(scheduled_for) FILTER (WHERE status = 'queued' AND scheduled_for <= now())), 0)::double precision
		FROM medusa_jobs
		WHERE status IN ('queued', 'running', 'failed')
		GROUP BY queue
		ORDER BY queue`)
	if err != nil {
		return nil, fmt.Errorf("queue: stats: %w", err)
	}
	defer rows.Close()

	var stats []Stats
	for rows.Next() {
		var (
			s      Stats
			oldest float64
		)
		if err := rows.Scan(&s.Queue, &s.Ready, &s.Scheduled, &s.Running, &s.Failed, &oldest); err != nil {
			return nil, fmt.Errorf("queue: stats: %w", err)
		}
		s.OldestReady = time.Duration(oldest * float64(time.Second))
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("queue: stats: %w", err)
	}
	return stats, nil
}
