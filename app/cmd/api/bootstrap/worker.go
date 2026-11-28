package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
	"uuid"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/auth"
	"github.com/imlargo/medusa/httpserver"
	"github.com/imlargo/medusa/lifecycle"
	"github.com/imlargo/medusa/metrics"
	"github.com/imlargo/medusa/postgres/events"
	"github.com/imlargo/medusa/postgres/queue"
	"github.com/imlargo/medusa/tracing"

	"github.com/imlargo/medusa/app/internal/account"
	"github.com/imlargo/medusa/app/internal/config"
	"github.com/imlargo/medusa/app/internal/store"
)

// workerConcurrency is how many jobs of the default queue one worker process
// runs at once.
const workerConcurrency = 10

// NewWorker builds the process that runs queued jobs (cmd/worker): the same
// dependencies as the API, with the queue's worker where the HTTP API is.
// Any number of them can run; a job is claimed by one.
//
// It also listens on WORKER_PORT for /metrics and /health, which is all the
// HTTP a worker serves.
func NewWorker(ctx context.Context, name string) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	container, err := NewContainer(ctx, cfg, MinimalOptions())
	if err != nil {
		return nil, fmt.Errorf("initialize %s: %w", name, err)
	}
	if err := container.StartTracing(ctx, name); err != nil {
		return nil, errors.Join(fmt.Errorf("initialize %s: %w", name, err), container.Close())
	}

	jobs := store.NewJobs(container.DB)

	registry := prometheus.NewRegistry()
	jobMetrics, err := metrics.NewJobs(registry, queueStats(jobs))
	if err != nil {
		return nil, fmt.Errorf("initialize %s: %w", name, err)
	}

	worker := jobs.Worker(container.Logger,
		queue.Queues{queue.DefaultQueue: workerConcurrency},
		queue.WithStopTimeout(cfg.Runtime.ShutdownTimeout),
		// A job runs as the request that enqueued it: its logs carry that
		// request's ID, and its handler sees that caller.
		queue.WithJobContext(func(ctx context.Context, job *queue.Job) context.Context {
			correlationID := job.CorrelationID
			if correlationID == "" {
				// A scheduled job has no request behind it: it starts an ID
				// of its own, which its logs, the jobs it enqueues and the
				// calls it makes then share.
				correlationID = uuid.New().String()
			}
			ctx = medusa.WithRequestID(ctx, correlationID)
			if caller, ok := auth.PrincipalFromMetadata(job.Metadata); ok {
				ctx = auth.WithPrincipal(ctx, caller)
			}
			// The job's span continues the trace of whatever enqueued it.
			return tracing.StartJob(ctx, job.Kind, job.Queue, job.Metadata)
		}),
		queue.WithOnFinish(func(ctx context.Context, job *queue.Job, took time.Duration, err error) {
			jobMetrics.ObserveRun(job.Queue, job.Kind, job.Waited, took, err)
			tracing.EndJob(ctx, err)
		}),
	)
	registerJobs(worker, jobs, container)

	admin := httpserver.New(workerAdmin(registry), container.Logger,
		httpserver.WithHost(cfg.Server.Host),
		httpserver.WithPort(cfg.Server.WorkerPort),
	)

	return &App{
		App: lifecycle.New(
			lifecycle.WithName(name),
			lifecycle.WithServer(worker, admin),
			lifecycle.WithLogger(container.Logger),
		),
		Container: container,
	}, nil
}

// registerJobs installs every subscriber and the handler of every kind of
// job, the way registerRoutes installs every route.
func registerJobs(worker *queue.Worker, jobs *queue.Client, c *Container) {
	subscribers := events.NewSubscribers(jobs)
	events.Subscribe(subscribers, account.EventRegistered, account.SubscriberWelcome, account.Welcome(c.Logger))
	subscribers.Attach(worker)

	// Daily, at a quiet hour. With CatchUp a deploy at that minute delays the
	// run instead of skipping it; Coalesce keeps a backlog from repeating it.
	queue.Register(worker, account.KindPurgeSessions, account.PurgeSessions(c.Services.Account), queue.Coalesce())
	worker.Schedule(account.KindPurgeSessions, "17 3 * * *", nil, queue.CatchUp(6*time.Hour))
}

// queueStats adapts the queue's state to what the metrics package exports.
func queueStats(jobs *queue.Client) func(ctx context.Context) ([]metrics.QueueStats, error) {
	return func(ctx context.Context) ([]metrics.QueueStats, error) {
		stats, err := jobs.Stats(ctx)
		if err != nil {
			return nil, err
		}

		out := make([]metrics.QueueStats, len(stats))
		for i, s := range stats {
			out[i] = metrics.QueueStats{
				Queue: s.Queue, Ready: s.Ready, Scheduled: s.Scheduled,
				Running: s.Running, Failed: s.Failed, OldestReady: s.OldestReady,
			}
		}
		return out, nil
	}
}

// workerAdmin serves what an orchestrator and a metrics scraper ask a worker.
func workerAdmin(registry *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}
