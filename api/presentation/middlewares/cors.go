package middlewares

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/api/infrastructure/config"
)

const (
	corsAllowHeaders = "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-User-ID, If-Match, If-None-Match"
	corsAllowMethods = "POST, OPTIONS, GET, PUT, DELETE, PATCH, HEAD"
)

// CorsMiddleware applies the CORS policy from cfg.Cors.AllowOrigins, a comma
// separated list of origins or "*".
//
// Browsers reject "Access-Control-Allow-Origin: *" combined with credentials,
// so the wildcard is sent without Allow-Credentials. For an explicit list the
// request origin is echoed back (with credentials) only when it is allowed.
func CorsMiddleware(cfg *config.Config) gin.HandlerFunc {
	allowAll, allowed := parseAllowedOrigins(cfg.Cors.AllowOrigins)

	return func(c *gin.Context) {
		h := c.Writer.Header()
		origin := c.GetHeader("Origin")

		switch {
		case allowAll:
			h.Set("Access-Control-Allow-Origin", "*")
		case origin != "" && isOriginAllowed(origin, allowed):
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Add("Vary", "Origin")
		}

		h.Set("Access-Control-Allow-Headers", corsAllowHeaders)
		h.Set("Access-Control-Allow-Methods", corsAllowMethods)
		h.Set("Access-Control-Expose-Headers", "ETag, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func parseAllowedOrigins(list string) (allowAll bool, allowed map[string]struct{}) {
	allowed = make(map[string]struct{})
	for origin := range strings.SplitSeq(list, ",") {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		switch origin {
		case "":
		case "*":
			allowAll = true
		default:
			allowed[strings.ToLower(origin)] = struct{}{}
		}
	}
	return allowAll, allowed
}

func isOriginAllowed(origin string, allowed map[string]struct{}) bool {
	_, ok := allowed[strings.ToLower(strings.TrimRight(origin, "/"))]
	return ok
}
