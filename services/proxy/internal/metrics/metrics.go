// Package metrics defines the proxy's Prometheus metrics.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds the load balancer's Prometheus collectors.
type Metrics struct {
	RequestCount        *prometheus.CounterVec
	RequestDuration     *prometheus.HistogramVec
	BackendUpGauge      *prometheus.GaugeVec
	ActiveConnections   *prometheus.GaugeVec
	BackendResponseTime *prometheus.HistogramVec
	BackendErrors       *prometheus.CounterVec
}

// New creates a new metrics collection and registers it with reg.
// A nil reg skips registration (useful in tests).
func New(namespace string, reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		RequestCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "request_count_total",
				Help:      "Total number of requests handled by the load balancer",
			},
			[]string{"backend", "status", "method"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_duration_seconds",
				Help:      "Request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"backend"},
		),
		BackendUpGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "backend_up",
				Help:      "Whether the backend is up (1) or down (0)",
			},
			[]string{"backend"},
		),
		ActiveConnections: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "backend_connections_active",
				Help:      "Number of active connections per backend",
			},
			[]string{"backend"},
		),
		BackendResponseTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "backend_response_seconds",
				Help:      "Backend response time in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"backend"},
		),
		BackendErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "backend_errors_total",
				Help:      "Total number of backend errors",
			},
			[]string{"backend", "error_type"},
		),
	}

	if reg != nil {
		reg.MustRegister(
			m.RequestCount,
			m.RequestDuration,
			m.BackendUpGauge,
			m.ActiveConnections,
			m.BackendResponseTime,
			m.BackendErrors,
		)
	}

	return m
}
