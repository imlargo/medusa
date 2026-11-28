package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imlargo/medusa/postgres/pgtest"
)

// noMigrations is the application schema of these tests: the queue installs
// its own table.
var noMigrations = fstest.MapFS{"00001_init.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}

func newClient(t *testing.T, opts ...ClientOption) (*Client, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.New(t, noMigrations)
	if err := Install(t.Context(), pool); err != nil {
		t.Fatalf("Install() = %v", err)
	}
	return New(pool, opts...), pool
}

// startWorker runs w until the test ends.
func startWorker(t *testing.T, w *Worker) {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- w.Start(context.Background()) }()
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = w.Stop(stopCtx)
		<-done
	})
}

func fastWorker(c *Client, queues Queues, opts ...WorkerOption) *Worker {
	opts = append([]WorkerOption{
		WithPollInterval(20 * time.Millisecond),
		WithBackoff(func(int) time.Duration { return 0 }),
	}, opts...)
	return c.Worker(slog.New(slog.DiscardHandler), queues, opts...)
}

// eventually polls check until it holds.
func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

type jobRow struct {
	status    string
	attempts  int
	lastError string
}

func readJob(t *testing.T, pool *pgxpool.Pool, id int64) jobRow {
	t.Helper()

	var row jobRow
	err := pool.QueryRow(t.Context(), "SELECT status, attempts, last_error FROM medusa_jobs WHERE id = $1", id).
		Scan(&row.status, &row.attempts, &row.lastError)
	if err != nil {
		t.Fatalf("read job %d: %v", id, err)
	}
	return row
}

type receipt struct {
	OrderID int `json:"order_id"`
}

func TestInstallIsIdempotent(t *testing.T) {
	t.Parallel()
	_, pool := newClient(t)

	if err := Install(t.Context(), pool); err != nil {
		t.Errorf("second Install() = %v, want nil", err)
	}
}

func TestAJobRunsWithItsPayload(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t, WithCorrelationID(func(context.Context) string { return "req-1" }))

	got := make(chan *Job, 1)
	var payload atomic.Int64
	w := fastWorker(c, Queues{DefaultQueue: 2}, WithJobContext(func(ctx context.Context, job *Job) context.Context {
		got <- job
		return ctx
	}))
	Register(w, "orders.send_receipt", func(_ context.Context, in receipt) error {
		payload.Store(int64(in.OrderID))
		return nil
	})
	startWorker(t, w)

	id, enqueued, err := c.Enqueue(t.Context(), "orders.send_receipt", receipt{OrderID: 42})
	if err != nil || !enqueued {
		t.Fatalf("Enqueue() = %d, %v, %v", id, enqueued, err)
	}

	eventually(t, "the job to succeed", func() bool { return readJob(t, pool, id).status == "succeeded" })
	if payload.Load() != 42 {
		t.Errorf("handler got order %d, want 42", payload.Load())
	}
	if job := <-got; job.CorrelationID != "req-1" || job.Attempt != 1 {
		t.Errorf("job = %+v, want the correlation ID req-1 on attempt 1", job)
	}
}

// The reason to keep the queue in the application's database: the job exists
// exactly when the change that causes it does.
func TestAJobEnqueuedInARolledBackTransactionDoesNotExist(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)
	ctx := t.Context()

	rollback := errors.New("the order could not be saved")
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, _, err := c.EnqueueTx(ctx, tx, "orders.send_receipt", receipt{OrderID: 1}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("BeginFunc() = %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM medusa_jobs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("jobs = %d, want none after the rollback", count)
	}
}

