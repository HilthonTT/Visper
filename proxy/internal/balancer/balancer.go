package balancer

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hilthontt/visper/proxy/internal/config"
	"github.com/hilthontt/visper/proxy/internal/metrics"
)

// Strategy represents a load balancing strategy
type Strategy int

const (
	RoundRobin Strategy = iota
	LeastConnections
	IPHash
	Random
	WeightedRoundRobin
)

// ParseStrategy converts a strategy string to a Strategy enum
func ParseStrategy(s string) (Strategy, error) {
	switch s {
	case "round_robin":
		return RoundRobin, nil
	case "least_connections":
		return LeastConnections, nil
	case "ip_hash":
		return IPHash, nil
	case "random":
		return Random, nil
	case "weighted_round_robin":
		return WeightedRoundRobin, nil
	default:
		return 0, fmt.Errorf("unknown strategy: %s", s)
	}
}

// Backend represents a server to forward requests to
type Backend struct {
	URL          *url.URL
	ReverseProxy *httputil.ReverseProxy

	mux         sync.RWMutex
	alive       bool
	failCount   int
	connections int
	weight      int // guarded by LoadBalancer.mux
}

// SetAlive updates the alive status of the backend
func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	b.alive = alive
	if alive {
		b.failCount = 0
	}
	b.mux.Unlock()
}

// IsAlive returns true if the backend is alive
func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.alive
}

// ResetFailCount resets the failure count of the backend
func (b *Backend) ResetFailCount() {
	b.mux.Lock()
	b.failCount = 0
	b.mux.Unlock()
}

// IncreaseFailCount increases the failure count of the backend
func (b *Backend) IncreaseFailCount() int {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.failCount++
	return b.failCount
}

// Connections returns the number of in-flight requests to the backend.
func (b *Backend) Connections() int {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.connections
}

func (b *Backend) addConnections(delta int) {
	b.mux.Lock()
	b.connections += delta
	b.mux.Unlock()
}

// Options configures a LoadBalancer.
type Options struct {
	Strategy            Strategy
	HealthCheckInterval time.Duration
	HealthCheckPath     string
	MaxFailCount        int
	Metrics             *metrics.Metrics
	// ClientIP returns the real client IP (used by IPHash and X-Real-IP).
	ClientIP func(*http.Request) string
	// TrustForwarded reports whether the request's X-Forwarded-For header
	// came from a trusted proxy and may be extended rather than replaced.
	TrustForwarded func(*http.Request) bool
}

// LoadBalancer represents the load balancer
type LoadBalancer struct {
	mux                 sync.Mutex
	backends            []*Backend
	current             int
	healthCheckInterval time.Duration
	healthCheckPath     string
	maxFailCount        int
	strategy            Strategy

	metrics        *metrics.Metrics
	clientIP       func(*http.Request) string
	trustForwarded func(*http.Request) bool
	transport      http.RoundTripper
	healthClient   *http.Client
	intervalReset  chan struct{}
}

// New creates a new load balancer. Call RunHealthChecks to start health checks.
func New(backends []config.BackendConfig, opts Options) (*LoadBalancer, error) {
	if opts.HealthCheckInterval <= 0 {
		opts.HealthCheckInterval = config.DefaultHealthCheckInterval
	}
	if opts.HealthCheckPath == "" {
		opts.HealthCheckPath = config.DefaultHealthCheckPath
	}
	if opts.MaxFailCount <= 0 {
		opts.MaxFailCount = config.DefaultMaxFailCount
	}
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
		healthCheckInterval: opts.HealthCheckInterval,
		healthCheckPath:     opts.HealthCheckPath,
		maxFailCount:        opts.MaxFailCount,
		strategy:            opts.Strategy,
		metrics:             opts.Metrics,
		clientIP:            opts.ClientIP,
		trustForwarded:      opts.TrustForwarded,
		transport:           newTransport(),
		healthClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   2 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   2 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
			Timeout: 3 * time.Second,
		},
		intervalReset: make(chan struct{}, 1),
	}

	built, err := lb.buildBackends(backends, nil)
	if err != nil {
		return nil, err
	}
	lb.backends = built
	return lb, nil
}

