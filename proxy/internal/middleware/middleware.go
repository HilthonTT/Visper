package middleware

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"time"

	"github.com/hilthontt/visper/proxy/internal/throttling"
)

const rateLimitMessage = "Rate limit exceeded. Please try again later."

func tooManyRequests(w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
	}
	http.Error(w, rateLimitMessage, http.StatusTooManyRequests)
}

// Throttling limits requests per method and path.
func Throttling(throttler *throttling.Throttler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := r.Method + ":" + r.URL.Path
			if !throttler.IncrementAndCheck(route, 1) {
				tooManyRequests(w, throttler.Cooldown())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HierarchicalThrottling rejects a request with 429 as soon as any level denies it.
func HierarchicalThrottling(ht *throttling.HierarchicalThrottler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ok, level := ht.CheckRequest(r); !ok {
				tooManyRequests(w, level.Throttler.Cooldown())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireBearerToken protects a handler with a static bearer token. An empty
// token disables the handler entirely (404), so admin endpoints are never
// exposed unauthenticated by default.
func RequireBearerToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.NotFound(w, r)
			return
		}
		got := r.Header.Get("Authorization")
		want := "Bearer " + token
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="proxy-admin"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
