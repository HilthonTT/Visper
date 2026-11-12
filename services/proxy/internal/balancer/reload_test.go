package balancer

import (
	"testing"
	"time"

	"github.com/hilthontt/visper/services/proxy/internal/config"
	"github.com/hilthontt/visper/services/proxy/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestNewRejectsBadConfig(t *testing.T) {
	for _, cfg := range []*config.Config{
		testConfig("", "localhost:5005"),
		testConfig("", "ftp://x"),
		testConfig("", "http://"),
		testConfig(""),
		testConfig("", "http://a:1", "http://a:1"),
		testConfig("nope", "http://a:1"),
	} {
		if _, err := New(cfg, Options{}); err == nil {
			t.Errorf("expected error for %+v", cfg)
		}
	}
}

func TestReloadKeepsBackendsByURL(t *testing.T) {
	lb := newTestLB(t, testConfig("", "http://a:1", "http://b:1"), Options{})
	a, b := lb.backends[0], lb.backends[1]
	b.setAlive(false)

	err := lb.Reload(&config.Config{
		Strategy:            "least_connections",
		HealthCheckInterval: config.Duration(time.Second),
		Backends:            []config.BackendConfig{{URL: "http://c:1"}, {URL: "http://b:1", Weight: 5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := lb.Backends()
	if hosts(got...)[0] != "c:1" || got[1] != b || got[1].weight != 5 || got[1].IsAlive() {
		t.Fatalf("reload did not preserve existing backend: %v", hosts(got...))
	}
	for _, x := range got {
		if x == a {
			t.Fatal("removed backend still present")
		}
	}
	if lb.strategy != LeastConnections || lb.interval() != time.Second {
		t.Fatalf("settings not updated: strategy=%v interval=%v", lb.strategy, lb.interval())
	}
	select {
	case <-lb.reloaded:
	default:
		t.Fatal("reload did not wake the health checker")
	}
}

func TestFailedReloadChangesNothing(t *testing.T) {
	lb := newTestLB(t, testConfig("", "http://a:1"), Options{})
	before := lb.Backends()
	for _, cfg := range []*config.Config{
		{Strategy: "round_robin", Backends: []config.BackendConfig{{URL: "http://a:1", Weight: 9}, {URL: "not a url"}}},
		{Strategy: "nope", Backends: []config.BackendConfig{{URL: "http://b:1"}}},
	} {
		if err := lb.Reload(cfg); err == nil {
			t.Fatalf("expected error for %+v", cfg)
		}
	}
	after := lb.Backends()
	if len(after) != 1 || after[0] != before[0] || after[0].weight != 1 || lb.strategy != RoundRobin {
		t.Fatalf("failed reload changed state: %v weight=%d strategy=%v", hosts(after...), after[0].weight, lb.strategy)
	}
}

func TestRemovedBackendGaugeNotResurrected(t *testing.T) {
	reg := prometheus.NewRegistry()
	lb := newTestLB(t, testConfig("", "http://a:1", "http://b:1"), Options{Metrics: metrics.New("t", reg)})
	removed := lb.backends[1]
	if err := lb.Reload(testConfig("round_robin", "http://a:1")); err != nil {
		t.Fatal(err)
	}
	// An in-flight health check or proxy error for the removed backend
	// finishing after the reload.
	lb.setBackendUp(removed, false)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "t_backend_up" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetValue() == "b:1" {
					t.Fatal("backend_up series for removed backend was recreated")
				}
			}
		}
	}
}
