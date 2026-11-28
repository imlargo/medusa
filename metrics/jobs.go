package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// statsTimeout bounds reading the queues' state during a scrape.
const statsTimeout = 5 * time.Second

// Job outcomes, the values of the outcome label.
const (
	OutcomeSucceeded = "succeeded"
	OutcomeFailed    = "failed"
)

// QueueStats is the state of one job queue when it is scraped.
type QueueStats struct {
	Queue string

	// Ready jobs are due and waiting for a worker.
	Ready int64

	// Scheduled jobs are waiting for their time.
	Scheduled int64

	Running int64

	// Failed jobs ran out of attempts.
	Failed int64

	// OldestReady is how long the oldest ready job has waited.
	OldestReady time.Duration
}

// Jobs records background job metrics: how long jobs wait and run, how they
// end, and the state of each queue. It knows no queue implementation: the
// worker reports each run with ObserveRun, and the state is read through the
// function given to NewJobs.
type Jobs struct {
	runs     *prometheus.CounterVec
	duration *prometheus.HistogramVec
	wait     *prometheus.HistogramVec
}

// NewJobs registers the job collectors with registry (nil for Prometheus'
// default one).
//
// stats is called on every scrape to report each queue's depth; pass nil in
// a process that should not report it. Depth belongs to the queue, not to a
// process, so when several processes export it, aggregate with max, not sum.
func NewJobs(registry prometheus.Registerer, stats func(ctx context.Context) ([]QueueStats, error)) (*Jobs, error) {
	if registry == nil {
		registry = prometheus.DefaultRegisterer
	}

	m := &Jobs{
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "jobs_runs_total",
			Help: "Job runs, by how they ended",
		}, []string{"queue", "kind", "outcome"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "jobs_run_duration_seconds",
			Help:    "How long a job's handler ran",
			Buckets: []float64{.05, .1, .5, 1, 5, 10, 30, 60, 300, 600, 1800},
		}, []string{"queue", "kind"}),
		wait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "jobs_wait_seconds",
			Help:    "How long a job was due before a worker claimed it",
			Buckets: []float64{.1, .5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1800},
		}, []string{"queue", "kind"}),
	}

	collectors := []prometheus.Collector{m.runs, m.duration, m.wait}
	if stats != nil {
		collectors = append(collectors, newQueueCollector(stats))
	}
	for _, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			return nil, fmt.Errorf("register job metrics: %w", err)
		}
	}
	return m, nil
}

// ObserveRun records one run of a job: how long it waited for a worker, how
// long its handler took, and whether it returned an error.
func (m *Jobs) ObserveRun(queue, kind string, waited, took time.Duration, err error) {
	outcome := OutcomeSucceeded
	if err != nil {
		outcome = OutcomeFailed
	}

	m.runs.WithLabelValues(queue, kind, outcome).Inc()
	m.duration.WithLabelValues(queue, kind).Observe(took.Seconds())
	m.wait.WithLabelValues(queue, kind).Observe(waited.Seconds())
}

// queueCollector reads the queues' state at scrape time, so the numbers are
// as fresh as the scrape and nothing polls in between.
type queueCollector struct {
	stats func(ctx context.Context) ([]QueueStats, error)

	ready, scheduled, running, failed, oldest *prometheus.Desc
}

func newQueueCollector(stats func(ctx context.Context) ([]QueueStats, error)) *queueCollector {
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, []string{"queue"}, nil)
	}
	return &queueCollector{
		stats:     stats,
		ready:     desc("jobs_ready", "Jobs due and waiting for a worker"),
		scheduled: desc("jobs_scheduled", "Jobs waiting for their time"),
		running:   desc("jobs_running", "Jobs a worker is running"),
		failed:    desc("jobs_failed", "Jobs that ran out of attempts"),
		oldest:    desc("jobs_oldest_ready_seconds", "How long the oldest ready job has waited"),
	}
}

func (c *queueCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{c.ready, c.scheduled, c.running, c.failed, c.oldest} {
		ch <- desc
	}
}

func (c *queueCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), statsTimeout)
	defer cancel()

	stats, err := c.stats(ctx)
	if err != nil {
		// Reported as a failed scrape of this metric, not as zero jobs.
		ch <- prometheus.NewInvalidMetric(c.ready, err)
		return
	}

	for _, s := range stats {
		gauge := func(desc *prometheus.Desc, value float64) {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, s.Queue)
		}
		gauge(c.ready, float64(s.Ready))
		gauge(c.scheduled, float64(s.Scheduled))
		gauge(c.running, float64(s.Running))
		gauge(c.failed, float64(s.Failed))
		gauge(c.oldest, s.OldestReady.Seconds())
	}
}
