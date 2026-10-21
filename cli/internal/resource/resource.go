// Package resource holds endpoint configuration read from the environment.
package resource

import (
	"github.com/hilthontt/visper/cli/internal/env"
)

type resource struct {
	Api struct {
		Url string
	}
}

// Resource is populated from the environment at start-up.
var Resource resource

func init() {
	Resource.Api.Url = env.GetString("API_URL", "http://localhost:5004")
}
