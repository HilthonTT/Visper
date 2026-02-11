package middlewares

import (
	"bytes"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/infrastructure/cache"
)

type ResponseCache struct {
	cache *cache.Cache
}

type CachedResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

type responseRecorder struct {
	gin.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

// newResponseRecorder creates a new response recorder
func newResponseRecorder(w gin.ResponseWriter) *responseRecorder {
	return &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           &bytes.Buffer{},
	}
}

func NewResponseCache() *ResponseCache {
	options := cache.DefaultOptions()
	options.CleanupInterval = 5 * time.Minute

	return &ResponseCache{
		cache: cache.NewCache(options),
	}
}

// Middleware creates a caching middleware
func (rc *ResponseCache) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		r := c.Request

		if r.Method != http.MethodGet {
			c.Next()
			return
		}

		// Create a cache key from the request
		key := r.URL.String()

		// Check if we have a cached response
		if cachedResp, found := rc.cache.Get(key); found {
			resp := cachedResp.(*CachedResponse)

			// Set headers
			for k, v := range resp.Headers {
				c.Writer.Header().Set(k, v)
			}

			// Write status code and body
			c.Writer.WriteHeader(resp.StatusCode)
			c.Writer.Write(resp.Body)
			return
		}

		// Create a response recorder
		rr := newResponseRecorder(c.Writer)
		c.Writer = rr

		// Call the next handler with rr
		c.Next()

		// Only cache successful responses
		if rr.statusCode >= http.StatusOK && rr.statusCode < http.StatusMultipleChoices {
			resp := &CachedResponse{
				StatusCode: rr.statusCode,
				Headers:    make(map[string]string),
				Body:       rr.body.Bytes(),
			}

			// Copy headers
			for k, v := range rr.Header() {
				if len(v) > 0 {
					resp.Headers[k] = v[0]
				}
			}

			// Store in cache with TTL
			rc.cache.Set(key, resp, 5*time.Minute)
		}
	}
}
