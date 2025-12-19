package api

import (
	"fmt"
	"net/http"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriter) WriterHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func (app *Application) rateLimiterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceKey := app.ratelimiter.GetSourceKey(r)

		maxBurst := app.ratelimiter.GetMaxBurst()
		if !app.ratelimiter.Allow(sourceKey) {
			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", maxBurst))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", "1") // Retry after 1 second

			app.logger.Warnw("rate limit exceeded",
				"source", sourceKey,
				"path", r.URL.Path,
				"method", r.Method,
			)

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
		origin := r.Header.Get("Origin")

		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// allow preflight requests from the browser API
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (app *Application) loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := newResponseWriter(w)
		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		fields := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration_ms", duration.Milliseconds(),
			"bytes", wrapped.bytes,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"client_ip", r.RemoteAddr,
		}

		if r.URL.RawQuery != "" {
			fields = append(fields, "query", r.URL.RawQuery)
		}

		switch {
		case wrapped.statusCode >= 500:
			app.logger.Errorw("request completed with server error", fields...)
		case wrapped.statusCode >= 400:
			app.logger.Warnw("request completed with client error", fields...)
		default:
			app.logger.Infow("request completed", fields...)
		}
	})
}

func (app *Application) prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := newResponseWriter(w)

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start).Seconds()

		app.logger.Debugw("prometheus metrics",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration_seconds", duration,
		)
	})
}
