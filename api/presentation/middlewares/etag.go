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

		writer := &responseWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = writer

		c.Next()

		if isETaggableResponse(c) {
			responseBody := writer.body.Bytes()
			etag := generateETag(responseBody)

			store.SetETag(resourceURI, etag)
			c.Header("ETag", "\""+etag+"\"")

			if c.Request.Method == http.MethodGet && ifNoneMatch == etag {
				c.Status(http.StatusNotModified)
				c.Writer = &emptyResponseWriter{ResponseWriter: c.Writer}
				return
			}
		}
	}
}

type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

type emptyResponseWriter struct {
	gin.ResponseWriter
}

func (w *emptyResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func isETaggableResponse(c *gin.Context) bool {
	contentType := c.Writer.Header().Get("Content-Type")
	return c.Writer.Status() == http.StatusOK &&
		strings.Contains(strings.ToLower(contentType), "json")
}

func generateETag(content []byte) string {
	hash := sha512.Sum512(content)
	return hex.EncodeToString(hash[:])
}

func canSkipETag(c *gin.Context) bool {
	return c.Request.Method == http.MethodPost || c.Request.Method == http.MethodDelete
}

func containsMethod(methods []string, method string) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}