// buildBackends creates backends for cfgs, reusing entries from existing that
// have the same URL so their health state and connections are preserved.
func (lb *LoadBalancer) buildBackends(cfgs []config.BackendConfig, existing []*Backend) ([]*Backend, error) {
	if len(cfgs) == 0 {
		return nil, errors.New("at least one backend is required")
	}
	byURL := make(map[string]*Backend, len(existing))
	for _, b := range existing {
		byURL[b.URL.String()] = b
	}

	backends := make([]*Backend, len(cfgs))
	for i, c := range cfgs {
		weight := c.Weight
		if weight <= 0 {
			weight = 1 // Default weight
		}
		u, err := url.Parse(c.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid backend url %q: %w", c.URL, err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("invalid backend url %q: must be absolute http(s) URL", c.URL)
		}
		if b, ok := byURL[u.String()]; ok {
			b.weight = weight
			backends[i] = b
			continue
		}
		b := &Backend{URL: u, alive: true, weight: weight}
		b.ReverseProxy = lb.newReverseProxy(b)
		backends[i] = b
		lb.metrics.BackendUpGauge.WithLabelValues(u.Host).Set(1)
	}
	return backends, nil
}

// Reload replaces the load balancer configuration, keeping backends whose URL
// is unchanged.
func (lb *LoadBalancer) Reload(cfg *config.Config) error {
	strategy, err := ParseStrategy(cfg.Strategy)
	if err != nil {
		return fmt.Errorf("invalid strategy: %w", err)
	}

	lb.mux.Lock()
	backends, err := lb.buildBackends(cfg.Backends, lb.backends)
	if err != nil {
		lb.mux.Unlock()
		return err
	}
	removed := make(map[*Backend]struct{}, len(lb.backends))
	for _, b := range lb.backends {
		removed[b] = struct{}{}
	}
	for _, b := range backends {
		delete(removed, b)
	}
	lb.backends = backends
	lb.current = 0
	lb.strategy = strategy
	if cfg.MaxFailCount > 0 {
		lb.maxFailCount = cfg.MaxFailCount
	}
	if cfg.HealthCheckPath != "" {
		lb.healthCheckPath = cfg.HealthCheckPath
	}
	if d := time.Duration(cfg.HealthCheckInterval); d > 0 && d != lb.healthCheckInterval {
		lb.healthCheckInterval = d
		select {
		case lb.intervalReset <- struct{}{}:
		default:
		}
	}
	lb.mux.Unlock()

	for b := range removed {
		lb.metrics.BackendUpGauge.DeleteLabelValues(b.URL.Host)
	}

	log.Printf("Configuration reloaded with %d backends and strategy: %s", len(backends), cfg.Strategy)
	return nil
}

// Backends returns a snapshot of the current backends.
func (lb *LoadBalancer) Backends() []*Backend {
	lb.mux.Lock()
	defer lb.mux.Unlock()
	return append([]*Backend(nil), lb.backends...)
}

func (lb *LoadBalancer) chooseBackendByStrategy(r *http.Request) *Backend {
	lb.mux.Lock()
	defer lb.mux.Unlock()

	switch lb.strategy {
	case LeastConnections:
		return lb.leastConnectionsSelect()
	case IPHash:
		return lb.ipHashSelect(r)
	case Random:
		return lb.randomSelect()
	case WeightedRoundRobin:
		return lb.weightedSelect()
	default:
		return lb.roundRobinSelect()
	}
}

// roundRobinSelect selects a backend using round-robin algorithm.
// Must be called with lb.mux held.
func (lb *LoadBalancer) roundRobinSelect() *Backend {
	n := len(lb.backends)
	for i := range n {
		idx := (lb.current + i) % n
		if lb.backends[idx].IsAlive() {
			lb.current = (idx + 1) % n
			return lb.backends[idx]
		}
	}
	return nil
}

