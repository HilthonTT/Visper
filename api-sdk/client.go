package apisdk

import (
	"context"
	"net/http"
	"os"
	"slices"

	"github.com/hilthontt/visper/api-sdk/internal/requestconfig"
	"github.com/hilthontt/visper/api-sdk/option"
)

type Client struct {
	Options []option.RequestOption
	Room    *RoomService
}

func DefaultClientOptions() []option.RequestOption {
	defaults := []option.RequestOption{
		option.WithEnvironmentDev(),
	}
	if o, ok := os.LookupEnv("VISPER_BASE_URL"); ok {
		defaults = append(defaults, option.WithBaseURL(o))
	}
	return defaults
}

func NewClient(opts ...option.RequestOption) *Client {
	opts = append(DefaultClientOptions(), opts...)

	r := &Client{
		Options: opts,
		Room:    NewRoomService(opts...),
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
