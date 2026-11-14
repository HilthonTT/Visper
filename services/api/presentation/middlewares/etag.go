package middlewares

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ETagMiddleware sets an ETag on JSON 200 responses and answers a matching
// If-None-Match with 304. The tag is computed from each response itself:
// responses differ per user, so a tag stored per path (as before) leaked
// between users, made If-Match fail spuriously and grew without bound.
func ETagMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if canSkipETag(c) {
			c.Next()
			return
		}

		ifNoneMatch := strings.Trim(c.GetHeader("If-None-Match"), "\"")

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
