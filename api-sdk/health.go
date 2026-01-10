package apisdk

import (
	"context"
	"net/http"
	"slices"

	"github.com/hilthontt/visper/api-sdk/internal/apijson"
	"github.com/hilthontt/visper/api-sdk/internal/requestconfig"
	"github.com/hilthontt/visper/api-sdk/option"
)

type HealthService struct {
	Options []option.RequestOption
}

func NewHealthService(opts ...option.RequestOption) *HealthService {
	h := &HealthService{opts}
	return h
}

// Get retrieves the health status of the API
func (h *HealthService) Get(ctx context.Context, opts ...option.RequestOption) (*HealthResponse, error) {
	opts = slices.Concat(h.Options, opts)
	path := "health"

	res := &HealthResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

type HealthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

func (r *HealthResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}
