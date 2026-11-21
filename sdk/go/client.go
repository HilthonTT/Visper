package apisdk

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"

	"net/http/cookiejar"

	"github.com/hilthontt/visper/sdk/go/internal/requestconfig"
	"github.com/hilthontt/visper/sdk/go/option"
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
		Jar:           jar,
		CheckRedirect: dropTokenOnRedirect,
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

// dropTokenOnRedirect keeps the user token from following a redirect to
// another host. The http.Client drops Authorization and cookies there, but
// forwards other headers, X-User-Token included, to wherever the redirect
// points.
func dropTokenOnRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		req.Header.Del(UserTokenHeader)
	}
	return nil
}

func NewClient(opts ...option.RequestOption) *Client {
	// Clip so that appending to opts (here or in a service) can never write into
	// a backing array shared with another service's options.
	opts = slices.Clip(slices.Concat(DefaultClientOptions(), opts))

	aiBaseURL := "http://localhost:8088"
	if u, ok := os.LookupEnv("VISPER_AI_BASE_URL"); ok {
		aiBaseURL = u
	}
	aiOpts := slices.Concat(opts, []option.RequestOption{option.WithBaseURL(aiBaseURL)})

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