// leastConnectionsSelect selects the backend with the least active connections
func (lb *LoadBalancer) leastConnectionsSelect() *Backend {
	var leastConnBackend *Backend
	leastConn := -1

	for _, b := range lb.backends {
		if !b.IsAlive() {
			continue
		}
		if connCount := b.Connections(); leastConn == -1 || connCount < leastConn {
			leastConn = connCount
			leastConnBackend = b
		}
	}

	return leastConnBackend
}

// ipHashSelect selects a backend based on client IP hash
func (lb *LoadBalancer) ipHashSelect(r *http.Request) *Backend {
	hash := fnv.New32a()
	hash.Write([]byte(lb.clientIP(r)))
	n := uint32(len(lb.backends))
	idx := hash.Sum32() % n

	// Find the selected backend or next available
	for i := range n {
		if b := lb.backends[(idx+i)%n]; b.IsAlive() {
			return b
		}
	}
	return nil
}

// randomSelect randomly selects an alive backend
func (lb *LoadBalancer) randomSelect() *Backend {
	var alive []*Backend
	for _, b := range lb.backends {
		if b.IsAlive() {
			alive = append(alive, b)
		}
	}
	if len(alive) == 0 {
		return nil
	}
	return alive[rand.IntN(len(alive))]
}

// weightedSelect picks an alive backend with probability proportional to its
// weight (weighted random).
func (lb *LoadBalancer) weightedSelect() *Backend {
	totalWeight := 0
	for _, b := range lb.backends {
		if b.IsAlive() {
			totalWeight += b.weight
		}
	}
	if totalWeight == 0 {
		return nil
	}

	target := rand.IntN(totalWeight)
	for _, b := range lb.backends {
		if !b.IsAlive() {
			continue
		}
		target -= b.weight
		if target < 0 {
			return b
		}
	}
	return nil
}

// remoteIP returns the host part of r.RemoteAddr.
func remoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// RunHealthChecks performs health checks on all backends until ctx is done.
func (lb *LoadBalancer) RunHealthChecks(ctx context.Context) {
	lb.mux.Lock()
	interval := lb.healthCheckInterval
	lb.mux.Unlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lb.checkAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-lb.intervalReset:
			lb.mux.Lock()
			ticker.Reset(lb.healthCheckInterval)
			lb.mux.Unlock()
		case <-ticker.C:
			lb.checkAll(ctx)
		}
	}
}

func (lb *LoadBalancer) checkAll(ctx context.Context) {
	lb.mux.Lock()
	backends := append([]*Backend(nil), lb.backends...)
	path := lb.healthCheckPath
	lb.mux.Unlock()

	var wg sync.WaitGroup
	for _, backend := range backends {
		wg.Go(func() {
			alive := lb.isBackendAlive(ctx, backend.URL, path)
			backend.SetAlive(alive)

			label := backend.URL.Host
			if alive {
				lb.metrics.BackendUpGauge.WithLabelValues(label).Set(1)
			} else {
				lb.metrics.BackendUpGauge.WithLabelValues(label).Set(0)
				lb.metrics.BackendErrors.WithLabelValues(label, "health_check").Inc()
			}
		})
	}
	wg.Wait()
}

// isBackendAlive checks if a backend is alive by making an HTTP request
func (lb *LoadBalancer) isBackendAlive(ctx context.Context, u *url.URL, path string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.JoinPath(path).String(), nil)
	if err != nil {
		return false
	}
	resp, err := lb.healthClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) // allow conn reuse
	return resp.StatusCode < http.StatusInternalServerError       // Consider any non-5xx response as alive
}

// newTransport creates a transport with optimized connection pooling.
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100, // Important for load balancers
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// HTTP/1.1 only so Upgrade (WebSocket) requests can be proxied.
		ForceAttemptHTTP2: false,
	}
}

