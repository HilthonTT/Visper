package balancer

import (
	"errors"
	"time"

	"github.com/hilthontt/visper/services/proxy/internal/config"
)

// Reload replaces the load balancer configuration. Backends whose URL is
// unchanged are kept, along with their health state and open connections.
// On error nothing is changed.
func (lb *LoadBalancer) Reload(cfg *config.Config) error {
	if err := lb.apply(cfg); err != nil {
		return err
	}
	select {
	case lb.reloaded <- struct{}{}:
	default: // a wake-up is already pending
	}
	logf("configuration reloaded: %d backends, strategy %s", len(cfg.Backends), cfg.Strategy)
	return nil
}

// apply validates cfg and swaps it in atomically.
func (lb *LoadBalancer) apply(cfg *config.Config) error {
	strategy, err := ParseStrategy(orDefault(cfg.Strategy, config.DefaultStrategy))
	if err != nil {
		return err
	}

	lb.mu.Lock()
	backends, added, err := lb.buildBackends(cfg.Backends)
	if err != nil {
		lb.mu.Unlock()
		return err
	}
	removed := make(map[*Backend]struct{}, len(lb.backends))
	for _, b := range lb.backends {
		removed[b] = struct{}{}
	}
	for i, b := range backends {
		delete(removed, b)
		b.weight = max(1, cfg.Backends[i].Weight)
	}

	lb.backends = backends
	lb.next = 0
	lb.strategy = strategy
	lb.healthCheckInterval = orDefault(time.Duration(cfg.HealthCheckInterval), config.DefaultHealthCheckInterval)
	lb.healthCheckPath = orDefault(cfg.HealthCheckPath, config.DefaultHealthCheckPath)
	lb.maxFailCount = orDefault(cfg.MaxFailCount, config.DefaultMaxFailCount)

	for _, b := range added {
		lb.metrics.BackendUpGauge.WithLabelValues(b.label()).Set(1)
	}
	lb.mu.Unlock()

	for b := range removed {
		lb.metrics.BackendUpGauge.DeleteLabelValues(b.label())
	}
	return nil
}

// buildBackends returns the backend list for cfgs, reusing current backends
// with the same URL, and the backends that are new. It does not modify any
// shared state. Must be called with lb.mu held.
func (lb *LoadBalancer) buildBackends(cfgs []config.BackendConfig) (all, added []*Backend, err error) {
	if len(cfgs) == 0 {
		return nil, nil, errors.New("at least one backend is required")
	}
	current := make(map[string]*Backend, len(lb.backends))
	for _, b := range lb.backends {
		current[b.URL.String()] = b
	}

	all = make([]*Backend, len(cfgs))
	seen := make(map[string]struct{}, len(cfgs))
	for i, c := range cfgs {
		u, err := config.ParseBackendURL(c.URL)
		if err != nil {
			return nil, nil, err
		}
		key := u.String()
		if _, dup := seen[key]; dup {
			return nil, nil, errors.New("duplicate backend url " + key)
		}
		seen[key] = struct{}{}

		if b, ok := current[key]; ok {
			all[i] = b
			continue
		}
		b := &Backend{URL: u, alive: true}
		b.proxy = lb.newReverseProxy(b)
		all[i] = b
		added = append(added, b)
	}
	return all, added, nil
}

func orDefault[T int | string | time.Duration](v, def T) T {
	var zero T
	if v <= zero {
		return def
	}
	return v
}
