package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/imlargo/medusa"
	"github.com/imlargo/medusa/httpserver"
	"github.com/imlargo/medusa/lifecycle"
	"github.com/imlargo/medusa/metrics"
	"github.com/imlargo/medusa/postgres/queue"

	"github.com/imlargo/medusa/app/internal/account"
	"github.com/imlargo/medusa/app/internal/config"
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

	jobs := queue.New(container.DB)

	registry := prometheus.NewRegistry()
	jobMetrics, err := metrics.NewJobs(registry, queueStats(jobs))
	if err != nil {
		return nil, fmt.Errorf("initialize %s: %w", name, err)
	}

	worker := jobs.Worker(container.Logger,
		queue.Queues{queue.DefaultQueue: workerConcurrency},
		queue.WithStopTimeout(cfg.Runtime.ShutdownTimeout),
		// The job's logs carry the ID of the request that enqueued it.
		queue.WithJobContext(func(ctx context.Context, job *queue.Job) context.Context {
			return medusa.WithRequestID(ctx, job.CorrelationID)
		}),
		queue.WithOnFinish(func(_ context.Context, job *queue.Job, took time.Duration, err error) {
			jobMetrics.ObserveRun(job.Queue, job.Kind, job.Waited, took, err)
		}),
	)
	registerJobs(worker, container)

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

// registerJobs installs the handler of every kind of job, the way
// registerRoutes installs every route.
func registerJobs(worker *queue.Worker, c *Container) {
	queue.Register(worker, account.KindWelcome, account.Welcome(c.Logger))
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
