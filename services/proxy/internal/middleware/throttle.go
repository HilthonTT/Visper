// Package middleware holds the HTTP middlewares used in front of the proxy.
package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/hilthontt/visper/services/proxy/internal/throttling"
)

// Middleware wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain wraps h with middlewares; the first middleware is the outermost.
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for _, m := range slices.Backward(middlewares) {
		h = m(h)
	}
	return h
}

// Throttle rejects a request with 429 as soon as any level of h denies it.
func Throttle(h *throttling.Hierarchy) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if d := h.Allow(r); !d.Allowed {
				if d.RetryAfter > 0 {
					secs := (d.RetryAfter + time.Second - 1) / time.Second // round up
					w.Header().Set("Retry-After", strconv.Itoa(int(secs)))
				}
				http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
