package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Worker defaults.
const (
	// DefaultPollInterval is how often an idle queue is checked for work.
	DefaultPollInterval = 2 * time.Second

	// DefaultLease is how long a claim lasts without being renewed. A worker
	// that dies holds its jobs for at most this long.
	DefaultLease = time.Minute

	// DefaultRetention is how long succeeded jobs are kept before they are
	// deleted. Failed jobs are never deleted.
	DefaultRetention = 7 * 24 * time.Hour

	// DefaultStopTimeout is how long running jobs get to finish once the
	// worker is told to stop.
	DefaultStopTimeout = 30 * time.Second
)

// writeTimeout bounds each write that records a job's outcome. It runs after
// the handler, when the handler's own context may already be over.
const writeTimeout = 10 * time.Second

// Job is a claimed job, as its handler sees it.
type Job struct {
	ID      int64
	Queue   string
	Kind    string
	Payload json.RawMessage

	// Attempt is 1 the first time the job runs.
	Attempt     int
	MaxAttempts int

	// CorrelationID is the ID of the request that enqueued the job, when the
	// client was built with WithCorrelationID.
	CorrelationID string

	// Metadata is what the client's WithMetadata recorded when the job was
	// enqueued. Empty, never nil, when it recorded nothing.
	Metadata map[string]string

	// Waited is how long the job was due before a worker claimed it.
	Waited time.Duration

	EnqueuedAt time.Time

	leaseToken string
}

// HandlerFunc runs a job. An error sends the job back to be retried; a
// [Permanent] one fails it at once. It must return when ctx is canceled: the
// worker is stopping, or the job's lease was lost and another worker has it.
type HandlerFunc func(ctx context.Context, job *Job) error

type handler struct {
	fn       HandlerFunc
	coalesce bool
	timeout  time.Duration
}

// HandlerOption configures how one kind of job runs.
type HandlerOption func(*handler)

// Coalesce means one run does the work of every due job of its kind: the
// handler processes whatever is pending when it starts. Claiming one then
// retires the others, so a backlog runs the job once instead of replaying
// every missed slot.
func Coalesce() HandlerOption {
	return func(h *handler) { h.coalesce = true }
}

// Timeout cancels the handler's context after d. The run then counts as a
// failed attempt.
func Timeout(d time.Duration) HandlerOption {
	return func(h *handler) { h.timeout = d }
}

// permanentError marks a failure no retry can fix.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent wraps err so the job fails without another attempt: the input is
// wrong, or the thing it refers to is gone.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// Queues names the queues a worker serves and how many jobs of each it runs
// at once.
type Queues map[string]int

// Worker claims and runs jobs. It satisfies lifecycle.Server: Start blocks
// until Stop, which lets running jobs finish.
type Worker struct {
	client    *Client
	logger    *slog.Logger
	queues    Queues
	handlers  map[string]handler
	periodic  []periodic
	scheduled []scheduled

	owner        string
	pollInterval time.Duration
	lease        time.Duration
	retention    time.Duration
	stopTimeout  time.Duration
	backoff      func(attempt int) time.Duration
	jobContext   func(ctx context.Context, job *Job) context.Context
	onFinish     func(ctx context.Context, job *Job, took time.Duration, err error)

	started   atomic.Bool
	stopLoops context.CancelFunc
	stopJobs  context.CancelFunc
	loopsDone chan struct{} // closed when no loop can claim another job
	jobs      sync.WaitGroup
	drainOnce sync.Once
	mu        sync.Mutex
}

type periodic struct {
	kind    string
	every   time.Duration
	payload any
	opts    []EnqueueOption
}

type scheduled struct {
	kind    string
	when    cron
	payload any
	opts    []EnqueueOption
	catchUp time.Duration
}

// WorkerOption configures a Worker.
type WorkerOption func(*Worker)

// WithPollInterval sets how often an idle queue is checked for work.
func WithPollInterval(d time.Duration) WorkerOption {
	return func(w *Worker) { w.pollInterval = d }
}

// WithLease sets how long a claim lasts without being renewed. The worker
// renews it three times per lease, so a lease only expires when the worker
// is gone or cannot reach the database.
func WithLease(d time.Duration) WorkerOption {
	return func(w *Worker) { w.lease = d }
}

// WithRetention sets how long succeeded jobs are kept. Zero keeps them
// forever.
func WithRetention(d time.Duration) WorkerOption {
	return func(w *Worker) { w.retention = d }
}

// WithStopTimeout sets how long running jobs get to finish once the worker
// is told to stop, [DefaultStopTimeout] by default. Keep it under the
// orchestrator's grace period.
func WithStopTimeout(d time.Duration) WorkerOption {
	return func(w *Worker) { w.stopTimeout = d }
}

