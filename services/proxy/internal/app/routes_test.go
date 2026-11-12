package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hilthontt/visper/services/proxy/internal/balancer"
	"github.com/hilthontt/visper/services/proxy/internal/clientip"
	"github.com/hilthontt/visper/services/proxy/internal/config"
)

func newTestRoutes(t *testing.T, env map[string]string) (http.Handler, *balancer.LoadBalancer) {
	t.Helper()
	cfg, err := config.Load(nil, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	lb, err := balancer.New(&cfg.LB, balancer.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ips, _ := clientip.NewResolver(nil)
	return routes(cfg, lb, newThrottle(cfg.Throttle, ips, nil)), lb
}

func serve(h http.Handler, method, path, auth string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAdminReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lb.json")
	write := func(s string) {
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"backends":[{"url":"http://a:1"}]}`)
	h, lb := newTestRoutes(t, map[string]string{"PROXY_CONFIG": path, "PROXY_ADMIN_TOKEN": "s3cret"})

	if w := serve(h, http.MethodPost, "/admin/reload", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated reload: status %d", w.Code)
	}

	write(`{"backends":[{"url":"http://b:1"},{"url":"http://c:1"}]}`)
	if w := serve(h, http.MethodPost, "/admin/reload", "Bearer s3cret"); w.Code != http.StatusOK {
		t.Fatalf("reload: status %d: %s", w.Code, w.Body)
	}
	if n := len(lb.Backends()); n != 2 {
		t.Fatalf("%d backends after reload, want 2", n)
	}

	write(`{"backends":[{"url":"nope"}]}`)
	w := serve(h, http.MethodPost, "/admin/reload", "Bearer s3cret")
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "nope") {
		t.Fatalf("bad reload: status %d: %s", w.Code, w.Body)
	}
	if n := len(lb.Backends()); n != 2 {
		t.Fatal("failed reload changed the backends")
	}
}
