// Package health runs dependency checks for readiness probes and serves the
// liveness and readiness endpoints.
//
// A check is a function, so any client with a ping fits without an adapter:
//
//	checks := health.New(5 * time.Second)
//	checks.Register("database", pool.Ping)
//	checks.Register("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })
package health

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultTimeout bounds a full round of checks when New is given zero.
const DefaultTimeout = 5 * time.Second

// Status values reported for each check and for the whole report. They are
// part of the /ready response contract.
const (
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
)

// CheckFunc reports a dependency as unhealthy by returning an error.
type CheckFunc func(ctx context.Context) error

// Result is the outcome of one check. Err is never serialized: a driver error
// can name hosts, users and databases, and /ready is public.
type Result struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Err    error  `json:"-"`
}

// Report is the combined outcome of every check. Draining means the instance
// is shutting down: it is unhealthy on purpose, and no check ran.
type Report struct {
	Status   string   `json:"status"`
	Draining bool     `json:"draining,omitempty"`
	Checks   []Result `json:"checks,omitempty"`
}

type check struct {
	name string
	fn   CheckFunc
}

// Service holds the registered checks. Register them during initialization,
// before the first call to Check.
type Service struct {
	checks   []check
	timeout  time.Duration
	draining atomic.Bool
}

// New creates a service whose rounds of checks are bounded by timeout, or by
// DefaultTimeout when timeout is zero.
func New(timeout time.Duration) *Service {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &Service{timeout: timeout}
}

// Register adds a named check. It panics on an empty name or a nil function,
// both of which are wiring mistakes caught at startup.
func (s *Service) Register(name string, fn CheckFunc) {
	if name == "" {
		panic("health: Register called with an empty name")
	}
	if fn == nil {
		panic("health: Register called with a nil check for " + name)
	}
	s.checks = append(s.checks, check{name: name, fn: fn})
}

// Drain takes the instance out of rotation before it shuts down: from now on
// Check reports it unhealthy, so /ready answers 503, and Drain waits delay
// (or until ctx is done) for the load balancer to notice and stop sending
// requests. Run it as the first shutdown hook, before the server closes its
// listener; without it, requests routed during that window are refused.
//
// The delay should cover a few readiness probe periods. It is spent before
// the shutdown budget, so the orchestrator's grace period has to cover both.
func (s *Service) Drain(ctx context.Context, delay time.Duration) error {
	s.draining.Store(true)
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
	return nil
}

// Check runs every registered check and reports the combined status.
//
// Checks run concurrently, so a probe over N dependencies costs the slowest
// one rather than the sum. The timeout bounds the whole round, and results
// keep registration order so the response is stable across calls.
func (s *Service) Check(ctx context.Context) Report {
	if s.draining.Load() {
		return Report{Status: StatusUnhealthy, Draining: true}
	}

	checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	results := make([]Result, len(s.checks))

	var wg sync.WaitGroup
	for i, c := range s.checks {
		wg.Go(func() {
			result := Result{Name: c.name, Status: StatusHealthy}
			if err := c.fn(checkCtx); err != nil {
				result.Status = StatusUnhealthy
				result.Err = err
			}

			// Each goroutine owns exactly one slot, so no lock is needed.
			results[i] = result
		})
	}
	wg.Wait()

	status := StatusHealthy
	for _, result := range results {
		if result.Status != StatusHealthy {
			status = StatusUnhealthy
			break
		}
	}

	return Report{Status: status, Checks: results}
}
