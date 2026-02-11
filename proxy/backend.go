package main

import (
	"fmt"
	"hash/fnv"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

// Backend represents a server to forward requests to
type Backend struct {
	URL          *url.URL
	Alive        bool
	ReverseProxy *httputil.ReverseProxy
	mux          sync.RWMutex
	failCount    int
	weight       int
	connections  int
}

// SetAlive updates the alive status of the backend
func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	b.Alive = alive
	if alive {
		b.failCount = 0
	}
	b.mux.Unlock()
}

// IsAlive returns true if the backend is alive
func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	alive := b.Alive
	b.mux.RUnlock()
	return alive
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
	b.failCount++
	count := b.failCount
	b.mux.Unlock()
	return count
}

// LoadBalancer represents the load balancer
type LoadBalancer struct {
	backends            []*Backend
	current             int
	atomicCurrent       uint32
	mux                 sync.Mutex
	healthCheckInterval time.Duration
	maxFailCount        int
	strategy            Strategy
	metrics             *Metrics
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(
	backendURLs []string,
	weights []int,
	healthCheckInterval time.Duration,
	maxFailCount int,
	strategy Strategy,
) *LoadBalancer {
	if len(weights) == 0 {
		weights = make([]int, len(backendURLs))
		for i := range weights {
			weights[i] = 1 // Default weight
		}
	}

	backends := make([]*Backend, len(backendURLs))

	for i, rawURL := range backendURLs {
		url, err := url.Parse(rawURL)
		if err != nil {
			log.Fatal(err)
		}

		backends[i] = &Backend{
			URL:          url,
			Alive:        true,
			ReverseProxy: createOptimizedReverseProxy(url),
			weight:       weights[i],
		}

		// Configure error handler
		// (implementation same as before)
	}

	lb := &LoadBalancer{
		backends:            backends,
		healthCheckInterval: healthCheckInterval,
		maxFailCount:        maxFailCount,
		strategy:            strategy,
	}

	// Start health checks
	go lb.healthCheck()

	return lb
}

func (lb *LoadBalancer) chooseBackendByStrategy(r *http.Request) *Backend {
	lb.mux.Lock()
	defer lb.mux.Unlock()

	// Count alive backends
	aliveCount := 0
	for _, b := range lb.backends {
		if b.IsAlive() {
			aliveCount++
		}
	}

	if aliveCount == 0 {
		return nil
	}

	switch lb.strategy {
	case RoundRobin:
		return lb.fastRoundRobinSelect()
	case LeastConnections:
		return lb.leastConnectionsSelect()
	case IPHash:
		return lb.ipHashSelect(r)
	case Random:
		return lb.randomSelect()
	case WeightedRoundRobin:
		return lb.weightedRoundRobinSelect()
	default:
		return lb.roundRobinSelect()
	}
}

// roundRobinSelect selects a backend using round-robin algorithm
func (lb *LoadBalancer) roundRobinSelect() *Backend {
	// Initial position
	initialPosition := lb.current

	// Find next alive backend
	for i := 0; i < len(lb.backends); i++ {
		idx := (initialPosition + i) % len(lb.backends)
		if lb.backends[idx].IsAlive() {
			lb.current = idx
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

		b.mux.RLock()
		connCount := b.connections
		b.mux.RUnlock()

		if leastConn == -1 || connCount < leastConn {
			leastConn = connCount
			leastConnBackend = b
		}
	}

	return leastConnBackend
}

// ipHashSelect selects a backend based on client IP hash
func (lb *LoadBalancer) ipHashSelect(r *http.Request) *Backend {
	// Extract client IP
	ip := getClientIP(r)

	// Hash the IP
	hash := fnv.New32()
	hash.Write([]byte(ip))
	idx := hash.Sum32() % uint32(len(lb.backends))

	// Find the selected backend or next available
	initialIdx := idx
	for i := 0; i < len(lb.backends); i++ {
		checkIdx := (initialIdx + uint32(i)) % uint32(len(lb.backends))
		if lb.backends[checkIdx].IsAlive() {
			return lb.backends[checkIdx]
		}
	}

	return nil
}

// randomSelect randomly selects an alive backend
func (lb *LoadBalancer) randomSelect() *Backend {
	// Count alive backends and get their indices
	var aliveIndices []int
	for i, b := range lb.backends {
		if b.IsAlive() {
			aliveIndices = append(aliveIndices, i)
		}
	}

	if len(aliveIndices) == 0 {
		return nil
	}

	// Pick a random alive backend
	randomIdx := aliveIndices[rand.Intn(len(aliveIndices))]
	return lb.backends[randomIdx]
}

// weightedRoundRobinSelect selects a backend based on its weight
func (lb *LoadBalancer) weightedRoundRobinSelect() *Backend {
	// Count total weight of alive backends
	totalWeight := 0
	for _, b := range lb.backends {
		if b.IsAlive() {
			totalWeight += b.weight
		}
	}

	if totalWeight == 0 {
		return nil
	}

	// Pick a random point in the total weight
	targetWeight := rand.Intn(totalWeight)
	currentWeight := 0

	// Find the backend that contains this weight point
	for _, b := range lb.backends {
		if !b.IsAlive() {
			continue
		}

		currentWeight += b.weight
		if targetWeight < currentWeight {
			return b
		}
	}

	// Fallback - should not reach here
	return lb.roundRobinSelect()
}

// getClientIP extracts the client IP from a request
func getClientIP(r *http.Request) string {
	// Check for X-Forwarded-For header first
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		// X-Forwarded-For can contain multiple IPs, use the first one
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Otherwise use RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (lb *LoadBalancer) fastRoundRobinSelect() *Backend {
	numBackends := len(lb.backends)
	initialIndex := int(atomic.LoadUint32(&lb.atomicCurrent)) % numBackends

	for i := 0; i < numBackends; i++ {
		idx := (initialIndex + i) % numBackends
		if lb.backends[idx].IsAlive() {
			atomic.StoreUint32(&lb.atomicCurrent, uint32(idx+1))
			return lb.backends[idx]
		}
	}

	return nil
}

// NextBackend returns the next available backend using round-robin selection
func (lb *LoadBalancer) NextBackend() *Backend {
	lb.mux.Lock()
	defer lb.mux.Unlock()

	// Keep track of starting position to avoid infinite loop
	initialIndex := lb.current

	// Try to find a healthy backend
	for i := 0; i < len(lb.backends); i++ {
		idx := (initialIndex + i) % len(lb.backends)
		if lb.backends[idx].IsAlive() {
			lb.current = idx
			return lb.backends[idx]
		}
	}

	// No healthy backends found
	return nil
}

// healthCheck performs health checks on all backends
func (lb *LoadBalancer) healthCheck() {
	// Create a transport with connection pooling
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   3 * time.Second,
	}

	ticker := time.NewTicker(lb.healthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Use a worker pool to check health in parallel
		results := make(chan struct {
			index int
			alive bool
		}, len(lb.backends))

		// Launch goroutines for each backend
		for i, backend := range lb.backends {
			go func(i int, backend *Backend) {
				alive := isBackendAliveHTTP(backend.URL, client)
				results <- struct {
					index int
					alive bool
				}{i, alive}
			}(i, backend)
		}

		// Collect results
		for i := 0; i < len(lb.backends); i++ {
			result := <-results
			backend := lb.backends[result.index]
			backend.SetAlive(result.alive)

			// Update metrics
			backendLabel := backend.URL.Host
			if result.alive {
				lb.metrics.backendUpGauge.WithLabelValues(backendLabel).Set(1)
			} else {
				lb.metrics.backendUpGauge.WithLabelValues(backendLabel).Set(0)
				lb.metrics.backendErrors.WithLabelValues(backendLabel, "health_check").Inc()
			}
		}
	}
}

// isBackendAliveHTTP checks if a backend is alive by making an HTTP request
func isBackendAliveHTTP(u *url.URL, client *http.Client) bool {
	resp, err := client.Get(u.String() + "/health") // Assuming backends have a /health endpoint
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500 // Consider any non-5xx response as alive
}

// Use a pre-copy buffer pool for proxy operations
var bufferPool = &bufferPoolAdapter{
	pool: &sync.Pool{
		New: func() any {
			return make([]byte, 32*1024) // 32KB buffers
		},
	},
}

type bufferPoolAdapter struct {
	pool *sync.Pool
}

func (b *bufferPoolAdapter) Get() []byte {
	return b.pool.Get().([]byte)
}

func (b *bufferPoolAdapter) Put(buf []byte) {
	b.pool.Put(buf)
}

// Use an optimized reverse proxy that reuses buffers
func createOptimizedReverseProxy(target *url.URL) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)

		// Preserve the Host header if specified
		if req.Header.Get("Host") == "" {
			req.Host = target.Host
		}

		// If the target has query parameters, add them
		targetQuery := target.RawQuery
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
	}

	// Create a transport with optimized connection pooling
	transport := &http.Transport{
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
		// Enable HTTP/2 if needed
		ForceAttemptHTTP2: true,
	}

	return &httputil.ReverseProxy{
		Director:   director,
		Transport:  transport,
		BufferPool: bufferPool,
	}
}

