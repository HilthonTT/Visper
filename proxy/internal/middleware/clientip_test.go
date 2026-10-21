package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name    string
		trusted []string
		remote  string
		xff     []string
		want    string
	}{
		{"no xff", nil, "203.0.113.7:5555", nil, "203.0.113.7"},
		{"untrusted peer ignores spoofed xff", nil, "203.0.113.7:5555", []string{"1.2.3.4"}, "203.0.113.7"},
		{"trusted peer uses xff", []string{"10.0.0.0/8"}, "10.0.0.2:80", []string{"198.51.100.9"}, "198.51.100.9"},
		{"rightmost untrusted hop wins", []string{"10.0.0.0/8"}, "10.0.0.2:80", []string{"6.6.6.6, 198.51.100.9, 10.0.0.5"}, "198.51.100.9"},
		{"multiple xff headers", []string{"10.0.0.1"}, "10.0.0.1:80", []string{"6.6.6.6", "198.51.100.9"}, "198.51.100.9"},
		{"all hops trusted", []string{"10.0.0.0/8"}, "10.0.0.2:80", []string{"10.1.1.1"}, "10.1.1.1"},
		{"malformed hop stops walk", []string{"10.0.0.0/8"}, "10.0.0.2:80", []string{"garbage, 10.0.0.3"}, "10.0.0.3"},
		{"trusted peer without xff", []string{"10.0.0.0/8"}, "10.0.0.2:80", nil, "10.0.0.2"},
		{"ipv6 remote", nil, "[2001:db8::1]:443", nil, "2001:db8::1"},
		{"ipv4-mapped trusted", []string{"127.0.0.1"}, "[::ffff:127.0.0.1]:80", []string{"198.51.100.9"}, "198.51.100.9"},
		{"remote without port", nil, "203.0.113.7", nil, "203.0.113.7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewClientIPResolver(tt.trusted)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := res.ClientIP(r); got != tt.want {
				t.Fatalf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewClientIPResolverRejectsInvalid(t *testing.T) {
	for _, in := range []string{"not-an-ip", "10.0.0.0/99"} {
		if _, err := NewClientIPResolver([]string{in}); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestRequireBearerToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	tests := []struct {
		name   string
		token  string
		header string
		want   int
	}{
		{"disabled without token", "", "Bearer x", http.StatusNotFound},
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
			RequireBearerToken(tt.token, ok).ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status %d, want %d", w.Code, tt.want)
			}
		})
	}
}