func TestUniqueKeys(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)
	ctx := t.Context()

	first, enqueued, err := c.Enqueue(ctx, "sync", nil, Unique("sync:2026-11-27"), UniqueWhileActive("sync"))
	if err != nil || !enqueued {
		t.Fatalf("first Enqueue() = %v, %v", enqueued, err)
	}
	if id, enqueued, err := c.Enqueue(ctx, "sync", nil, Unique("sync:2026-11-27")); err != nil || enqueued || id != 0 {
		t.Errorf("Enqueue() with a taken unique key = %d, %v, %v, want 0, false, nil", id, enqueued, err)
	}
	if _, enqueued, err := c.Enqueue(ctx, "sync", nil, UniqueWhileActive("sync")); err != nil || enqueued {
		t.Errorf("Enqueue() with a taken active key = %v, %v, want false, nil", enqueued, err)
	}

	w := fastWorker(c, Queues{DefaultQueue: 1})
	w.Handle("sync", func(context.Context, *Job) error { return nil })
	startWorker(t, w)
	eventually(t, "the first job to succeed", func() bool { return readJob(t, pool, first).status == "succeeded" })

	// The unique key outlives the job; the active key does not.
	if _, enqueued, _ := c.Enqueue(ctx, "sync", nil, Unique("sync:2026-11-27")); enqueued {
		t.Error("the unique key was free again after its job finished")
	}
	if _, enqueued, err := c.Enqueue(ctx, "sync", nil, UniqueWhileActive("sync")); err != nil || !enqueued {
		t.Errorf("Enqueue() after the active job ended = %v, %v, want true", enqueued, err)
	}
}

func TestAFailingJobIsRetriedUntilItsAttemptsRunOut(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)

	var runs atomic.Int32
	w := fastWorker(c, Queues{DefaultQueue: 1})
	w.Handle("flaky", func(context.Context, *Job) error {
		runs.Add(1)
		return errors.New("the provider is down")
	})
	startWorker(t, w)

	id, _, err := c.Enqueue(t.Context(), "flaky", nil, MaxAttempts(3))
	if err != nil {
		t.Fatal(err)
	}

	eventually(t, "the job to fail for good", func() bool { return readJob(t, pool, id).status == "failed" })
	row := readJob(t, pool, id)
	if runs.Load() != 3 || row.attempts != 3 {
		t.Errorf("runs = %d, attempts = %d, want 3 and 3", runs.Load(), row.attempts)
	}
	if row.lastError != "the provider is down" {
		t.Errorf("last_error = %q, want the handler's error", row.lastError)
	}
}

func TestAPermanentFailureIsNotRetried(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)

	var runs atomic.Int32
	w := fastWorker(c, Queues{DefaultQueue: 1})
	w.Handle("bad_input", func(context.Context, *Job) error {
		runs.Add(1)
		return Permanent(errors.New("the order does not exist"))
	})
	startWorker(t, w)

	id, _, err := c.Enqueue(t.Context(), "bad_input", nil, MaxAttempts(5))
	if err != nil {
		t.Fatal(err)
	}

	eventually(t, "the job to fail", func() bool { return readJob(t, pool, id).status == "failed" })
	if runs.Load() != 1 {
		t.Errorf("runs = %d, want 1", runs.Load())
	}
}

func TestAPanicFailsTheJobAndTheWorkerKeepsRunning(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)

	w := fastWorker(c, Queues{DefaultQueue: 1})
	w.Handle("panics", func(context.Context, *Job) error { panic("boom") })
	w.Handle("fine", func(context.Context, *Job) error { return nil })
	startWorker(t, w)

	bad, _, _ := c.Enqueue(t.Context(), "panics", nil, MaxAttempts(1))
	good, _, _ := c.Enqueue(t.Context(), "fine", nil)

	eventually(t, "both jobs to end", func() bool {
		return readJob(t, pool, bad).status == "failed" && readJob(t, pool, good).status == "succeeded"
	})
}

func TestClaimOrderIsPriorityThenDueTime(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t)
	ctx := t.Context()

	low, _, _ := c.Enqueue(ctx, "k", nil)
	high, _, _ := c.Enqueue(ctx, "k", nil, Priority(10))
	other, _, _ := c.Enqueue(ctx, "k", nil, In("other"))
	later, _, _ := c.Enqueue(ctx, "k", nil, After(time.Hour))

	claimed, err := c.claim(ctx, DefaultQueue, 10, "test", time.Minute)
	if err != nil {
		t.Fatalf("claim() = %v", err)
	}
	if len(claimed) != 2 || claimed[0].ID != high || claimed[1].ID != low {
		t.Errorf("claimed = %v, want the high priority job then the low one", jobIDs(claimed))
	}
	for _, job := range claimed {
		if job.ID == other || job.ID == later {
			t.Errorf("claimed job %d, which is in another queue or not due", job.ID)
		}
	}
}

