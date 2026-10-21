package option

import (
	"context"
	"net/http"
	"testing"

	"github.com/hilthontt/visper/api-sdk/internal/requestconfig"
)

func TestQueryOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []RequestOption
		want string
	}{
		{"set", []RequestOption{WithQuery("a", "1"), WithQuery("a", "2")}, "a=2"},
		{"add", []RequestOption{WithQuery("a", "1"), WithQueryAdd("a", "2")}, "a=1&a=2"},
		{"escapes", []RequestOption{WithQuery("q", "x y+z")}, "q=x+y%2Bz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := requestconfig.NewRequestConfig(context.Background(), http.MethodGet, "p", nil, nil, tt.opts...)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Request.URL.RawQuery; got != tt.want {
				t.Fatalf("query = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithBaseURLAddsTrailingSlash(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://h", "http://h"},
		{"http://h/api", "http://h/api/"},
		{"http://h/api/", "http://h/api/"},
	}
	for _, tt := range tests {
		cfg, err := requestconfig.NewRequestConfig(context.Background(), http.MethodGet, "", nil, nil, WithBaseURL(tt.in))
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.BaseURL.String(); got != tt.want {
			t.Errorf("WithBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
