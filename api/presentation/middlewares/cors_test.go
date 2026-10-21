package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/infrastructure/config"
)

func TestCorsMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		allowOrigins    string
		origin          string
		method          string
		wantStatus      int
		wantAllowOrigin string
		wantCredentials string
	}{
		{"wildcard has no credentials", "*", "https://a.example", http.MethodGet, http.StatusOK, "*", ""},
		{"listed origin is echoed", "https://a.example,https://b.example", "https://b.example", http.MethodGet, http.StatusOK, "https://b.example", "true"},
		{"unlisted origin gets nothing", "https://a.example", "https://evil.example", http.MethodGet, http.StatusOK, "", ""},
		{"preflight short-circuits", "https://a.example", "https://a.example", http.MethodOptions, http.StatusNoContent, "https://a.example", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(CorsMiddleware(&config.Config{Cors: config.CorsConfig{AllowOrigins: tt.allowOrigins}}))
			r.Any("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(tt.method, "/x", nil)
			req.Header.Set("Origin", tt.origin)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tt.wantAllowOrigin {
				t.Errorf("Allow-Origin = %q, want %q", got, tt.wantAllowOrigin)
			}
			if got := w.Header().Get("Access-Control-Allow-Credentials"); got != tt.wantCredentials {
				t.Errorf("Allow-Credentials = %q, want %q", got, tt.wantCredentials)
			}
		})
	}
}

func TestETagMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(ETagMiddleware(NewInMemoryETagStore()))
	r.GET("/json", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"hello": "world"}) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/json", nil))

	etag := w.Header().Get("ETag")
	if w.Code != http.StatusOK || etag == "" || w.Body.Len() == 0 {
		t.Fatalf("first request: status=%d etag=%q body=%q", w.Code, etag, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	req.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotModified {
		t.Fatalf("conditional request: status = %d, want 304", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("304 response must not have a body, got %q", w.Body.String())
	}
}