// WithBackoff sets the wait before a failed job runs again; attempt is the
// number of the run that just failed.
func WithBackoff(fn func(attempt int) time.Duration) WorkerOption {
	return func(w *Worker) { w.backoff = fn }
}

// WithJobContext prepares the context a handler runs with, for example to
// put the job's CorrelationID back as the request ID so logs carry it.
func WithJobContext(fn func(ctx context.Context, job *Job) context.Context) WorkerOption {
	return func(w *Worker) { w.jobContext = fn }
}

// WithOnFinish is called after every run, with how long the handler took and
// its error. It is where metrics and error reporting hook in: ctx is the
// handler's, with whatever WithJobContext put in it (the request ID, the
// caller), and a panic arrives as a *[PanicError]. A job that will be
// retried is reported on each failed attempt; compare job.Attempt with
// job.MaxAttempts to report only the last.
func WithOnFinish(fn func(ctx context.Context, job *Job, took time.Duration, err error)) WorkerOption {
	return func(w *Worker) { w.onFinish = fn }
}

// Worker builds a worker for queues. Register every handler before starting
// it. A nil logger means slog.Default().
func (c *Client) Worker(logger *slog.Logger, queues Queues, opts ...WorkerOption) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	host, _ := os.Hostname()

	w := &Worker{
		client:       c,
		logger:       logger,
		queues:       queues,
		handlers:     map[string]handler{},
		owner:        fmt.Sprintf("%s:%d", host, os.Getpid()),
		pollInterval: DefaultPollInterval,
		lease:        DefaultLease,
		retention:    DefaultRetention,
		stopTimeout:  DefaultStopTimeout,
		backoff:      defaultBackoff,
		jobContext:   func(ctx context.Context, _ *Job) context.Context { return ctx },
		onFinish:     func(context.Context, *Job, time.Duration, error) {},
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Handle registers the handler for a kind of job. It panics on a kind
// registered twice, which is a wiring mistake found at startup.
func (w *Worker) Handle(kind string, fn HandlerFunc, opts ...HandlerOption) {
	if _, exists := w.handlers[kind]; exists {
		panic("queue: a handler for " + kind + " is already registered")
	}
	h := handler{fn: fn}
	for _, opt := range opts {
		opt(&h)
	}
	w.handlers[kind] = h
}

// Register is Handle for a handler that takes the decoded payload. A payload
// that does not decode into T fails the job permanently.
func Register[T any](w *Worker, kind string, fn func(ctx context.Context, payload T) error, opts ...HandlerOption) {
	w.Handle(kind, func(ctx context.Context, job *Job) error {
		var payload T
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return Permanent(fmt.Errorf("decode payload: %w", err))
		}
		return fn(ctx, payload)
	}, opts...)
}

// Schedule enqueues a job of kind at the minutes a cron expression names, in
// UTC: "35 6 * * *" is 06:35 every day, "40 4,16 * * *" is 04:40 and 16:40,
// "*/5 * * * *" is every five minutes. The fields are minute, hour, day of
// month, month and day of week; names and @daily shortcuts are not supported.
// It panics on an expression it cannot read, a wiring mistake found at
// startup.
//
// Every replica of the worker tries, and a unique key per minute lets exactly
// one succeed, so the job runs once per cluster. A minute that passes while
// no worker is running is lost unless the schedule has [CatchUp], which a
// job with few slots a day should. The kind needs a handler like any other.
func (w *Worker) Schedule(kind, cronExpr string, payload any, opts ...EnqueueOption) {
	when, err := parseCron(cronExpr)
	if err != nil {
		panic("queue: schedule of " + kind + ": " + err.Error())
	}

	var o enqueueOptions
	for _, opt := range opts {
		opt(&o)
	}
	w.scheduled = append(w.scheduled, scheduled{kind: kind, when: when, payload: payload, opts: opts, catchUp: o.catchUp})
}

// Periodic enqueues a job of kind once per interval. Slots are aligned to
// the clock in UTC, not to when the process started: an hourly job is due on
// the hour, on every replica. For a time of day, use [Worker.Schedule].
// Every replica of the worker tries, and a unique key per slot lets exactly
// one succeed, so the job runs once per cluster. The kind needs a handler
// like any other.
func (w *Worker) Periodic(kind string, every time.Duration, payload any, opts ...EnqueueOption) {
	if every < time.Second {
		panic("queue: the interval of periodic job " + kind + " must be at least a second")
	}
	w.periodic = append(w.periodic, periodic{kind: kind, every: every, payload: payload, opts: opts})
}

