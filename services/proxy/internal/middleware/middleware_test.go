package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hilthontt/visper/services/proxy/internal/throttling"
)

var noContent = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

func TestRequireBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		header string
		want   int
	}{
		{"disabled without token", "", "Bearer ", http.StatusNotFound},
		{"missing header", "s3cret", "", http.StatusUnauthorized},
		{"wrong token", "s3cret", "Bearer nope", http.StatusUnauthorized},
		{"valid token", "s3cret", "Bearer s3cret", http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/admin/reload", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			w := httptest.NewRecorder()
			RequireBearerToken(tt.token)(noContent).ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestThrottle(t *testing.T) {
	h := throttling.NewHierarchy(&throttling.Level{
		Name:      "ip",
		Key:       func(r *http.Request) string { return r.RemoteAddr },
		Throttler: throttling.New(throttling.Config{MaxRequests: 1, Interval: time.Minute, Spans: 1, Cooldown: 3 * time.Second}, nil),
	})
	handler := Chain(noContent, Throttle(h))

	for i, want := range []int{http.StatusNoContent, http.StatusTooManyRequests} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != want {
			t.Fatalf("request %d: status %d, want %d", i, w.Code, want)
		}
		if want == http.StatusTooManyRequests && w.Header().Get("Retry-After") == "" {
			t.Fatal("429 without Retry-After")
		}
	}
}
