package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The rate limiter keys on c.ClientIP(). With no trusted proxies, a client
// must not be able to choose its own IP through X-Forwarded-For.
func TestClientIPIgnoresSpoofedForwardedForWithoutTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name    string
		trusted []string
		want    string
	}{
		{"no trusted proxies", nil, "203.0.113.7"},
		{"peer is a trusted proxy", []string{"203.0.113.7"}, "198.51.100.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			if err := r.SetTrustedProxies(tc.trusted); err != nil {
				t.Fatal(err)
			}
			var got string
			r.GET("/", func(c *gin.Context) { got = c.ClientIP() })

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "203.0.113.7:4444"
			req.Header.Set("X-Forwarded-For", "198.51.100.1")
			r.ServeHTTP(httptest.NewRecorder(), req)

			if got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}