// The fence: a worker that stalls past its lease loses the job, and nothing
// it writes afterwards may touch the run that replaced it.
func TestAnExpiredLeaseIsTakenOverAndItsOldHolderIsFencedOut(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)
	ctx := t.Context()

	id, _, _ := c.Enqueue(ctx, "k", nil, MaxAttempts(2))

	stalled, err := c.claim(ctx, DefaultQueue, 1, "stalled", time.Minute)
	if err != nil || len(stalled) != 1 {
		t.Fatalf("claim() = %v, %v", jobIDs(stalled), err)
	}
	if _, err := pool.Exec(ctx, "UPDATE medusa_jobs SET lease_expires = now() - interval '1 second' WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}

	if n, err := c.reap(ctx); err != nil || n != 1 {
		t.Fatalf("reap() = %d, %v, want 1", n, err)
	}
	taken, err := c.claim(ctx, DefaultQueue, 1, "healthy", time.Minute)
	if err != nil || len(taken) != 1 || taken[0].Attempt != 2 {
		t.Fatalf("second claim() = %v, %v, want the job on attempt 2", jobIDs(taken), err)
	}

	if owned, err := c.renew(ctx, stalled[0], time.Minute); err != nil || owned {
		t.Errorf("renew() by the stalled worker = %v, %v, want false", owned, err)
	}
	if owned, err := c.succeed(ctx, stalled[0]); err != nil || owned {
		t.Errorf("succeed() by the stalled worker = %v, %v, want false", owned, err)
	}
	if got := readJob(t, pool, id).status; got != "running" {
		t.Errorf("status = %q, want the new owner's run untouched", got)
	}

	if owned, err := c.succeed(ctx, taken[0]); err != nil || !owned {
		t.Errorf("succeed() by the owner = %v, %v, want true", owned, err)
	}
}

// A job that must not repeat is failed, not run again, when its worker dies.
func TestAnInterruptedJobWithOneAttemptFails(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)
	ctx := t.Context()

	id, _, _ := c.Enqueue(ctx, "post_to_instagram", nil, MaxAttempts(1))
	if _, err := c.claim(ctx, DefaultQueue, 1, "dies", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE medusa_jobs SET lease_expires = now() - interval '1 second' WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}

	if _, err := c.reap(ctx); err != nil {
		t.Fatal(err)
	}
	if row := readJob(t, pool, id); row.status != "failed" || row.lastError == "" {
		t.Errorf("job = %+v, want it failed with a reason", row)
	}
}

func TestStopReleasesARunningJobWithoutSpendingAnAttempt(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)

	started := make(chan struct{})
	w := fastWorker(c, Queues{DefaultQueue: 1})
	w.Handle("slow", func(ctx context.Context, _ *Job) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})

	done := make(chan error, 1)
	go func() { done <- w.Start(context.Background()) }()

	id, _, _ := c.Enqueue(t.Context(), "slow", nil)
	<-started

	stopCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := w.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Start() = %v", err)
	}

	if row := readJob(t, pool, id); row.status != "queued" || row.attempts != 0 {
		t.Errorf("job = %+v, want it queued again with no attempt spent", row)
	}
}

func TestStopWaitsForARunningJob(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)

	started := make(chan struct{})
	release := make(chan struct{})
	w := fastWorker(c, Queues{DefaultQueue: 1})
	w.Handle("slow", func(context.Context, *Job) error {
		close(started)
		<-release
		return nil
	})

	done := make(chan error, 1)
	go func() { done <- w.Start(context.Background()) }()

	id, _, _ := c.Enqueue(t.Context(), "slow", nil)
	<-started

	stopped := make(chan struct{})
	go func() {
		_ = w.Stop(context.Background())
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("Stop returned while a job was running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-stopped
	<-done

	if got := readJob(t, pool, id).status; got != "succeeded" {
		t.Errorf("status = %q, want succeeded", got)
	}
}

func TestConcurrencyIsPerQueue(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)

	var (
		mu      sync.Mutex
		running int
		peak    int
	)
	w := fastWorker(c, Queues{"narrow": 2})
	w.Handle("k", func(context.Context, *Job) error {
		mu.Lock()
		running++
		peak = max(peak, running)
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		running--
		mu.Unlock()
		return nil
	})
	startWorker(t, w)

	for range 8 {
		if _, _, err := c.Enqueue(t.Context(), "k", nil, In("narrow")); err != nil {
			t.Fatal(err)
		}
	}

	eventually(t, "every job to succeed", func() bool {
		var left int
		_ = pool.QueryRow(t.Context(), "SELECT count(*) FROM medusa_jobs WHERE status <> 'succeeded'").Scan(&left)
		return left == 0
	})
	mu.Lock()
	defer mu.Unlock()
	if peak != 2 {
		t.Errorf("peak concurrency = %d, want 2", peak)
	}
}

