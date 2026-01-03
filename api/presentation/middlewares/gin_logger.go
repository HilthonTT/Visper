package middlewares

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"go.uber.org/zap"
)

func GinLogger(logger *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

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
