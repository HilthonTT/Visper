package apisdk

import (
	"context"
	"net/http"
	"os"
	"slices"

	"net/http/cookiejar"

	"github.com/hilthontt/visper/api-sdk/internal/requestconfig"
	"github.com/hilthontt/visper/api-sdk/option"
)

type Client struct {
	Options []option.RequestOption
	Room    *RoomService
	Message *MessageService
	Health  *HealthService
	AI      *AIService
	File    *FileService
}

func DefaultClientOptions() []option.RequestOption {
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{
		Jar: jar,
	}

	defaults := []option.RequestOption{
		option.WithEnvironmentDev(),
		option.WithHTTPClient(httpClient),
	}
	if o, ok := os.LookupEnv("VISPER_BASE_URL"); ok {
		defaults = append(defaults, option.WithBaseURL(o))
	} else {
		defaults = append(defaults, option.WithBaseURL("http://localhost:5005"))
	}

	return defaults
}

func NewClient(opts ...option.RequestOption) *Client {
	opts = append(DefaultClientOptions(), opts...)

	aiBaseURL := "http://localhost:8088"
	if u, ok := os.LookupEnv("VISPER_AI_BASE_URL"); ok {
		aiBaseURL = u
	}
	aiOpts := append(opts, option.WithBaseURL(aiBaseURL))

	r := &Client{
		Options: opts,
		Room:    NewRoomService(opts...),
		Message: NewMessageService(opts...),
		Health:  NewHealthService(opts...),
		AI:      NewAIService(aiOpts...),
		File:    NewFileService(opts...),
	}

	return r
}

func (c *Client) Execute(ctx context.Context, method, path string, params, res any, opts ...option.RequestOption) error {
	opts = slices.Concat(c.Options, opts)
	return requestconfig.ExecuteNewRequest(ctx, method, path, params, res, opts...)
}

func (c *Client) Get(ctx context.Context, path string, params, res any, opts ...option.RequestOption) error {
	return c.Execute(ctx, http.MethodGet, path, params, res, opts...)
}

func (c *Client) Post(ctx context.Context, path string, params, res any, opts ...option.RequestOption) error {
	return c.Execute(ctx, http.MethodPost, path, params, res, opts...)
}

func (c *Client) Put(ctx context.Context, path string, params, res any, opts ...option.RequestOption) error {
	return c.Execute(ctx, http.MethodPut, path, params, res, opts...)
}

func (c *Client) Delete(ctx context.Context, path string, params, res any, opts ...option.RequestOption) error {
	return c.Execute(ctx, http.MethodDelete, path, params, res, opts...)
}
