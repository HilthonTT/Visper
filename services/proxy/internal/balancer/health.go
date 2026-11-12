package balancer

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"
)

func newHealthClient() *http.Client {
	return &http.Client{
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
	}
}

// RunHealthChecks checks every backend immediately and then on every
// health-check interval until ctx is done.
func (lb *LoadBalancer) RunHealthChecks(ctx context.Context) {
	ticker := time.NewTicker(lb.interval())
	defer ticker.Stop()

	lb.checkAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-lb.intervalReset:
			ticker.Reset(lb.interval())
		case <-ticker.C:
			lb.checkAll(ctx)
		}
	}
}

func (lb *LoadBalancer) interval() time.Duration {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.healthCheckInterval
}

// checkAll checks every backend concurrently and waits for the results.
func (lb *LoadBalancer) checkAll(ctx context.Context) {
	lb.mu.Lock()
	backends := slices.Clone(lb.backends)
	path := lb.healthCheckPath
	lb.mu.Unlock()

	var wg sync.WaitGroup
	for _, b := range backends {
		wg.Go(func() {
			alive := lb.probe(ctx, b.URL, path)
			b.setAlive(alive)
			lb.setBackendUp(b, alive)
			if !alive {
				lb.metrics.BackendErrors.WithLabelValues(b.label(), "health_check").Inc()
			}
		})
	}
	wg.Wait()
}

// probe reports whether the backend answers its health endpoint with a
// non-5xx status.
func (lb *LoadBalancer) probe(ctx context.Context, u *url.URL, path string) bool {
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
	return resp.StatusCode < http.StatusInternalServerError
}

// setBackendUp records b's health in the backend_up gauge, unless a
// concurrent Reload has removed b. Reload deletes a removed backend's series;
// setting it afterwards (from an in-flight health check or proxied request)
// would resurrect a stale series that is never cleaned up.
func (lb *LoadBalancer) setBackendUp(b *Backend, up bool) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if !slices.Contains(lb.backends, b) {
		return
	}
	v := 0.0
	if up {
		v = 1
	}
	lb.metrics.BackendUpGauge.WithLabelValues(b.label()).Set(v)
}
