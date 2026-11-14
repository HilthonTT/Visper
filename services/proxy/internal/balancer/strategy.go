package balancer

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"net/http"
)

// Strategy selects which backend serves a request.
type Strategy int

const (
	RoundRobin Strategy = iota
	LeastConnections
	IPHash
	Random
	// WeightedRoundRobin picks backends at random, proportionally to their weight.
	WeightedRoundRobin
)

var strategyNames = map[Strategy]string{
	RoundRobin:         "round_robin",
	LeastConnections:   "least_connections",
	IPHash:             "ip_hash",
	Random:             "random",
	WeightedRoundRobin: "weighted_round_robin",
}

// ParseStrategy converts a configuration name to a Strategy.
func ParseStrategy(s string) (Strategy, error) {
	for st, name := range strategyNames {
		if name == s {
			return st, nil
		}
	}
	return 0, fmt.Errorf("unknown strategy %q", s)
}

func (s Strategy) String() string {
	if name, ok := strategyNames[s]; ok {
		return name
	}
	return fmt.Sprintf("Strategy(%d)", int(s))
}

// pick returns an alive backend for r, or nil if none is alive.
func (lb *LoadBalancer) pick(r *http.Request) *Backend {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	switch lb.strategy {
	case LeastConnections:
		return lb.leastConnections()
	case IPHash:
		return lb.ipHash(r)
	case Random:
		return lb.random()
	case WeightedRoundRobin:
		return lb.weighted()
	default:
		return lb.roundRobin()
	}
}

// The selection methods below must be called with lb.mu held.

func (lb *LoadBalancer) roundRobin() *Backend {
	n := len(lb.backends)
	for i := range n {
		idx := (lb.next + i) % n
		if b := lb.backends[idx]; b.IsAlive() {
			lb.next = (idx + 1) % n
			return b
		}
	}
	return nil
}

func (lb *LoadBalancer) leastConnections() *Backend {
	var best *Backend
	bestConns := 0
	for _, b := range lb.backends {
		if !b.IsAlive() {
			continue
		}
		if c := b.Connections(); best == nil || c < bestConns {
			best, bestConns = b, c
		}
	}
	return best
}

// ipHash maps the client IP to a backend, falling over to the next alive
// backend if that one is down.
func (lb *LoadBalancer) ipHash(r *http.Request) *Backend {
	h := fnv.New32a()
	_, _ = h.Write([]byte(lb.clientIP(r)))
	n := uint32(len(lb.backends))
	start := h.Sum32() % n
	for i := range n {
		if b := lb.backends[(start+i)%n]; b.IsAlive() {
			return b
		}
	}
	return nil
}

func (lb *LoadBalancer) random() *Backend {
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

// weighted picks an alive backend with probability proportional to its weight.
func (lb *LoadBalancer) weighted() *Backend {
	// One snapshot of who is alive: health checks change it concurrently, and
	// a second pass seeing a different set could pick no backend at all.
	var alive []*Backend
	total := 0
	for _, b := range lb.backends {
		if b.IsAlive() {
			alive = append(alive, b)
			total += b.weight
		}
	}
	if total == 0 {
		return nil
	}
	target := rand.IntN(total)
	for _, b := range alive {
		if target -= b.weight; target < 0 {
			return b
		}
	}
	return nil
}
