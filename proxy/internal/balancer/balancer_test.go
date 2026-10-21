package balancer

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hilthontt/visper/proxy/internal/config"
)

func newTestLB(t *testing.T, urls []string, opts Options) *LoadBalancer {
	t.Helper()
	cfgs := make([]config.BackendConfig, len(urls))
	for i, u := range urls {
		cfgs[i] = config.BackendConfig{URL: u, Weight: 1}
	}
	lb, err := New(cfgs, opts)
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
	}
}

func TestBackendSelection(t *testing.T) {
	urls := []string{"http://a:1", "http://b:1", "http://c:1"}
	req := func(ip string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = ip + ":1234"
		return r
	}

	tests := []struct {
		name     string
		strategy Strategy
		dead     []int
		conns    []int
		want     []string // successive picks
	}{
		{"round robin cycles", RoundRobin, nil, nil, []string{"a:1", "b:1", "c:1", "a:1"}},
		{"round robin skips dead", RoundRobin, []int{1}, nil, []string{"a:1", "c:1", "a:1", "c:1"}},
		{"least connections", LeastConnections, nil, []int{5, 1, 3}, []string{"b:1", "b:1"}},
		{"least connections skips dead", LeastConnections, []int{1}, []int{5, 1, 3}, []string{"c:1"}},
		{"all dead", RoundRobin, []int{0, 1, 2}, nil, []string{"<nil>"}},
		{"random all dead", Random, []int{0, 1, 2}, nil, []string{"<nil>"}},
		{"weighted all dead", WeightedRoundRobin, []int{0, 1, 2}, nil, []string{"<nil>"}},
		{"random only alive", Random, []int{0, 2}, nil, []string{"b:1", "b:1", "b:1"}},
		{"weighted only alive", WeightedRoundRobin, []int{0, 1}, nil, []string{"c:1", "c:1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := newTestLB(t, urls, Options{Strategy: tt.strategy})
			for _, i := range tt.dead {
				lb.backends[i].SetAlive(false)
			}
			for i, c := range tt.conns {
				lb.backends[i].addConnections(c)
			}
			for i, want := range tt.want {
				if got := hosts(lb.chooseBackendByStrategy(req("198.51.100.1")))[0]; got != want {
					t.Fatalf("pick %d = %s, want %s", i, got, want)
				}
			}
		})
	}
}

func TestIPHashIsStickyAndFailsOver(t *testing.T) {
	lb := newTestLB(t, []string{"http://a:1", "http://b:1", "http://c:1"}, Options{Strategy: IPHash})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "198.51.100.1:1000"
	first := lb.chooseBackendByStrategy(r)
	r.RemoteAddr = "198.51.100.1:2000" // same IP, different port
	if got := lb.chooseBackendByStrategy(r); got != first {
		t.Fatalf("ip hash not sticky: %s vs %s", first.URL.Host, got.URL.Host)
	}
	first.SetAlive(false)
	if got := lb.chooseBackendByStrategy(r); got == nil || got == first {
		t.Fatal("ip hash did not fail over to an alive backend")
	}
}

func TestWeightedDistribution(t *testing.T) {
	lb, err := New([]config.BackendConfig{{URL: "http://a:1", Weight: 3}, {URL: "http://b:1", Weight: 1}}, Options{Strategy: WeightedRoundRobin})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for range 4000 {
		counts[lb.chooseBackendByStrategy(httptest.NewRequest(http.MethodGet, "/", nil)).URL.Host]++
	}
	if ratio := float64(counts["a:1"]) / float64(counts["b:1"]); ratio < 2.4 || ratio > 3.8 {
		t.Fatalf("unexpected weight ratio %.2f (%v)", ratio, counts)
	}
}

func TestNewRejectsBadBackends(t *testing.T) {
	for _, u := range []string{"localhost:5005", "ftp://x", "http://"} {
		if _, err := New([]config.BackendConfig{{URL: u}}, Options{}); err == nil {
			t.Errorf("expected error for %q", u)
		}
	}
	if _, err := New(nil, Options{}); err == nil {
		t.Error("expected error for no backends")
	}
}

