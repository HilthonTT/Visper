package middlewares

import (
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"go.uber.org/zap"
)

func GinLogger(logger *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := redactQuery(c.Request.URL.RawQuery)

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		clientIP := c.ClientIP()

		if len(c.Errors) > 0 {
			logger.Error("Request error",
				zap.String("method", method),
				zap.String("path", path),
				zap.String("query", query),
				zap.Int("status", statusCode),
				zap.Duration("latency", latency),
				zap.String("ip", clientIP),
				zap.String("errors", c.Errors.String()),
			)
		} else {
			logger.Info("Request",
				zap.String("method", method),
				zap.String("path", path),
				zap.String("query", query),
				zap.Int("status", statusCode),
				zap.Duration("latency", latency),
				zap.String("ip", clientIP),
			)
		}
	}
}

// secretQueryParams are query parameters that carry room secrets.
var secretQueryParams = []string{"join_code", "secure_code", "secure_token"}

// redactQuery hides the values of secretQueryParams in a raw query string.
func redactQuery(raw string) string {
	if raw == "" {
		return raw
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "[unparseable query]"
	}
	redacted := false
	for _, k := range secretQueryParams {
		if _, ok := values[k]; ok {
			values.Set(k, "REDACTED")
			redacted = true
		}
	}
	if !redacted {
		return raw
	}
	return values.Encode()
}
