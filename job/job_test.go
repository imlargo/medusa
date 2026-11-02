package job

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/imlargo/medusa/lifecycle"
)

// Periodic is run by lifecycle.App, so it must keep satisfying lifecycle.Server.
var _ lifecycle.Server = (*Periodic)(nil)

const interval = time.Minute

func quiet() Option { return WithLogger(slog.New(slog.DiscardHandler)) }

// startJob runs p.Start in the background and returns a channel that yields
// its result.
func startJob(p *Periodic) <-chan error {
	started := make(chan error, 1)
	go func() { started <- p.Start(context.Background()) }()
	return started
}

func stopJob(t *testing.T, p *Periodic, started <-chan error) {
	t.Helper()

	if err := p.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() = %v, want nil", err)
	}
	if err := <-started; err != nil {
		t.Errorf("Start() = %v, want nil", err)
	}
}

func TestEveryRunsOncePerInterval(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want int64
	}{
		{"first run after one interval", nil, 3},
		{"run on start", []Option{WithRunOnStart()}, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var runs atomic.Int64
				p := Every("count", interval, func(context.Context) error {
					runs.Add(1)
					return nil
				}, append(tt.opts, quiet())...)

				started := startJob(p)
				time.Sleep(3*interval + interval/2)
				synctest.Wait()

				if got := runs.Load(); got != tt.want {
					t.Errorf("runs = %d, want %d", got, tt.want)
				}
				stopJob(t, p, started)
			})
		})
	}
}

// A failing or panicking run is logged; the schedule carries on.
func TestFailuresDoNotStopTheSchedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runs atomic.Int64
		p := Every("flaky", interval, func(context.Context) error {
			switch runs.Add(1) {
			case 1:
				panic("boom")
			case 2:
				return errors.New("transient")
			}
			return nil
		}, quiet())

		started := startJob(p)
		time.Sleep(3*interval + interval/2)
		synctest.Wait()

		if got := runs.Load(); got != 3 {
			t.Errorf("runs = %d, want 3 despite a panic and an error", got)
		}
		stopJob(t, p, started)
	})
}

// A run longer than the interval delays the next one instead of overlapping.
func TestRunsNeverOverlap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var running, maxRunning atomic.Int64
		p := Every("slow", interval, func(ctx context.Context) error {
			n := running.Add(1)
			if n > maxRunning.Load() {
				maxRunning.Store(n)
			}
			defer running.Add(-1)

			select {
			case <-time.After(2*interval + interval/2):
			case <-ctx.Done():
			}
			return nil
		}, quiet())

		started := startJob(p)
		time.Sleep(10 * interval)
		synctest.Wait()

		if got := maxRunning.Load(); got != 1 {
			t.Errorf("concurrent runs = %d, want 1", got)
		}
		stopJob(t, p, started)
	})
}

// Stop cancels the run's context and waits for it to return.
func TestStopCancelsAndWaitsForTheInFlightRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var finished atomic.Bool
		p := Every("blocking", interval, func(ctx context.Context) error {
			<-ctx.Done()
			finished.Store(true)
			return ctx.Err()
		}, WithRunOnStart(), quiet())

		started := startJob(p)
		synctest.Wait()

		stopJob(t, p, started)
		if !finished.Load() {
			t.Error("Stop returned before the in-flight run did")
		}
	})
}

// A run that ignores its context cannot hold the shutdown forever.
func TestStopGivesUpAfterTheStopTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := Every("stubborn", interval, func(context.Context) error {
			time.Sleep(time.Hour)
			return nil
		}, WithRunOnStart(), WithStopTimeout(time.Second), quiet())

		started := startJob(p)
		synctest.Wait()

		if err := p.Stop(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Stop() = %v, want a deadline error", err)
		}
		<-started
	})
}

func TestStopBeforeStartPreventsTheSchedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runs atomic.Int64
		p := Every("never", interval, func(context.Context) error {
			runs.Add(1)
			return nil
		}, WithRunOnStart(), quiet())

		if err := p.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() = %v, want nil before Start", err)
		}
		if err := p.Start(context.Background()); err != nil {
			t.Fatalf("Start() = %v, want nil", err)
		}
		if got := runs.Load(); got != 0 {
			t.Errorf("runs = %d, want 0 after an early Stop", got)
		}
	})
}

func TestStartReturnsWhenItsContextIsCanceled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := Every("bounded", interval, func(context.Context) error { return nil }, quiet())

		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan error, 1)
		go func() { started <- p.Start(ctx) }()

		time.Sleep(interval / 2)
		cancel()

		if err := <-started; err != nil {
			t.Errorf("Start() = %v, want nil", err)
		}
	})
}

func TestEveryRejectsWiringMistakes(t *testing.T) {
	noop := func(context.Context) error { return nil }

	tests := []struct {
		name     string
		jobName  string
		interval time.Duration
		fn       Func
	}{
		{"empty name", "", interval, noop},
		{"zero interval", "x", 0, noop},
		{"nil func", "x", interval, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("Every did not panic")
				}
			}()
			Every(tt.jobName, tt.interval, tt.fn)
		})
	}
}
