package websocket

import (
	"net/http"
	"net/url"
	"strings"
)

// NewOriginChecker returns a CheckOrigin function for websocket upgrades.
//
// allowedOrigins is the comma separated CORS allow list from the config.
// "*" allows every origin. Requests without an Origin header (non-browser
// clients such as the CLI) are always allowed, and so are same-origin
// requests. Everything else must match the allow list, which prevents
// cross-site websocket hijacking with the user's cookies.
func NewOriginChecker(allowedOrigins string) func(r *http.Request) bool {
	allowAll := false
	allowed := make(map[string]struct{})

	for origin := range strings.SplitSeq(allowedOrigins, ",") {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin == "" {
			continue
		}
		if origin == "*" {
			allowAll = true
			continue
		}
		allowed[strings.ToLower(origin)] = struct{}{}
	}

	return func(r *http.Request) bool {
		if allowAll {
			return true
		}

		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}

		if _, ok := allowed[strings.ToLower(strings.TrimRight(origin, "/"))]; ok {
			return true
		}

		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	}
}
