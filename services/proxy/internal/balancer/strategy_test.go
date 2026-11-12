package balancer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hilthontt/visper/services/proxy/internal/config"
)

func TestParseStrategy(t *testing.T) {
	tests := []struct {
		in      string
		want    Strategy
		wantErr bool
	}{
		{"round_robin", RoundRobin, false},
		{"least_connections", LeastConnections, false},
		{"ip_hash", IPHash, false},
		{"random", Random, false},
		{"weighted_round_robin", WeightedRoundRobin, false},
		{"bogus", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseStrategy(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseStrategy(%q) = %v, %v", tt.in, got, err)
		}
		if err == nil && got.String() != tt.in {
			t.Errorf("%v.String() = %q, want %q", got, got.String(), tt.in)
		}
	}
}

func TestBackendSelection(t *testing.T) {
	urls := []string{"http://a:1", "http://b:1", "http://c:1"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.1:1234"

	tests := []struct {
		name     string
		strategy string
		dead     []int
		conns    []int
		want     []string // successive picks
	}{
		{"round robin cycles", "round_robin", nil, nil, []string{"a:1", "b:1", "c:1", "a:1"}},
		{"round robin skips dead", "round_robin", []int{1}, nil, []string{"a:1", "c:1", "a:1", "c:1"}},
		{"least connections", "least_connections", nil, []int{5, 1, 3}, []string{"b:1", "b:1"}},
		{"least connections skips dead", "least_connections", []int{1}, []int{5, 1, 3}, []string{"c:1"}},
		{"least connections all dead", "least_connections", []int{0, 1, 2}, nil, []string{"<nil>"}},
		{"all dead", "round_robin", []int{0, 1, 2}, nil, []string{"<nil>"}},
		{"ip hash all dead", "ip_hash", []int{0, 1, 2}, nil, []string{"<nil>"}},
		{"random all dead", "random", []int{0, 1, 2}, nil, []string{"<nil>"}},
		{"weighted all dead", "weighted_round_robin", []int{0, 1, 2}, nil, []string{"<nil>"}},
		{"random only alive", "random", []int{0, 2}, nil, []string{"b:1", "b:1", "b:1"}},
		{"weighted only alive", "weighted_round_robin", []int{0, 1}, nil, []string{"c:1", "c:1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := newTestLB(t, testConfig(tt.strategy, urls...), Options{})
			for _, i := range tt.dead {
				lb.backends[i].setAlive(false)
			}
			for i, c := range tt.conns {
				lb.backends[i].addConnections(c)
			}
			for i, want := range tt.want {
				if got := hosts(lb.pick(req))[0]; got != want {
					t.Fatalf("pick %d = %s, want %s", i, got, want)
				}
			}
		})
	}
}

func TestIPHashIsStickyAndFailsOver(t *testing.T) {
	lb := newTestLB(t, testConfig("ip_hash", "http://a:1", "http://b:1", "http://c:1"), Options{})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "198.51.100.1:1000"
	first := lb.pick(r)
	r.RemoteAddr = "198.51.100.1:2000" // same IP, different port
	if got := lb.pick(r); got != first {
		t.Fatalf("ip hash not sticky: %s vs %s", first.URL.Host, got.URL.Host)
	}
	first.setAlive(false)
	if got := lb.pick(r); got == nil || got == first {
		t.Fatal("ip hash did not fail over to an alive backend")
	}
}

func TestWeightedDistribution(t *testing.T) {
	cfg := &config.Config{
		Strategy: "weighted_round_robin",
		Backends: []config.BackendConfig{{URL: "http://a:1", Weight: 3}, {URL: "http://b:1", Weight: 1}},
	}
	lb := newTestLB(t, cfg, Options{})
	counts := map[string]int{}
	for range 4000 {
		counts[lb.pick(httptest.NewRequest(http.MethodGet, "/", nil)).URL.Host]++
	}
	if ratio := float64(counts["a:1"]) / float64(counts["b:1"]); ratio < 2.4 || ratio > 3.8 {
		t.Fatalf("unexpected weight ratio %.2f (%v)", ratio, counts)
	}
}