// Start claims and runs jobs until Stop is called or ctx is canceled, and
// returns once the running jobs have finished or been released.
func (w *Worker) Start(ctx context.Context) error {
	if !w.started.CompareAndSwap(false, true) {
		return errors.New("queue: the worker was already started")
	}

	loopCtx, stopLoops := context.WithCancel(ctx)
	// Jobs outlive the loops: stopping means no new claims first, and
	// interrupting what runs only when its time to finish is over.
	jobsCtx, stopJobs := context.WithCancel(context.WithoutCancel(ctx))

	loopsDone := make(chan struct{})

	w.mu.Lock()
	w.stopLoops, w.stopJobs, w.loopsDone = stopLoops, stopJobs, loopsDone
	w.mu.Unlock()

	w.logger.InfoContext(ctx, "queue worker started", slog.String("owner", w.owner), slog.Any("queues", map[string]int(w.queues)))

	var loops sync.WaitGroup
	for name, concurrency := range w.queues {
		loops.Go(func() { w.runQueue(loopCtx, jobsCtx, name, concurrency) })
	}
	loops.Go(func() { w.maintain(loopCtx) })
	loops.Wait()
	close(loopsDone)

	// Reached through Stop, which is already draining, or through ctx, where
	// nobody is: either way this returns when the drain is over.
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.stopTimeout)
	defer cancel()
	w.drain(stopCtx)

	w.logger.InfoContext(ctx, "queue worker stopped", slog.String("owner", w.owner))
	return nil
}

// Stop stops claiming jobs and waits for the running ones, until ctx is done
// or the stop timeout passes. Jobs still running then have their context
// canceled and go back to the queue without losing an attempt. Calling it
// before Start does nothing.
func (w *Worker) Stop(ctx context.Context) error {
	w.mu.Lock()
	stopLoops, loopsDone := w.stopLoops, w.loopsDone
	w.mu.Unlock()
	if stopLoops == nil {
		return nil
	}

	// The loops have to be gone before the running jobs are waited for: a
	// loop still claiming could start a job after the wait began.
	stopLoops()
	<-loopsDone

	ctx, cancel := context.WithTimeout(ctx, w.stopTimeout)
	defer cancel()
	w.drain(ctx)
	return nil
}

// drain waits for the running jobs, interrupting them when ctx ends. It runs
// once; a second caller waits for the first.
func (w *Worker) drain(ctx context.Context) {
	w.drainOnce.Do(func() {
		done := make(chan struct{})
		go func() {
			w.jobs.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			w.logger.Warn("queue worker is interrupting jobs that did not finish in time")
			w.stopJobs()
			<-done
		}
		w.stopJobs()
	})
}

// runQueue claims jobs of one queue, never holding more than concurrency.
func (w *Worker) runQueue(loopCtx, jobsCtx context.Context, name string, concurrency int) {
	slots := make(chan struct{}, concurrency)
	for range concurrency {
		slots <- struct{}{}
	}

	for {
		// Wait for one free slot, then take every other free one too.
		select {
		case <-loopCtx.Done():
			return
		case <-slots:
		}
		free := 1
		for len(slots) > 0 {
			<-slots
			free++
		}

		claimed, err := w.client.claim(loopCtx, name, free, w.owner, w.lease)
		if err != nil && loopCtx.Err() == nil {
			w.logger.ErrorContext(loopCtx, "queue claim failed", slog.String("queue", name), slog.Any("error", err))
		}
		for range free - len(claimed) {
			slots <- struct{}{}
		}
		for _, job := range claimed {
			w.jobs.Go(func() {
				defer func() { slots <- struct{}{} }()
				w.run(jobsCtx, job)
			})
		}

		// A full batch means there is probably more: claim again as soon as a
		// slot frees. Otherwise the queue is empty, or failing, and waits.
		if len(claimed) == free && err == nil {
			continue
		}
		select {
		case <-loopCtx.Done():
			return
		case <-time.After(w.pollInterval):
		}
	}
}

