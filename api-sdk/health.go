package apisdk

import (
	"context"
	"net/http"
	"slices"

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

func (h *HealthService) Get(ctx context.Context, opts ...option.RequestOption) (*healthResponse, error) {
	opts = slices.Concat(h.Options, opts)
	path := "health"

	res := &healthResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

type healthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Uptime    string `json:"uptime"`
}
