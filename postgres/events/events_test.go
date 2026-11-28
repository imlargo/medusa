package events_test

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

	"github.com/imlargo/medusa/postgres/events"
	"github.com/imlargo/medusa/postgres/pgtest"
	"github.com/imlargo/medusa/postgres/queue"
)

var noMigrations = fstest.MapFS{"00001_init.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}

type postPublished struct {
	PostID int `json:"post_id"`
}

func setup(t *testing.T) (*queue.Client, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.New(t, noMigrations)
	if err := queue.Install(t.Context(), pool); err != nil {
		t.Fatalf("Install() = %v", err)
	}
	return queue.New(pool), pool
}

// start runs a worker with subscribers attached until the test ends.
func start(t *testing.T, jobs *queue.Client, subscribers *events.Subscribers) {
	t.Helper()

	worker := jobs.Worker(slog.New(slog.DiscardHandler), queue.Queues{queue.DefaultQueue: 4},
		queue.WithPollInterval(20*time.Millisecond),
		queue.WithBackoff(func(int) time.Duration { return 0 }),
	)
	subscribers.Attach(worker)

	done := make(chan error, 1)
	go func() { done <- worker.Start(context.Background()) }()
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = worker.Stop(stopCtx)
		if err := <-done; err != nil {
			t.Errorf("Start() = %v", err)
		}
	})
}

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

// One fact, two reactions. The one that fails is retried on its own: the
// other runs once, and both see the same event ID.
func TestEachSubscriberIsDeliveredAndRetriedOnItsOwn(t *testing.T) {
	t.Parallel()
	jobs, _ := setup(t)

	var (
		mu          sync.Mutex
		analyticsID []string
		billingID   []string
	)
	subscribers := events.NewSubscribers(jobs)
	events.Subscribe(subscribers, "posting.post_published", "analytics.track_post", func(_ context.Context, e events.Event[postPublished]) error {
		mu.Lock()
		defer mu.Unlock()
		analyticsID = append(analyticsID, e.ID)
		if e.Data.PostID != 7 || e.Name != "posting.post_published" || e.PublishedAt.IsZero() {
			t.Errorf("event = %+v, want post 7 of posting.post_published with its time", e)
		}
		return nil
	})
	events.Subscribe(subscribers, "posting.post_published", "billing.count_post", func(_ context.Context, e events.Event[postPublished]) error {
		mu.Lock()
		defer mu.Unlock()
		billingID = append(billingID, e.ID)
		if len(billingID) == 1 {
			return errors.New("billing is down")
		}
		return nil
	})
	start(t, jobs, subscribers)

	id, err := events.Publish(t.Context(), jobs, "posting.post_published", postPublished{PostID: 7})
	if err != nil || id == "" {
		t.Fatalf("Publish() = %q, %v", id, err)
	}

	eventually(t, "billing to succeed on its second delivery", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(billingID) == 2
	})

	mu.Lock()
	defer mu.Unlock()
	if len(analyticsID) != 1 {
		t.Errorf("analytics ran %d times, want once: another subscriber's failure is not its retry", len(analyticsID))
	}
	for _, got := range append(analyticsID, billingID...) {
		if got != id {
			t.Errorf("a delivery had event ID %q, want %q on every one", got, id)
		}
	}
}

func TestAnEventPublishedInARolledBackTransactionIsNotDelivered(t *testing.T) {
	t.Parallel()
	jobs, pool := setup(t)
	ctx := t.Context()

	var delivered atomic.Int32
	subscribers := events.NewSubscribers(jobs)
	events.Subscribe(subscribers, "order.paid", "mail.receipt", func(context.Context, events.Event[struct{}]) error {
		delivered.Add(1)
		return nil
	})
	start(t, jobs, subscribers)

	rollback := errors.New("the order could not be saved")
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := events.PublishTx(ctx, jobs, tx, "order.paid", struct{}{}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("BeginFunc() = %v", err)
	}

	// A committed one, to know the worker has had its turn.
	if _, err := events.Publish(ctx, jobs, "order.paid", struct{}{}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the committed event to be delivered", func() bool { return delivered.Load() >= 1 })
	time.Sleep(100 * time.Millisecond)

	if got := delivered.Load(); got != 1 {
		t.Errorf("deliveries = %d, want only the committed event", got)
	}
}

func TestAnEventNobodySubscribesToIsNotAnError(t *testing.T) {
	t.Parallel()
	jobs, pool := setup(t)
	ctx := t.Context()

	start(t, jobs, events.NewSubscribers(jobs))

	if _, err := events.Publish(ctx, jobs, "nobody.cares", map[string]int{"n": 1}); err != nil {
		t.Fatalf("Publish() = %v", err)
	}

	eventually(t, "the event to be settled", func() bool {
		var open int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM medusa_jobs WHERE status <> 'succeeded'").Scan(&open)
		return open == 0
	})
	var jobCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM medusa_jobs").Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 {
		t.Errorf("jobs = %d, want only the event itself", jobCount)
	}
}

// The event's job can run twice (a crash after enqueueing the subscribers,
// before recording success). Each subscriber must still get one job.
func TestDeliveringAnEventTwiceGivesEachSubscriberOneJob(t *testing.T) {
	t.Parallel()
	jobs, pool := setup(t)
	ctx := t.Context()

	var runs atomic.Int32
	subscribers := events.NewSubscribers(jobs)
	events.Subscribe(subscribers, "order.paid", "mail.receipt", func(context.Context, events.Event[struct{}]) error {
		runs.Add(1)
		return nil
	})
	start(t, jobs, subscribers)

	if _, err := events.Publish(ctx, jobs, "order.paid", struct{}{}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the subscriber to run", func() bool { return runs.Load() == 1 })

	// Send the event's own job around again.
	if _, err := pool.Exec(ctx, "UPDATE medusa_jobs SET status = 'queued', finished_at = NULL WHERE kind = 'medusa.event'"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the event to be settled again", func() bool {
		var open int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM medusa_jobs WHERE status <> 'succeeded'").Scan(&open)
		return open == 0
	})

	var receipts int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM medusa_jobs WHERE kind = 'mail.receipt'").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 || runs.Load() != 1 {
		t.Errorf("mail.receipt jobs = %d, runs = %d, want 1 and 1", receipts, runs.Load())
	}
}

func TestSubscriberNamesAreUnique(t *testing.T) {
	jobs := queue.New(nil)
	subscribers := events.NewSubscribers(jobs)
	handler := func(context.Context, events.Event[struct{}]) error { return nil }
	events.Subscribe(subscribers, "a.happened", "b.react", handler)

	defer func() {
		if recover() == nil {
			t.Error("Subscribe() with a name used twice did not panic")
		}
	}()
	events.Subscribe(subscribers, "c.happened", "b.react", handler)
}