// run executes one claimed job and records its outcome.
func (w *Worker) run(jobsCtx context.Context, job *Job) {
	h, ok := w.handlers[job.Kind]
	if !ok {
		// Another build of the worker knows this kind: a deploy is rolling.
		// Hand it back without spending an attempt.
		w.logger.Warn("queue has a job of a kind with no handler", slog.String("kind", job.Kind), slog.Int64("job", job.ID))
		w.record(job, func(ctx context.Context) (bool, error) {
			return w.client.release(ctx, job, 5*w.pollInterval)
		})
		return
	}

	ctx, cancel := context.WithCancel(w.jobContext(jobsCtx, job))
	defer cancel()
	if h.timeout > 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, h.timeout)
		defer cancelTimeout()
	}

	if h.coalesce {
		if err := w.client.supersede(ctx, job); err != nil {
			w.logger.ErrorContext(ctx, "queue could not retire the jobs this one covers", slog.Int64("job", job.ID), slog.Any("error", err))
		}
	}

	// The lease is renewed while the handler runs. Losing it means another
	// worker has the job: this run stops, and its outcome is not recorded.
	var lost atomic.Bool
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		w.heartbeat(ctx, job, &lost, cancel)
	}()

	start := time.Now()
	err := w.call(ctx, h.fn, job)
	took := time.Since(start)

	cancel()
	<-heartbeatDone
	w.onFinish(ctx, job, took, err)

	attrs := []any{slog.String("kind", job.Kind), slog.Int64("job", job.ID), slog.Int("attempt", job.Attempt), slog.Duration("took", took)}
	switch {
	case lost.Load():
		w.logger.Warn("queue job lost its lease; another worker has it", attrs...)
	case err == nil:
		w.logger.Debug("queue job succeeded", attrs...)
		w.record(job, func(ctx context.Context) (bool, error) { return w.client.succeed(ctx, job) })
	case jobsCtx.Err() != nil:
		// Interrupted by shutdown, not failed: back to the queue as it was.
		w.logger.Info("queue job released on shutdown", attrs...)
		w.record(job, func(ctx context.Context) (bool, error) { return w.client.release(ctx, job, 0) })
	default:
		_, permanent := errors.AsType[*permanentError](err)
		w.logger.Error("queue job failed", append(attrs, slog.Any("error", err), slog.Bool("permanent", permanent))...)
		w.record(job, func(ctx context.Context) (bool, error) {
			return w.client.fail(ctx, job, err, permanent, w.backoff(job.Attempt))
		})
	}
}

// PanicError is the error a handler's panic becomes. The run counts as a
// failed attempt, and WithOnFinish receives this error with the stack.
type PanicError struct {
	// Value is what was passed to panic.
	Value any

	// Stack is the stack of the goroutine that panicked.
	Stack []byte
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

// call runs the handler and turns a panic into an error, so one bad job
// cannot take the worker down.
func (w *Worker) call(ctx context.Context, fn HandlerFunc, job *Job) (err error) {
	defer func() {
		if v := recover(); v != nil {
			stack := debug.Stack()
			w.logger.ErrorContext(ctx, "queue job panicked", slog.String("kind", job.Kind), slog.Int64("job", job.ID), slog.Any("panic", v), slog.String("stack", string(stack)))
			err = &PanicError{Value: v, Stack: stack}
		}
	}()
	return fn(ctx, job)
}

// heartbeat renews the job's lease until ctx ends, and cancels the run when
// the lease is gone.
func (w *Worker) heartbeat(ctx context.Context, job *Job, lost *atomic.Bool, cancel context.CancelFunc) {
	ticker := time.NewTicker(w.lease / 3)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		owned, err := w.client.renew(ctx, job, w.lease)
		if err != nil {
			// The database is unreachable: keep trying until the lease
			// expires there, where another worker's reaper decides.
			continue
		}
		if !owned {
			lost.Store(true)
			cancel()
			return
		}
	}
}

// record writes a job's outcome, on a context of its own: the handler's may
// already be canceled. A write that finds the lease gone is only logged, as
// another worker owns the job by then.
func (w *Worker) record(job *Job, write func(ctx context.Context) (bool, error)) {
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()

	owned, err := write(ctx)
	switch {
	case err != nil:
		w.logger.Error("queue could not record a job's outcome; it reruns when its lease expires", slog.Int64("job", job.ID), slog.Any("error", err))
	case !owned:
		w.logger.Warn("queue job's lease was gone when it finished", slog.Int64("job", job.ID))
	}
}

