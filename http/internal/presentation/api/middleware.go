package api

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/hilthontt/visper/internal/infrastructure/logging"
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

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("responseWriter does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := rw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return fmt.Errorf("responseWriter does not implement http.Pusher")
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

			app.logger.Warn(
				logging.General,
				logging.RateLimiting,
				"rate limit exceeded",
				map[logging.ExtraKey]any{
					"source": sourceKey,
					"path":   r.URL.Path,
					"method": r.Method,
				},
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

		extra := map[logging.ExtraKey]any{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status":      wrapped.statusCode,
			"duration_ms": duration.Milliseconds(),
			"bytes":       wrapped.bytes,
			"remote_addr": r.RemoteAddr,
			"user_agent":  r.UserAgent(),
			"client_ip":   r.RemoteAddr,
		}

		if r.URL.RawQuery != "" {
			extra["query"] = r.URL.RawQuery
		}

		switch {
		case wrapped.statusCode >= 500:
			app.logger.Error(logging.RequestResponse, logging.ExternalService, "request completed with server error", extra)
		case wrapped.statusCode >= 400:
			app.logger.Warn(logging.RequestResponse, logging.ExternalService, "request completed with client error", extra)
		default:
			app.logger.Info(logging.RequestResponse, logging.ExternalService, "request completed", extra)
		}
	})
}
func (app *Application) prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := newResponseWriter(w)

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start).Seconds()

		app.logger.Debug(
			logging.Prometheus,
			logging.ExternalService,
			"prometheus metrics",
			map[logging.ExtraKey]any{
				"method":           r.Method,
				"path":             r.URL.Path,
				"status_code":      wrapped.statusCode,
				"duration_seconds": duration,
			},
		)
	})
}
