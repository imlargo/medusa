// Package metrics records HTTP request metrics in Prometheus.
package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HTTP records request counts and durations, labeled by method, route
// template and status. It satisfies middleware.MetricsRecorder.
type HTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTP registers the HTTP collectors with registry.
//
// Pass nil to use Prometheus' default registry, which is what a single-process
// application wants. Pass prometheus.NewRegistry() when more than one instance
// may exist in the same process, most notably in tests: registering the same
// collector twice in one registry is an error.
func NewHTTP(registry prometheus.Registerer) (*HTTP, error) {
	if registry == nil {
		registry = prometheus.DefaultRegisterer
	}

	requests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	for _, collector := range []prometheus.Collector{requests, duration} {
		if err := registry.Register(collector); err != nil {
			return nil, fmt.Errorf("register http metrics: %w", err)
		}
	}

	return &HTTP{requests: requests, duration: duration}, nil
}

// RecordHTTPRequest counts one request.
func (m *HTTP) RecordHTTPRequest(method, path, status string) {
	m.requests.WithLabelValues(method, path, status).Inc()
}

// RecordHTTPDuration observes how long one request took.
func (m *HTTP) RecordHTTPDuration(method, path, status string, duration time.Duration) {
	m.duration.WithLabelValues(method, path, status).Observe(duration.Seconds())
}