// maintain does the work no single job owns: returning jobs whose worker
// died, deleting old finished jobs, and enqueueing periodic ones.
func (w *Worker) maintain(ctx context.Context) {
	reap := time.NewTicker(w.lease / 2)
	defer reap.Stop()
	clean := time.NewTicker(time.Hour)
	defer clean.Stop()
	schedule := time.NewTicker(time.Second)
	defer schedule.Stop()

	lastSlot := make([]time.Time, len(w.periodic))
	// The last minute whose schedules were enqueued. Starting one minute back
	// makes the first tick handle the current minute.
	lastMinute := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
	caughtUp := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-reap.C:
			if n, err := w.client.reap(ctx); err != nil {
				w.logger.ErrorContext(ctx, "queue reaper failed", slog.Any("error", err))
			} else if n > 0 {
				w.logger.WarnContext(ctx, "queue recovered jobs whose lease expired", slog.Int64("jobs", n))
			}
		case <-clean.C:
			if w.retention > 0 {
				if err := w.client.deleteFinished(ctx, w.retention); err != nil {
					w.logger.ErrorContext(ctx, "queue retention failed", slog.Any("error", err))
				}
			}
		case now := <-schedule.C:
			for i, p := range w.periodic {
				slot := now.UTC().Truncate(p.every)
				if slot.Equal(lastSlot[i]) {
					continue
				}
				key := fmt.Sprintf("periodic:%s:%s", p.kind, slot.Format(time.RFC3339))
				opts := append([]EnqueueOption{Unique(key)}, p.opts...)
				if _, _, err := w.client.Enqueue(ctx, p.kind, p.payload, opts...); err != nil {
					w.logger.ErrorContext(ctx, "queue could not enqueue a periodic job", slog.String("kind", p.kind), slog.Any("error", err))
					continue // tried again on the next tick
				}
				lastSlot[i] = slot
			}
			if !caughtUp {
				// Tried on every tick until it works, so a database that is
				// down when the worker starts does not cost the missed runs.
				caughtUp = w.enqueueMissed(ctx, lastMinute)
			}
			lastMinute = w.enqueueScheduled(ctx, lastMinute, now.UTC().Truncate(time.Minute))
		}
	}
}

// maxCatchUp is how far back a worker that stalled makes up for the minutes
// its own ticks skipped.
const maxCatchUp = time.Hour

// enqueueScheduled enqueues the scheduled jobs of every minute after last, up
// to current, and returns the last minute it finished. A minute that fails is
// tried again on the next tick; its unique keys make that safe.
func (w *Worker) enqueueScheduled(ctx context.Context, last, current time.Time) time.Time {
	if current.Sub(last) > maxCatchUp {
		last = current.Add(-maxCatchUp)
	}

	for minute := last.Add(time.Minute); !minute.After(current); minute = minute.Add(time.Minute) {
		for _, s := range w.scheduled {
			if !s.when.matches(minute) {
				continue
			}
			if _, err := w.enqueueSlot(ctx, s, minute); err != nil {
				w.logger.ErrorContext(ctx, "queue could not enqueue a scheduled job", slog.String("kind", s.kind), slog.Any("error", err))
				return last
			}
		}
		last = minute
	}
	return last
}

// enqueueMissed enqueues, for each schedule with a catch-up window, its most
// recent slot at or before upTo that is inside the window. The slot's unique
// key decides whether it was missed: when its job exists, nothing is added.
// It reports whether every schedule was handled.
func (w *Worker) enqueueMissed(ctx context.Context, upTo time.Time) bool {
	for _, s := range w.scheduled {
		oldest := upTo.Add(-s.catchUp)
		for minute := upTo; s.catchUp > 0 && !minute.Before(oldest); minute = minute.Add(-time.Minute) {
			if !s.when.matches(minute) {
				continue
			}
			enqueued, err := w.enqueueSlot(ctx, s, minute)
			if err != nil {
				w.logger.ErrorContext(ctx, "queue could not make up a missed scheduled job", slog.String("kind", s.kind), slog.Any("error", err))
				return false
			}
			if enqueued {
				w.logger.InfoContext(ctx, "queue made up a scheduled job that was missed", slog.String("kind", s.kind), slog.Time("slot", minute))
			}
			break // only the most recent slot
		}
	}
	return true
}

// enqueueSlot enqueues the job of one minute of a schedule, once: the key is
// the same on every replica and for every later attempt.
func (w *Worker) enqueueSlot(ctx context.Context, s scheduled, minute time.Time) (bool, error) {
	key := fmt.Sprintf("schedule:%s:%s", s.kind, minute.Format(time.RFC3339))
	opts := append([]EnqueueOption{Unique(key)}, s.opts...)
	_, enqueued, err := w.client.Enqueue(ctx, s.kind, s.payload, opts...)
	return enqueued, err
}

// defaultBackoff waits 5s, 10s, 20s... up to 10 minutes, give or take a
// fifth so jobs that failed together do not come back together.
func defaultBackoff(attempt int) time.Duration {
	wait := min(5*time.Second<<min(attempt-1, 10), 10*time.Minute)
	jitter := wait / 5
	return wait - jitter + rand.N(2*jitter+1) //nolint:gosec // G404: jitter spreads retries out, it guards nothing
}
