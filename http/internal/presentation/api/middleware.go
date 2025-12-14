package api

import (
	"fmt"
	"net/http"
)

func (app *Application) rateLimiterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceKey := app.ratelimiter.GetSourceKey(r)

		maxBurst := app.ratelimiter.GetMaxBurst()
		if !app.ratelimiter.Allow(sourceKey) {
			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", maxBurst))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", "1") // Retry after 1 second

			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		remaining := app.ratelimiter.Remaining(sourceKey)
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", maxBurst))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		next.ServeHTTP(w, r)
	})
}

func (app *Application) enableCors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// allow preflight requests from the browser API
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
