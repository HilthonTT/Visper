package balancer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hilthontt/visper/services/proxy/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestProxyForwardsRequest(t *testing.T) {
	type seen struct {
		path, query, host, xff, realIP, proto, keepAlive string
	}
	got := make(chan seen, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{r.URL.Path, r.URL.RawQuery, r.Host, r.Header.Get("X-Forwarded-For"),
			r.Header.Get("X-Real-IP"), r.Header.Get("X-Forwarded-Proto"), r.Header.Get("Keep-Alive")}
		w.Header().Set("X-Backend", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "hello")
	}))
	defer backend.Close()

	lb := newTestLB(t, testConfig("", backend.URL+"/api"), Options{})
	front := httptest.NewServer(lb)
	defer front.Close()

	req, _ := http.NewRequest(http.MethodPost, front.URL+"/rooms?id=1", strings.NewReader("body"))
	// Spoofed: the peer is untrusted, so these must be replaced.
	req.Header.Set("X-Forwarded-For", "6.6.6.6")
	req.Header.Set("X-Real-IP", "6.6.6.6")
	req.Header.Set("X-Forwarded-Proto", "https")
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
	want := seen{"/api/rooms", "id=1", backendURL.Host, "127.0.0.1", "127.0.0.1", "http", ""}
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

	lb := newTestLB(t, testConfig("", backend.URL), Options{TrustForwarded: func(*http.Request) bool { return true }})
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

	m := metrics.New("t", prometheus.NewRegistry())
	cfg := testConfig("", deadURL)
	cfg.MaxFailCount = 2
	lb := newTestLB(t, cfg, Options{Metrics: m})
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

	host := lb.backends[0].label()
	if n := testutil.ToFloat64(m.BackendErrors.WithLabelValues(host, "proxy_error")); n != 2 {
		t.Errorf("proxy_error = %v, want 2", n)
	}
	// Regression: proxy errors used to be counted as response errors too.
	if n := testutil.ToFloat64(m.BackendErrors.WithLabelValues(host, "response_error")); n != 0 {
		t.Errorf("response_error = %v, want 0", n)
	}
	if up := testutil.ToFloat64(m.BackendUpGauge.WithLabelValues(host)); up != 0 {
		t.Errorf("backend_up = %v, want 0", up)
	}
}

func TestBackend5xxCountsAsResponseError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	m := metrics.New("t", prometheus.NewRegistry())
	lb := newTestLB(t, testConfig("", backend.URL), Options{Metrics: m})
	w := httptest.NewRecorder()
	lb.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	host := lb.backends[0].label()
	if n := testutil.ToFloat64(m.BackendErrors.WithLabelValues(host, "response_error")); n != 1 {
		t.Errorf("response_error = %v, want 1", n)
	}
	if n := testutil.ToFloat64(m.RequestCount.WithLabelValues(host, "500", http.MethodGet)); n != 1 {
		t.Errorf("request_count{status=500} = %v, want 1", n)
	}
}

// Regression: a client hanging up was answered with 499, which counted as
// a success and cleared the backend's failure count.
func TestClientDisconnectKeepsFailureCount(t *testing.T) {
	lb := newTestLB(t, testConfig("", "http://127.0.0.1:1"), Options{})
	b := lb.backends[0]
	b.recordFailure(10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the client is already gone
	w := httptest.NewRecorder()
	lb.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))

	if w.Code != statusClientClosedRequest {
		t.Fatalf("status %d, want %d", w.Code, statusClientClosedRequest)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failCount != 1 {
		t.Fatalf("failCount = %d, want 1 (unchanged)", b.failCount)
	}
}

// Regression: a TLS handshake timeout is a net.Error timeout but does not
// match context.DeadlineExceeded, so it was answered with 502.
func TestTLSHandshakeTimeoutIs504(t *testing.T) {
	// Accepts TCP connections but never speaks TLS.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()

	lb := newTestLB(t, testConfig("", "https://"+ln.Addr().String()), Options{})
	lb.backends[0].proxy.Transport = &http.Transport{TLSHandshakeTimeout: 50 * time.Millisecond}

	w := httptest.NewRecorder()
	lb.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("status %d, want 504", w.Code)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestIsTimeout(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{context.DeadlineExceeded, true},
		// net.Error timeouts that don't match context.DeadlineExceeded,
		// like http.Transport's TLS handshake timeout.
		{fmt.Errorf("proxy: %w", timeoutErr{}), true},
		{&net.OpError{Op: "read", Err: timeoutErr{}}, true},
		{context.Canceled, false},
		{io.ErrUnexpectedEOF, false},
	}
	for _, tt := range tests {
		if got := isTimeout(tt.err); got != tt.want {
			t.Errorf("isTimeout(%v) = %v, want %v", tt.err, got, tt.want)
		}
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

	lb := newTestLB(t, testConfig("", backend.URL), Options{})
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
	_, _ = io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: keep-alive, Upgrade\r\n\r\n")

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
