package balancer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthCheck(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
		}
	}))
	defer healthy.Close()
	sick := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer sick.Close()

	lb := newTestLB(t, testConfig("", healthy.URL, sick.URL), Options{})
	lb.backends[0].setAlive(false)
	lb.checkAll(t.Context())
	if !lb.backends[0].IsAlive() || lb.backends[1].IsAlive() {
		t.Fatalf("alive = %v %v, want true false", lb.backends[0].IsAlive(), lb.backends[1].IsAlive())
	}
}

func TestReloadTriggersImmediateHealthCheck(t *testing.T) {
	var hits atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer backend.Close()

	cfg := testConfig("", "http://127.0.0.1:1") // nothing listens there
	cfg.HealthCheckInterval = 1 << 62           // effectively never ticks
	lb := newTestLB(t, cfg, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); lb.RunHealthChecks(ctx) }()
	defer func() { cancel(); <-done }()

	// Wait for the startup check to mark the unreachable backend down, so
	// only the reload itself can trigger the next check.
	deadline := time.Now().Add(5 * time.Second)
	for lb.backends[0].IsAlive() {
		if time.Now().After(deadline) {
			t.Fatal("startup health check did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cfg = testConfig("", backend.URL)
	cfg.HealthCheckInterval = 1 << 62 // unchanged: the interval alone does not wake the checker
	if err := lb.Reload(cfg); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for hits.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("new backend was not health-checked after reload")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Regression: the health client followed redirects, so a backend whose
// health endpoint redirects (e.g. to a login page elsewhere) was judged by
// the redirect target and could be marked down.
func TestHealthCheckRedirectMeansAlive(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/login", http.StatusFound) // nothing listens there
	}))
	defer backend.Close()

	lb := newTestLB(t, testConfig("", backend.URL), Options{})
	lb.checkAll(t.Context())
	if !lb.backends[0].IsAlive() {
		t.Fatal("backend answering its health check with a redirect was marked down")
	}
}

// Regression: checks cut short by shutdown failed and marked every backend
// down (and logged / counted health_check errors) on the way out.
func TestHealthCheckIgnoresCancelledChecks(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer backend.Close()

	lb := newTestLB(t, testConfig("", backend.URL), Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lb.checkAll(ctx)
	if !lb.backends[0].IsAlive() {
		t.Fatal("cancelled health check marked the backend down")
	}
}
