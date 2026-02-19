package main

import (
	"net/http"

	"github.com/hilthontt/visper/proxy/internal/throttling"
)

func ThrottlingMiddleware(throttler *throttling.Throttler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := r.Method + ":" + r.URL.Path
			if !throttler.IncrementAndCheck(route, 1) {
				http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func HierarchicalThrottlingMiddleware(ht *throttling.HierarchicalThrottler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !ht.CheckRequest(r) {
				http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
