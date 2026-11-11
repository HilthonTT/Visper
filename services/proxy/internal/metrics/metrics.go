package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics defines our Prometheus metrics
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

// ResponseWriter wraps http.ResponseWriter to capture the status code.
// It implements Unwrap so http.ResponseController (used by
// httputil.ReverseProxy for flushing and WebSocket hijacking) still reaches
// the underlying writer.
type ResponseWriter struct {
	http.ResponseWriter
	StatusCode  int
	wroteHeader bool
}

// NewResponseWriter wraps w, defaulting the status code to 200.
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{ResponseWriter: w, StatusCode: http.StatusOK}
}

// WriteHeader intercepts the status code
func (w *ResponseWriter) WriteHeader(statusCode int) {
	if !w.wroteHeader {
		w.StatusCode = statusCode
		// 1xx informational responses (other than 101) may be followed by
		// the final status.
		w.wroteHeader = statusCode >= 200 || statusCode == http.StatusSwitchingProtocols
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

// WroteHeader reports whether a final status code was written through w.
func (w *ResponseWriter) WroteHeader() bool {
	return w.wroteHeader
}

// Write records an implicit 200 status.
func (w *ResponseWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

// Unwrap returns the underlying http.ResponseWriter.
func (w *ResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
