package balancer

import (
	"testing"

	"github.com/hilthontt/visper/services/proxy/internal/config"
)

// testConfig builds a config with weight-1 backends for urls.
func testConfig(strategy string, urls ...string) *config.Config {
	cfg := &config.Config{Strategy: strategy}
	for _, u := range urls {
		cfg.Backends = append(cfg.Backends, config.BackendConfig{URL: u, Weight: 1})
	}
	return cfg
}

func newTestLB(t *testing.T, cfg *config.Config, opts Options) *LoadBalancer {
	t.Helper()
	lb, err := New(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	return lb
}

func hosts(bs ...*Backend) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		if b == nil {
			out[i] = "<nil>"
			continue
		}
		out[i] = b.URL.Host
	}
	return out
}
