package middlewares

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var concurrencyCheckMethods = []string{http.MethodPut, http.MethodPatch}

type ETagStore interface {
	GetETag(resourceURI string) string
	SetETag(resourceURI, etag string)
}

type InMemoryETagStore struct {
	mu    sync.RWMutex
	store map[string]string
}

func NewInMemoryETagStore() ETagStore {
	return &InMemoryETagStore{
		store: make(map[string]string),
	}
}

func (s *InMemoryETagStore) GetETag(resourceURI string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store[resourceURI]
}

func (s *InMemoryETagStore) SetETag(resourceURI, etag string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[resourceURI] = etag
}

func ETagMiddleware(store ETagStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if canSkipETag(c) {
			c.Next()
			return
		}

		resourceURI := c.Request.URL.Path
		ifNoneMatch := strings.Trim(c.GetHeader("If-None-Match"), "\"")
		ifMatch := strings.Trim(c.GetHeader("If-Match"), "\"")

		if containsMethod(concurrencyCheckMethods, c.Request.Method) && ifMatch != "" {
			currentETag := store.GetETag(resourceURI)
			if currentETag != "" && ifMatch != currentETag {
				c.AbortWithStatus(http.StatusPreconditionFailed)
				return
			}
		}

		// The body is buffered so that the ETag header can still be set and a
		// 304 returned after the handler ran. Previously the body was streamed
		// straight through, which made both impossible.
		original := c.Writer
		writer := &bufferedWriter{ResponseWriter: original}
		c.Writer = writer

		c.Next()

		c.Writer = original
		body := writer.body.Bytes()

		if len(body) > 0 && isETaggableResponse(original) {
			etag := generateETag(body)

			store.SetETag(resourceURI, etag)
			original.Header().Set("ETag", "\""+etag+"\"")

			if c.Request.Method == http.MethodGet && ifNoneMatch == etag {
				original.Header().Del("Content-Length")
				original.WriteHeader(http.StatusNotModified)
				original.WriteHeaderNow()
				return
			}
		}

		if len(body) > 0 {
			_, _ = original.Write(body)
		}
	}
}

// bufferedWriter captures the response body instead of sending it. Status
// codes and headers still go to the underlying writer (gin only records them
// until the first body write), and Hijack/Flush are delegated so websocket
// upgrades keep working.
type bufferedWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *bufferedWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *bufferedWriter) WriteString(s string) (int, error) {
	return w.body.WriteString(s)
}

func isETaggableResponse(w gin.ResponseWriter) bool {
	contentType := w.Header().Get("Content-Type")
	return w.Status() == http.StatusOK &&
		strings.Contains(strings.ToLower(contentType), "json")
}

func generateETag(content []byte) string {
	hash := sha512.Sum512(content)
	return hex.EncodeToString(hash[:])
}

func canSkipETag(c *gin.Context) bool {
	return c.Request.Method == http.MethodPost ||
		c.Request.Method == http.MethodDelete ||
		strings.EqualFold(c.GetHeader("Upgrade"), "websocket")
}

func containsMethod(methods []string, method string) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}