func TestCoalesceRunsABacklogOnce(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)
	ctx := t.Context()

	for range 5 {
		if _, _, err := c.Enqueue(ctx, "track_analytics", nil); err != nil {
			t.Fatal(err)
		}
	}

	var runs atomic.Int32
	w := fastWorker(c, Queues{DefaultQueue: 1})
	w.Handle("track_analytics", func(context.Context, *Job) error {
		runs.Add(1)
		return nil
	}, Coalesce())
	startWorker(t, w)

	eventually(t, "the backlog to be settled", func() bool {
		var open int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM medusa_jobs WHERE status IN ('queued', 'running')").Scan(&open)
		return open == 0
	})

	var superseded int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM medusa_jobs WHERE status = 'superseded'").Scan(&superseded); err != nil {
		t.Fatal(err)
	}
	if runs.Load() != 1 || superseded != 4 {
		t.Errorf("runs = %d, superseded = %d, want 1 and 4", runs.Load(), superseded)
	}
}

// Two replicas schedule the same periodic job; each slot must run once.
func TestAPeriodicJobRunsOncePerSlotAcrossWorkers(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)
	ctx := t.Context()

	for range 2 {
		w := fastWorker(c, Queues{DefaultQueue: 1})
		w.Handle("every_second", func(context.Context, *Job) error { return nil })
		w.Periodic("every_second", time.Second, nil)
		startWorker(t, w)
	}

	eventually(t, "two slots to be enqueued", func() bool {
		var jobs int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM medusa_jobs WHERE kind = 'every_second'").Scan(&jobs)
		return jobs >= 2
	})

	var jobs, slots int
	if err := pool.QueryRow(ctx, "SELECT count(*), count(DISTINCT unique_key) FROM medusa_jobs WHERE kind = 'every_second'").Scan(&jobs, &slots); err != nil {
		t.Fatal(err)
	}
	if jobs != slots {
		t.Errorf("%d jobs for %d slots, want one job per slot", jobs, slots)
	}
}

func TestStatsReportDepthAndAge(t *testing.T) {
	t.Parallel()
	c, pool := newClient(t)
	ctx := t.Context()

	old, _, _ := c.Enqueue(ctx, "k", nil, In("mail"))
	_, _, _ = c.Enqueue(ctx, "k", nil, In("mail"))
	_, _, _ = c.Enqueue(ctx, "k", nil, In("mail"), After(time.Hour))
	if _, err := pool.Exec(ctx, "UPDATE medusa_jobs SET scheduled_for = now() - interval '90 seconds' WHERE id = $1", old); err != nil {
		t.Fatal(err)
	}

	stats, err := c.Stats(ctx)
	if err != nil || len(stats) != 1 {
		t.Fatalf("Stats() = %+v, %v, want one queue", stats, err)
	}
	s := stats[0]
	if s.Queue != "mail" || s.Ready != 2 || s.Scheduled != 1 || s.Running != 0 || s.Failed != 0 {
		t.Errorf("stats = %+v, want mail with 2 ready and 1 scheduled", s)
	}
	if s.OldestReady < 90*time.Second || s.OldestReady > 5*time.Minute {
		t.Errorf("OldestReady = %s, want about 90s", s.OldestReady)
	}
}

func jobIDs(jobs []*Job) []int64 {
	ids := make([]int64, len(jobs))
	for i, job := range jobs {
		ids[i] = job.ID
	}
	return ids
}
