package api

import (
	"net/http"

	"github.com/hilthontt/visper/internal/infrastructure/json"
)

func (app *Application) rateLimiterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allow, retryAfter := app.ratelimiter.Allow(r.RemoteAddr); !allow {
			json.WriteRateLimitError(w, int(retryAfter))
			return
		}

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
