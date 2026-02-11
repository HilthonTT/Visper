package main

import (
	"hash/fnv"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
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
	mux                 sync.Mutex
	healthCheckInterval time.Duration
	maxFailCount        int
	strategy            Strategy
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
			ReverseProxy: httputil.NewSingleHostReverseProxy(url),
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
		return lb.roundRobinSelect()
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

// isBackendAlive checks if a backend is alive by establishing a TCP connection
func isBackendAlive(u *url.URL) bool {
	timeout := 2 * time.Second
	conn, err := net.DialTimeout("tcp", u.Host, timeout)
	if err != nil {
		log.Printf("Health check failed for %s: %v", u.Host, err)
		return false
	}
	defer conn.Close()
	return true
}

// healthCheck performs health checks on all backends
func (lb *LoadBalancer) healthCheck() {
	ticker := time.NewTicker(lb.healthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Starting health check...")
		for _, backend := range lb.backends {
			alive := isBackendAlive(backend.URL)
			backend.SetAlive(alive)
			status := "up"
			if !alive {
				status = "down"
			}
			log.Printf("Backend %s status: %s", backend.URL.Host, status)
		}
		log.Println("Health check completed")
	}
}

// ServeHTTP implements the http.Handler interface
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := lb.chooseBackendByStrategy(r)
	if backend == nil {
		http.Error(w, "No available backends", http.StatusServiceUnavailable)
		return
	}

	// Increment connection counter (for least connections strategy)
	backend.mux.Lock()
	backend.connections++
	backend.mux.Unlock()

	log.Printf("Forwarding request to: %s", backend.URL.Host)

	// Wrap the response writer to intercept the response status
	wrappedWriter := &responseWriterInterceptor{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	backend.ReverseProxy.ServeHTTP(wrappedWriter, r)

	// Decrement connection counter when request is done
	backend.mux.Lock()
	backend.connections--
	backend.mux.Unlock()

	// Reset fail count on successful request
	if wrappedWriter.statusCode < 500 {
		backend.ResetFailCount()
	}
}

// responseWriterInterceptor wraps http.ResponseWriter to capture the status code
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader intercepts the status code
func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}
