// Package events delivers a fact to everyone who reacts to it: a
// transactional outbox on the job queue, with no broker.
//
// A module that produces a fact publishes it in the transaction that writes
// it, so the event exists exactly when the fact does:
//
//	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
//	    if err := posts.MarkPublished(ctx, tx, id); err != nil {
//	        return err
//	    }
//	    _, err := events.PublishTx(ctx, jobs, tx, "posting.post_published", PostPublished{PostID: id})
//	    return err
//	})
//
// Any number of subscribers react, each on its own:
//
//	subscribers := events.NewSubscribers(jobs)
//	events.Subscribe(subscribers, "posting.post_published", "analytics.track_post", analytics.TrackPost)
//	events.Subscribe(subscribers, "posting.post_published", "billing.count_post", billing.CountPost)
//	subscribers.Attach(worker)
//
// # What a subscriber is promised
//
// Each subscriber gets the event as a job of its own, named after the
// subscriber, with its own attempts and retries: one that fails does not
// stop the others and is not run again because another failed. Delivery is
// at least once, so a subscriber has to be safe to run twice for the same
// event; [Event.ID] is the same on every delivery, to tell.
//
// Publishing an event nobody subscribes to is not an error.
//
// # Deploying
//
// A worker hands an event to the subscribers it knows. During a rolling
// deploy, a worker of the old version may pick up an event that only the new
// version has a subscriber for, and that subscriber then never sees it.
// Release a new subscriber before the release that starts publishing its
// event, or deploy workers before the processes that publish.
//
// Use an event for a fact with more than one consumer, or whose consumers
// the producer should not know. Work for one known consumer is a job; a
// question that needs an answer is a function call.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"uuid"

	"github.com/imlargo/medusa/postgres"
	"github.com/imlargo/medusa/postgres/queue"
)

// dispatchKind is the kind of the job a published event is. Its handler
// turns it into one job per subscriber.
const dispatchKind = "medusa.event"

// envelope is an event as it travels in a job's payload.
type envelope struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	PublishedAt time.Time       `json:"published_at"`
	Data        json.RawMessage `json:"data"`
}

// Event is an event as a subscriber receives it.
type Event[T any] struct {
	// ID identifies the event. It is the same on every delivery and for
	// every subscriber: use it to do the work once.
	ID string

	// Name is what happened, as "<domain>.<fact>": "posting.post_published".
	Name string

	PublishedAt time.Time

	Data T
}

// Publish publishes an event outside a transaction. See [PublishTx].
func Publish(ctx context.Context, jobs *queue.Client, name string, data any, opts ...queue.EnqueueOption) (string, error) {
	return publish(ctx, name, data, func(env envelope) error {
		_, _, err := jobs.Enqueue(ctx, dispatchKind, env, opts...)
		return err
	})
}

// PublishTx publishes the event name with data, encoded as JSON, through tx:
// it is delivered when tx commits and never if it rolls back. It returns the
// event's ID. opts shape the job that carries the event to its subscribers
// (queue.In, queue.Priority), not the subscribers' own jobs.
func PublishTx(ctx context.Context, jobs *queue.Client, tx postgres.Conn, name string, data any, opts ...queue.EnqueueOption) (string, error) {
	return publish(ctx, name, data, func(env envelope) error {
		_, _, err := jobs.EnqueueTx(ctx, tx, dispatchKind, env, opts...)
		return err
	})
}

func publish(ctx context.Context, name string, data any, enqueue func(envelope) error) (string, error) {
	if name == "" {
		return "", fmt.Errorf("events: publish: the event has no name")
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("events: publish %s: encode data: %w", name, err)
	}

	env := envelope{ID: uuid.New().String(), Name: name, PublishedAt: time.Now().UTC(), Data: encoded}
	if err := enqueue(env); err != nil {
		return "", fmt.Errorf("events: publish %s: %w", name, err)
	}
	return env.ID, nil
}

