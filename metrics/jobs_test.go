package metrics_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/imlargo/medusa/metrics"
)

func TestJobsExportTheQueuesStateAtScrapeTime(t *testing.T) {
	registry := prometheus.NewRegistry()
	_, err := metrics.NewJobs(registry, func(context.Context) ([]metrics.QueueStats, error) {
		return []metrics.QueueStats{{Queue: "mail", Ready: 3, Running: 1, Failed: 2, OldestReady: 90 * time.Second}}, nil
	})
	if err != nil {
		t.Fatalf("NewJobs() = %v", err)
	}

	want := `
# HELP jobs_oldest_ready_seconds How long the oldest ready job has waited
# TYPE jobs_oldest_ready_seconds gauge
jobs_oldest_ready_seconds{queue="mail"} 90
# HELP jobs_ready Jobs due and waiting for a worker
# TYPE jobs_ready gauge
jobs_ready{queue="mail"} 3
# HELP jobs_failed Jobs that ran out of attempts
# TYPE jobs_failed gauge
jobs_failed{queue="mail"} 2
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(want), "jobs_ready", "jobs_failed", "jobs_oldest_ready_seconds"); err != nil {
		t.Error(err)
	}
}

func TestJobsRecordEachRun(t *testing.T) {
	registry := prometheus.NewRegistry()
	jobs, err := metrics.NewJobs(registry, nil)
	if err != nil {
		t.Fatalf("NewJobs() = %v", err)
	}

	jobs.ObserveRun("mail", "send", 2*time.Second, time.Second, nil)
	jobs.ObserveRun("mail", "send", time.Second, time.Second, errors.New("smtp down"))

	want := `
# HELP jobs_runs_total Job runs, by how they ended
# TYPE jobs_runs_total counter
jobs_runs_total{kind="send",outcome="failed",queue="mail"} 1
jobs_runs_total{kind="send",outcome="succeeded",queue="mail"} 1
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(want), "jobs_runs_total"); err != nil {
		t.Error(err)
	}
	if got := testutil.CollectAndCount(registry, "jobs_wait_seconds", "jobs_run_duration_seconds"); got != 2 {
		t.Errorf("histograms = %d, want the wait and the duration of the kind", got)
	}
}

// A failed read must show up as a failed scrape, not as an empty queue.
func TestJobsReportAFailedStatsRead(t *testing.T) {
	registry := prometheus.NewRegistry()
	if _, err := metrics.NewJobs(registry, func(context.Context) ([]metrics.QueueStats, error) {
		return nil, errors.New("database is down")
	}); err != nil {
		t.Fatalf("NewJobs() = %v", err)
	}

	if _, err := registry.Gather(); err == nil {
		t.Error("Gather() = nil, want the stats error")
	}
}
