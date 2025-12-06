package tui

import (
	apisdk "github.com/hilthontt/visper/api-sdk"
	"github.com/hilthontt/visper/api-sdk/option"
)

func (m model) CreateSDKClient() *apisdk.Client {
	options := []option.RequestOption{}

	// TODO: Implement the rest
	return apisdk.NewClient(options...)
}
