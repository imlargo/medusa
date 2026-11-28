package bootstrap

import (
	"context"
	"fmt"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/lifecycle"
	"github.com/imlargo/medusa/postgres/queue"

	"github.com/imlargo/medusa/app/internal/account"
	"github.com/imlargo/medusa/app/internal/config"
)

// workerConcurrency is how many jobs of the default queue one worker process
// runs at once.
const workerConcurrency = 10

// NewWorker builds the process that runs queued jobs (cmd/worker): the same
// dependencies as the API, with the queue's worker where the HTTP server is.
// Any number of them can run; a job is claimed by one.
func NewWorker(ctx context.Context, name string) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	container, err := NewContainer(ctx, cfg, MinimalOptions())
	if err != nil {
		return nil, fmt.Errorf("initialize %s: %w", name, err)
	}

	worker := queue.New(container.DB).Worker(container.Logger,
		queue.Queues{queue.DefaultQueue: workerConcurrency},
		queue.WithStopTimeout(cfg.Runtime.ShutdownTimeout),
		// The job's logs carry the ID of the request that enqueued it.
		queue.WithJobContext(func(ctx context.Context, job *queue.Job) context.Context {
			return medusa.WithRequestID(ctx, job.CorrelationID)
		}),
	)
	registerJobs(worker, container)

	return &App{
		App: lifecycle.New(
			lifecycle.WithName(name),
			lifecycle.WithServer(worker),
			lifecycle.WithLogger(container.Logger),
		),
		Container: container,
	}, nil
}

// registerJobs installs the handler of every kind of job, the way
// registerRoutes installs every route.
func registerJobs(worker *queue.Worker, c *Container) {
	queue.Register(worker, account.KindWelcome, account.Welcome(c.Logger))
}
