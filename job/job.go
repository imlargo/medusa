// Package job runs background work on a schedule inside the process
// lifecycle.
//
// A Periodic is a lifecycle.Server, so it starts with the application and is
// stopped, and waited for, during the same ordered shutdown as the HTTP
// server:
//
//	cleanup := job.Every("expire-sessions", time.Minute, sessions.Expire,
//	    job.WithLogger(logger),
//	)
//	app := lifecycle.New(lifecycle.WithServer(srv, cleanup))
//
// Runs never overlap: a run that outlasts the interval delays the next one
// instead of starting a second copy. A run that panics is logged and the
// schedule continues. Every run gets a context that is canceled when the job
// stops, so long work can end early.
//
// The schedule is per process: with several replicas, each one runs the job.
// Work that must run once per cluster needs a lock or a queue on top.
package job

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// DefaultStopTimeout bounds how long Stop waits for an in-flight run.
const DefaultStopTimeout = 10 * time.Second

// Func is one run of a job. It should return when ctx is canceled.
type Func func(ctx context.Context) error

// Periodic runs a Func every interval until stopped.
type Periodic struct {
	name        string
	interval    time.Duration
	fn          Func
	logger      *slog.Logger
	runOnStart  bool
	stopTimeout time.Duration

	// mu guards the fields below, which Start publishes and Stop reads from
	// another goroutine.
	mu      sync.Mutex
	stopped bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// Option configures a Periodic.
type Option func(p *Periodic)

// Every returns a job that calls fn every interval once started. It panics on
// an empty name, a non-positive interval or a nil fn, which are wiring
// mistakes caught at startup.
func Every(name string, interval time.Duration, fn Func, opts ...Option) *Periodic {
	switch {
	case name == "":
		panic("job: Every called with an empty name")
	case interval <= 0:
		panic(fmt.Sprintf("job: Every(%q) called with a non-positive interval %s", name, interval))
	case fn == nil:
		panic(fmt.Sprintf("job: Every(%q) called with a nil func", name))
	}

	p := &Periodic{
		name:        name,
		interval:    interval,
		fn:          fn,
		logger:      slog.Default(),
		stopTimeout: DefaultStopTimeout,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// WithLogger sets the logger for run failures. The default is slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(p *Periodic) { p.logger = l }
}

// WithRunOnStart runs the job once as soon as it starts, instead of waiting a
// full interval for the first run.
func WithRunOnStart() Option {
	return func(p *Periodic) { p.runOnStart = true }
}

// WithStopTimeout sets how long Stop waits for an in-flight run to return
// after its context is canceled.
func WithStopTimeout(d time.Duration) Option {
	return func(p *Periodic) { p.stopTimeout = d }
}

// Start runs the schedule and blocks until Stop is called or ctx is canceled.
// It always returns nil: a failed run is logged and the schedule continues.
func (p *Periodic) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return nil
	}
	done := make(chan struct{})
	p.cancel, p.done = cancel, done
	p.mu.Unlock()
	defer close(done)

	if p.runOnStart {
		p.run(runCtx)
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-runCtx.Done():
			return nil
		case <-ticker.C:
			p.run(runCtx)
		}
	}
}

// Stop cancels the in-flight run, if any, and waits for it to return, up to
// the stop timeout or ctx's deadline, whichever comes first. Calling it more
// than once, or before Start, is safe.
func (p *Periodic) Stop(ctx context.Context) error {
	p.mu.Lock()
	p.stopped = true
	cancel, done := p.cancel, p.done
	p.mu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()

	waitCtx, waitCancel := context.WithTimeout(ctx, p.stopTimeout)
	defer waitCancel()

	select {
	case <-done:
		return nil
	case <-waitCtx.Done():
		return fmt.Errorf("job %s: in-flight run did not return within %s: %w", p.name, p.stopTimeout, waitCtx.Err())
	}
}

// run calls fn once, turning a panic into a logged failure so one bad run
// cannot take the process down.
func (p *Periodic) run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			p.logger.ErrorContext(ctx, "job panicked",
				slog.String("job", p.name),
				slog.Any("panic", r),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	start := time.Now()
	if err := p.fn(ctx); err != nil && ctx.Err() == nil {
		p.logger.ErrorContext(ctx, "job failed",
			slog.String("job", p.name),
			slog.Duration("duration", time.Since(start)),
			slog.Any("error", err),
		)
	}
}
