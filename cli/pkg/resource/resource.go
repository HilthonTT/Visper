package resource

import (
	"github.com/hilthontt/visper/cli/pkg/env"
)

type resource struct {
	Api struct {
		Url string `json:"url"`
	}
}

var Resource resource

func init() {
	Resource.Api.Url = env.GetString("API_URL", "http://localhost:8080")
}