// ServeHTTP implements the http.Handler interface
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := lb.chooseBackendByStrategy(r)
	if backend == nil {
		http.Error(w, "No available backends", http.StatusServiceUnavailable)
		lb.metrics.requestCount.WithLabelValues("none", "503", r.Method).Inc()
		return
	}

	// Track request start time
	start := time.Now()

	// Increment connection counter
	backend.mux.Lock()
	backend.connections++
	backend.mux.Unlock()

	// Update metrics for active connections
	backendLabel := backend.URL.Host
	lb.metrics.activeConnections.WithLabelValues(backendLabel).Inc()

	// Create a wrapped response writer to capture the status code
	wrappedWriter := &metricsResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// Forward the request to the backend
	log.Printf("Forwarding request to: %s", backend.URL.Host)
	backend.ReverseProxy.ServeHTTP(wrappedWriter, r)

	// Calculate request duration
	duration := time.Since(start).Seconds()

	// Decrement connection counter
	backend.mux.Lock()
	backend.connections--
	backend.mux.Unlock()

	// Update metrics for active connections
	lb.metrics.activeConnections.WithLabelValues(backendLabel).Dec()

	// Update request metrics
	statusCode := fmt.Sprintf("%d", wrappedWriter.statusCode)
	lb.metrics.requestCount.WithLabelValues(backendLabel, statusCode, r.Method).Inc()
	lb.metrics.requestDuration.WithLabelValues(backendLabel).Observe(duration)
	lb.metrics.backendResponseTime.WithLabelValues(backendLabel).Observe(duration)

	// Reset fail count on successful request
	if wrappedWriter.statusCode < 500 {
		backend.ResetFailCount()
	} else {
		lb.metrics.backendErrors.WithLabelValues(backendLabel, "response_error").Inc()
	}
}
