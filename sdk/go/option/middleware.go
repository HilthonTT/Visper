package option

import (
	"log"
	"net/http"
	"net/http/httputil"
	"regexp"
)

var sensitiveHeaderRegex = regexp.MustCompile(`(?i)^(Authorization|Cookie|Set-Cookie|X-Api-Key): .+`)

func redactSensitiveHeaders(s string) string {
	return sensitiveHeaderRegex.ReplaceAllString(s, "$1: [REDACTED]")
}

func WithDebugLog(logger *log.Logger) RequestOption {
	if logger == nil {
		logger = log.Default()
	}

	return WithMiddleware(func(r *http.Request, next MiddlewareNext) (*http.Response, error) {
		if dump, err := httputil.DumpRequestOut(r, true); err == nil {
			logger.Printf("REQUEST:\n%s\n", redactSensitiveHeaders(string(dump)))
		}

		resp, err := next(r)

		if resp != nil {
			if dump, err := httputil.DumpResponse(resp, true); err == nil {
				logger.Printf("RESPONSE:\n%s\n", redactSensitiveHeaders(string(dump)))
			}
		}

		if err != nil {
			logger.Printf("REQUEST ERROR: %v", err)
		}

		return resp, err
	})
}
