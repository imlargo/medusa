// Package store is what every feature shares about the database: running
// several writes as one transaction, and enqueueing jobs and publishing
// events inside it. It imports no feature, so any feature can use it,
// including those that own tables.
package store

import (
	"context"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/postgres"
	"github.com/imlargo/medusa/postgres/events"
	"github.com/imlargo/medusa/postgres/queue"
	"github.com/imlargo/medusa/tracing"
)

// Store is the database as the features share it.
type Store struct {
	pool postgres.Conn
	jobs *queue.Client
}

// New shares the database behind pool.
func New(pool postgres.Conn) *Store {
	return &Store{pool: pool, jobs: NewJobs(pool)}
}

// NewJobs is the app's client of the job queue on pool. Every job it
// enqueues remembers the ID of the request that caused it and who made it,
// so the worker builds its own client with it too.
func NewJobs(pool postgres.Conn) *queue.Client {
	return queue.New(pool, queue.WithCorrelationID(medusa.RequestIDFrom), queue.WithMetadata(callerMetadata))
}

// callerMetadata is what a job remembers about what caused it: who, so its
// handler runs as the same subject and tenant, and which trace, so its span
// continues it (see bootstrap/worker.go).
func callerMetadata(ctx context.Context) map[string]string {
	metadata := map[string]string{}
	if caller, ok := auth.PrincipalFrom(ctx); ok {
		metadata = caller.Metadata()
	}
	tracing.Inject(ctx, metadata)
	return metadata
}

// WithTx runs fn in one transaction, which travels in the context fn
// receives: every repository called with that context joins it, whichever
// feature it belongs to, and so do Enqueue and Publish. It commits when fn
// returns nil and rolls back when it returns an error or panics. Inside
// another WithTx it is a savepoint.
//
//	err := s.store.WithTx(ctx, func(ctx context.Context) error {
//	    order, err := s.orders.Create(ctx, in)
//	    if err != nil {
//	        return err
//	    }
//	    return s.store.Publish(ctx, EventPlaced, Placed{OrderID: order.ID})
//	})
//
// Pass that context on to every call that belongs to the transaction. A
// call made with the outer context runs outside it.
func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return postgres.Transact(ctx, s.pool, fn)
}

// Enqueue adds a job for the worker (see cmd/worker). Called with the
// context of a WithTx, the job is part of the transaction: it exists exactly
// when the writes around it are committed.
func (s *Store) Enqueue(ctx context.Context, kind string, payload any, opts ...queue.EnqueueOption) error {
	_, _, err := s.jobs.EnqueueTx(ctx, postgres.ConnFrom(ctx, s.pool), kind, payload, opts...)
	return err
}

// Publish announces a fact to whoever subscribes to it (see registerJobs in
// bootstrap/worker.go), each of which reacts on its own. Like Enqueue, with
// the context of a WithTx it is part of the transaction. Use it for a fact
// with several consumers, or whose consumers the feature should not know;
// work for one known consumer is Enqueue.
func (s *Store) Publish(ctx context.Context, event string, data any) error {
	_, err := events.PublishTx(ctx, s.jobs, postgres.ConnFrom(ctx, s.pool), event, data)
	return err
}