// Subscribers is who reacts to which event in this process. Fill it at
// startup, before attaching it to a worker.
type Subscribers struct {
	jobs    *queue.Client
	byEvent map[string][]subscriber
	names   map[string]bool
}

type subscriber struct {
	name    string
	handler queue.HandlerFunc
	enqueue []queue.EnqueueOption
	handle  []queue.HandlerOption
}

// NewSubscribers delivers events through jobs.
func NewSubscribers(jobs *queue.Client) *Subscribers {
	return &Subscribers{jobs: jobs, byEvent: map[string][]subscriber{}, names: map[string]bool{}}
}

// SubscribeOption configures one subscriber.
type SubscribeOption func(*subscriber)

// WithEnqueue shapes the subscriber's jobs: its queue (queue.In), its
// attempts (queue.MaxAttempts), its priority.
func WithEnqueue(opts ...queue.EnqueueOption) SubscribeOption {
	return func(s *subscriber) { s.enqueue = append(s.enqueue, opts...) }
}

// WithHandler configures how the subscriber runs: queue.Timeout.
func WithHandler(opts ...queue.HandlerOption) SubscribeOption {
	return func(s *subscriber) { s.handle = append(s.handle, opts...) }
}

// Subscribe has fn react to every event named event. name identifies the
// subscriber: it is the kind of its jobs, what its metrics and failed jobs
// are listed under, and it has to be unique in the application
// ("<domain>.<reaction>": "analytics.track_post"). It panics on a name used
// twice, a wiring mistake found at startup.
//
// Data that does not decode into T fails the subscriber's job permanently.
func Subscribe[T any](s *Subscribers, event, name string, fn func(ctx context.Context, event Event[T]) error, opts ...SubscribeOption) {
	if s.names[name] {
		panic("events: a subscriber named " + name + " is already registered")
	}
	s.names[name] = true

	sub := subscriber{
		name: name,
		handler: func(ctx context.Context, job *queue.Job) error {
			var env envelope
			if err := json.Unmarshal(job.Payload, &env); err != nil {
				return queue.Permanent(fmt.Errorf("decode event: %w", err))
			}
			var data T
			if err := json.Unmarshal(env.Data, &data); err != nil {
				return queue.Permanent(fmt.Errorf("decode data of %s: %w", env.Name, err))
			}
			return fn(ctx, Event[T]{ID: env.ID, Name: env.Name, PublishedAt: env.PublishedAt, Data: data})
		},
	}
	for _, opt := range opts {
		opt(&sub)
	}
	s.byEvent[event] = append(s.byEvent[event], sub)
}

// Attach registers on worker what delivering needs: the handler that turns a
// published event into one job per subscriber, and each subscriber's own.
// The worker has to serve the queue events are published to, and the queues
// of the subscribers.
func (s *Subscribers) Attach(worker *queue.Worker) {
	worker.Handle(dispatchKind, s.dispatch)
	for _, subs := range s.byEvent {
		for _, sub := range subs {
			worker.Handle(sub.name, sub.handler, sub.handle...)
		}
	}
}

// dispatch enqueues one job per subscriber of the event. A key per event and
// subscriber makes it safe to run again: a subscriber that already has its
// job is skipped, so a dispatch that failed halfway only adds what is
// missing.
func (s *Subscribers) dispatch(ctx context.Context, job *queue.Job) error {
	var env envelope
	if err := json.Unmarshal(job.Payload, &env); err != nil {
		return queue.Permanent(fmt.Errorf("decode event: %w", err))
	}

	for _, sub := range s.byEvent[env.Name] {
		opts := append([]queue.EnqueueOption{queue.Unique("event:" + env.ID + ":" + sub.name)}, sub.enqueue...)
		if _, _, err := s.jobs.Enqueue(ctx, sub.name, env, opts...); err != nil {
			return fmt.Errorf("deliver %s to %s: %w", env.Name, sub.name, err)
		}
	}
	return nil
}
