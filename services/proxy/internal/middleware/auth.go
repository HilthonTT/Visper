package middleware

import (
	"crypto/subtle"
	"net/http"
)

// RequireBearerToken protects a handler with a static bearer token. An empty
// token disables the handler entirely (404), so admin endpoints are never
// exposed unauthenticated by default.
func RequireBearerToken(token string) Middleware {
	want := []byte("Bearer " + token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				http.NotFound(w, r)
				return
			}
			got := []byte(r.Header.Get("Authorization"))
			if subtle.ConstantTimeCompare(got, want) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="proxy-admin"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