func TestProxyForwardsRequest(t *testing.T) {
	type seen struct {
		path, query, host, xff, realIP, conn string
	}
	got := make(chan seen, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{r.URL.Path, r.URL.RawQuery, r.Host, r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-IP"), r.Header.Get("Keep-Alive")}
		w.Header().Set("X-Backend", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "hello")
	}))
	defer backend.Close()

	lb := newTestLB(t, []string{backend.URL + "/api"}, Options{})
	front := httptest.NewServer(lb)
	defer front.Close()

	req, _ := http.NewRequest(http.MethodPost, front.URL+"/rooms?id=1", strings.NewReader("body"))
	req.Header.Set("X-Forwarded-For", "6.6.6.6") // spoofed, peer is untrusted
	req.Header.Set("X-Real-IP", "6.6.6.6")
	req.Header.Set("Keep-Alive", "timeout=5") // hop-by-hop
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated || string(body) != "hello" || resp.Header.Get("X-Backend") != "ok" {
		t.Fatalf("unexpected response %d %q %v", resp.StatusCode, body, resp.Header)
	}
	s := <-got
	backendURL, _ := url.Parse(backend.URL)
	want := seen{"/api/rooms", "id=1", backendURL.Host, "127.0.0.1", "127.0.0.1", ""}
	if s != want {
		t.Fatalf("backend saw %+v, want %+v", s, want)
	}
}

func TestProxyTrustedForwardedFor(t *testing.T) {
	got := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Forwarded-For")
	}))
	defer backend.Close()

	lb := newTestLB(t, []string{backend.URL}, Options{TrustForwarded: func(*http.Request) bool { return true }})
	front := httptest.NewServer(lb)
	defer front.Close()

	req, _ := http.NewRequest(http.MethodGet, front.URL, nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if xff := <-got; xff != "198.51.100.9, 127.0.0.1" {
		t.Fatalf("X-Forwarded-For = %q", xff)
	}
}

func TestProxyErrorMarksBackendDown(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close() // connection refused from now on

	lb := newTestLB(t, []string{deadURL}, Options{MaxFailCount: 2})
	front := httptest.NewServer(lb)
	defer front.Close()

	wantCodes := []int{http.StatusBadGateway, http.StatusBadGateway, http.StatusServiceUnavailable}
	for i, want := range wantCodes {
		resp, err := http.Get(front.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("request %d: status %d, want %d", i, resp.StatusCode, want)
		}
	}
	if c := lb.backends[0].Connections(); c != 0 {
		t.Fatalf("connection counter leaked: %d", c)
	}
}

func TestProxyWebSocketUpgrade(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			http.Error(w, "expected upgrade", http.StatusBadRequest)
			return
		}
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = brw.Flush()
		line, _ := brw.ReadString('\n') // echo one line
		_, _ = brw.WriteString("echo:" + line)
		_ = brw.Flush()
	}))
	defer backend.Close()

	lb := newTestLB(t, []string{backend.URL}, Options{})
	front := httptest.NewUnstartedServer(lb)
	front.Config.WriteTimeout = 50 * time.Millisecond // upgraded conns must outlive it
	front.Start()
	defer front.Close()

	conn, err := net.Dial("tcp", front.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status %d, want 101", resp.StatusCode)
	}
	time.Sleep(100 * time.Millisecond) // past the server WriteTimeout
	_, _ = io.WriteString(conn, "ping\n")
	line, err := br.ReadString('\n')
	if err != nil || line != "echo:ping\n" {
		t.Fatalf("got %q, %v", line, err)
	}
}

func TestReloadKeepsBackendsByURL(t *testing.T) {
	lb := newTestLB(t, []string{"http://a:1", "http://b:1"}, Options{})
	a, b := lb.backends[0], lb.backends[1]
	b.SetAlive(false)

	err := lb.Reload(&config.Config{
		Strategy: "least_connections",
		Backends: []config.BackendConfig{{URL: "http://c:1"}, {URL: "http://b:1", Weight: 5}},
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
	if lb.strategy != LeastConnections {
		t.Fatal("strategy not updated")
	}
	if err := lb.Reload(&config.Config{Strategy: "nope", Backends: []config.BackendConfig{{URL: "http://a:1"}}}); err == nil {
		t.Fatal("expected error for bad strategy")
	}
}

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

	lb := newTestLB(t, []string{healthy.URL, sick.URL}, Options{})
	lb.backends[0].SetAlive(false)
	lb.checkAll(t.Context())
	if !lb.backends[0].IsAlive() || lb.backends[1].IsAlive() {
		t.Fatalf("alive = %v %v, want true false", lb.backends[0].IsAlive(), lb.backends[1].IsAlive())
	}
}
