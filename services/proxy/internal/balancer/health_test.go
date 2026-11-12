package balancer

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