// newReverseProxy builds the reverse proxy for b. Rewrite (unlike Director)
// strips client-supplied Forwarded/X-Forwarded-* headers before they reach
// the backend, and ReverseProxy itself removes hop-by-hop headers while
// passing Connection: Upgrade through for WebSockets.
func (lb *LoadBalancer) newReverseProxy(b *Backend) *httputil.ReverseProxy {
	target := b.URL
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target) // also sets the outbound Host to the target host
			pr.SetXForwarded()
			if prior := pr.In.Header.Values("X-Forwarded-For"); len(prior) > 0 && lb.trustForwarded(pr.In) {
				pr.Out.Header.Set("X-Forwarded-For",
					strings.Join(prior, ", ")+", "+pr.Out.Header.Get("X-Forwarded-For"))
			}
			pr.Out.Header.Set("X-Real-IP", lb.clientIP(pr.In))
		},
		Transport: lb.transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			label := b.URL.Host
			if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
				// The client went away; not the backend's fault.
				w.WriteHeader(499)
				return
			}
			log.Printf("proxy error for %s: %v", label, err)
			lb.metrics.BackendErrors.WithLabelValues(label, "proxy_error").Inc()

			lb.mux.Lock()
			maxFail := lb.maxFailCount
			lb.mux.Unlock()
			if b.IncreaseFailCount() >= maxFail {
				b.SetAlive(false)
				lb.metrics.BackendUpGauge.WithLabelValues(label).Set(0)
				log.Printf("backend %s marked down after %d failures", label, maxFail)
			}

			status := http.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			http.Error(w, http.StatusText(status), status)
		},
	}
}

// isWebSocketRequest reports whether r asks for a WebSocket upgrade.
func isWebSocketRequest(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, v := range r.Header.Values("Connection") {
		for _, tok := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(tok), "upgrade") {
				return true
			}
		}
	}
	return false
}

// ServeHTTP implements the http.Handler interface
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := lb.chooseBackendByStrategy(r)
	if backend == nil {
		lb.metrics.RequestCount.WithLabelValues("none", "503", r.Method).Inc()
		w.Header().Set("Retry-After", "5")
		http.Error(w, "No available backends", http.StatusServiceUnavailable)
		return
	}

	start := time.Now()
	backendLabel := backend.URL.Host

	backend.addConnections(1)
	lb.metrics.ActiveConnections.WithLabelValues(backendLabel).Inc()
	defer func() {
		// Runs for WebSockets too, once the upgraded connection closes.
		backend.addConnections(-1)
		lb.metrics.ActiveConnections.WithLabelValues(backendLabel).Dec()
	}()

	websocket := isWebSocketRequest(r)
	if websocket {
		log.Printf("WebSocket upgrade request to: %s", backendLabel)
		// Long-lived connection: clear the server's read/write deadlines,
		// which would otherwise survive the hijack and kill the socket.
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Time{})
		_ = rc.SetWriteDeadline(time.Time{})
	}

	wrappedWriter := metrics.NewResponseWriter(w)
	backend.ReverseProxy.ServeHTTP(wrappedWriter, r)

	statusCode := wrappedWriter.StatusCode
	if websocket && !wrappedWriter.WroteHeader() {
		// ReverseProxy writes the 101 directly to the hijacked connection.
		statusCode = http.StatusSwitchingProtocols
	}

	lb.metrics.RequestCount.WithLabelValues(backendLabel, strconv.Itoa(statusCode), r.Method).Inc()
	if !websocket {
		duration := time.Since(start).Seconds()
		lb.metrics.RequestDuration.WithLabelValues(backendLabel).Observe(duration)
		lb.metrics.BackendResponseTime.WithLabelValues(backendLabel).Observe(duration)
	}

	// Reset fail count on successful request
	if statusCode < http.StatusInternalServerError {
		backend.ResetFailCount()
	} else {
		lb.metrics.BackendErrors.WithLabelValues(backendLabel, "response_error").Inc()
	}
}
