package apisdk

import (
	"context"
	"net/http"
	"slices"

	"github.com/hilthontt/visper/api-sdk/internal/apijson"
	"github.com/hilthontt/visper/api-sdk/internal/requestconfig"
	"github.com/hilthontt/visper/api-sdk/option"
)

type AIService struct {
	Options []option.RequestOption
}

func NewAIService(opts ...option.RequestOption) *AIService {
	return &AIService{opts}
}

type AIEnhanceRequest struct {
	Message string `json:"message"`
	Style   string `json:"style"` // professional, casual, concise, friendly, formal
	Tone    string `json:"tone"`  // confident, polite, neutral, enthusiastic
}

func (r *AIEnhanceRequest) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type AIEnhanceResponse struct {
	Original     string   `json:"original"`
	Enhanced     string   `json:"enhanced"`
	Style        string   `json:"style"`
	Tone         string   `json:"tone"`
	Improvements []string `json:"improvements"`
	Cached       bool     `json:"cached"`
}

func (r *AIEnhanceResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

func (s *AIService) Enhance(ctx context.Context, body AIEnhanceRequest, opts ...option.RequestOption) (*AIEnhanceResponse, error) {
	opts = slices.Concat(s.Options, opts)
	res := &AIEnhanceResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, "api/v1/ai/enhance", body, res, opts...)
	return res, err
}

func (s *AIService) HealthCheck(ctx context.Context, opts ...option.RequestOption) (bool, error) {
	opts = slices.Concat(s.Options, opts)
	var result map[string]any
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, "api/v1/ai/health", nil, &result, opts...)
	if err != nil {
		return false, err
	}
	status, _ := result["status"].(string)
	return status == "healthy", nil
}
