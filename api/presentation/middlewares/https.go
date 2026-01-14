package middlewares

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/infrastructure/config"
)

func ForceHttps(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.TLS != nil {
			c.Next()
			return
		}

		if c.GetHeader("X-Forwarded-Proto") == "https" {
			c.Next()
			return
		}

		host := c.Request.Host
		if cfg.Server.InternalPort != cfg.Server.ExternalPort {
			host = strings.Replace(host, fmt.Sprintf(":%s", cfg.Server.InternalPort), fmt.Sprintf(":%s", cfg.Server.ExternalPort), 1)
		}

		c.Redirect(http.StatusMovedPermanently, "https://"+host+c.Request.RequestURI)
		c.Abort()
	}
}
