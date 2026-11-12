package balancer

import (
	"net/http/httputil"
	"net/url"
	"sync"
)

// Backend is an upstream server.
type Backend struct {
	URL   *url.URL
	proxy *httputil.ReverseProxy

	mu          sync.Mutex
	alive       bool
	failCount   int // consecutive proxy errors
	connections int // in-flight requests, including open WebSockets

	weight int // guarded by LoadBalancer.mu
}

// label identifies the backend in metrics and logs.
func (b *Backend) label() string { return b.URL.Host }

// IsAlive reports whether the backend currently receives traffic.
func (b *Backend) IsAlive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.alive
}

// Connections returns the number of in-flight requests to the backend.
func (b *Backend) Connections() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.connections
}

func (b *Backend) setAlive(alive bool) {
	b.mu.Lock()
	b.alive = alive
	if alive {
		b.failCount = 0
	}
	b.mu.Unlock()
}

// recordFailure counts a proxy error and marks the backend down once
// maxFail consecutive errors are reached. It reports whether this call
// took the backend down.
func (b *Backend) recordFailure(maxFail int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failCount++
	if b.alive && b.failCount >= maxFail {
		b.alive = false
		return true
	}
	return false
}

func (b *Backend) resetFailures() {
	b.mu.Lock()
	b.failCount = 0
	b.mu.Unlock()
}

func (b *Backend) addConnections(delta int) {
	b.mu.Lock()
	b.connections += delta
	b.mu.Unlock()
}
