// Package balancer implements the HTTP(S)/WebSocket load balancer: backend
// selection strategies, active health checks, passive failure detection and
// hot reloading of the backend set.
package balancer

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/hilthontt/visper/services/proxy/internal/config"
	"github.com/hilthontt/visper/services/proxy/internal/metrics"
)

// Options holds the load balancer's collaborators. All fields are optional.
type Options struct {
	Metrics *metrics.Metrics
	// ClientIP returns the real client IP (used by IPHash and X-Real-IP).
	// Defaults to the host part of RemoteAddr.
	ClientIP func(*http.Request) string
	// TrustForwarded reports whether the request's X-Forwarded-* headers
	// came from a trusted proxy and may be kept rather than replaced.
	TrustForwarded func(*http.Request) bool
}

// LoadBalancer is an http.Handler that proxies each request to a backend.
type LoadBalancer struct {
	metrics        *metrics.Metrics
	clientIP       func(*http.Request) string
	trustForwarded func(*http.Request) bool
	transport      http.RoundTripper
	healthClient   *http.Client
	// reloaded wakes the health-check loop after a reload so new backends
	// and a new interval take effect immediately.
	reloaded chan struct{}

	mu                  sync.Mutex
	backends            []*Backend
	next                int // round-robin cursor
	strategy            Strategy
	healthCheckInterval time.Duration
	healthCheckPath     string
	maxFailCount        int
}

// New creates a load balancer for cfg. Call RunHealthChecks to start active
// health checks.
func New(cfg *config.Config, opts Options) (*LoadBalancer, error) {
	if opts.Metrics == nil {
		opts.Metrics = metrics.New("loadBalancer", nil)
	}
	if opts.ClientIP == nil {
		opts.ClientIP = remoteIP
	}
	if opts.TrustForwarded == nil {
		opts.TrustForwarded = func(*http.Request) bool { return false }
	}

	lb := &LoadBalancer{
		metrics:        opts.Metrics,
		clientIP:       opts.ClientIP,
		trustForwarded: opts.TrustForwarded,
		transport:      newProxyTransport(),
		healthClient:   newHealthClient(),
		reloaded:       make(chan struct{}, 1),
	}
	if err := lb.apply(cfg); err != nil {
		return nil, err
	}
	return lb, nil
}

// Backends returns a snapshot of the current backends.
func (lb *LoadBalancer) Backends() []*Backend {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return append([]*Backend(nil), lb.backends...)
}

// ServeHTTP proxies r to a backend chosen by the configured strategy.
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := lb.pick(r)
	if backend == nil {
		lb.metrics.RequestCount.WithLabelValues("none", "503", r.Method).Inc()
		w.Header().Set("Retry-After", "5")
		// Nothing was forwarded, so even a POST is safe for the client to resend.
		w.Header().Set("X-Should-Retry", "true")
		http.Error(w, "No available backends", http.StatusServiceUnavailable)
		return
	}

	label := backend.label()
	start := time.Now()

	backend.addConnections(1)
	lb.metrics.ActiveConnections.WithLabelValues(label).Inc()
	defer func() {
		// Runs for WebSockets too, once the upgraded connection closes.
		backend.addConnections(-1)
		lb.metrics.ActiveConnections.WithLabelValues(label).Dec()
	}()

	websocket := isWebSocketRequest(r)
	if websocket {
		// Long-lived connection: clear the server's read/write deadlines,
		// which would otherwise survive the hijack and kill the socket.
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Time{})
		_ = rc.SetWriteDeadline(time.Time{})
	}

	rec := newStatusRecorder(w)
	backend.proxy.ServeHTTP(rec, r)

	status := rec.status
	if websocket && !rec.wroteHeader {
		// ReverseProxy writes the 101 directly to the hijacked connection.
		status = http.StatusSwitchingProtocols
	}

	lb.metrics.RequestCount.WithLabelValues(label, strconv.Itoa(status), r.Method).Inc()
	if !websocket {
		d := time.Since(start).Seconds()
		lb.metrics.RequestDuration.WithLabelValues(label).Observe(d)
		lb.metrics.BackendResponseTime.WithLabelValues(label).Observe(d)
	}

	switch {
	case rec.proxyFailed:
		// Already accounted for by the proxy error handler.
	case status >= http.StatusInternalServerError:
		lb.metrics.BackendErrors.WithLabelValues(label, "response_error").Inc()
	default:
		backend.resetFailures()
	}
}

// remoteIP returns the host part of r.RemoteAddr.
func remoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func logf(format string, args ...any) { log.Printf("balancer: "+format, args...) }
