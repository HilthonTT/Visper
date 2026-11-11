// Package resource holds endpoint configuration read from the environment.
package resource

import (
	"github.com/hilthontt/visper/clients/cli/internal/env"
)

type resource struct {
	Api struct {
		Url string
	}
}

// Resource is populated from the environment at start-up.
var Resource resource

// DefaultAPIURL is the API's own port. The optional proxy listens on :5004;
// point VISPER_BASE_URL at it to go through the proxy.
const DefaultAPIURL = "http://localhost:5005"

func init() {
	Resource.Api.Url = apiURL()
}

// apiURL reads VISPER_BASE_URL (the documented variable, shared with the SDK),
// then the legacy API_URL, then falls back to DefaultAPIURL.
// Empty values count as unset.
func apiURL() string {
	for _, key := range []string{"VISPER_BASE_URL", "API_URL"} {
		if v := env.GetString(key, ""); v != "" {
			return v
		}
	}
	return DefaultAPIURL
}
