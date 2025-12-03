package apisdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/hilthontt/visper/api-sdk/internal/apijson"
	"github.com/hilthontt/visper/api-sdk/internal/requestconfig"
	"github.com/hilthontt/visper/api-sdk/option"
)

type RoomService struct {
	Options []option.RequestOption
}

func NewRoomService(opts ...option.RequestOption) *RoomService {
	r := &RoomService{opts}
	return r
}

func (r *RoomService) New(ctx context.Context, body roomNewParams, opts ...option.RequestOption) (*roomNewResponse, error) {
	opts = slices.Concat(r.Options, opts)
	path := "rooms"

	res := &roomNewResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

func (r *RoomService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*roomNewResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("/rooms/%s", id)
	res := &roomNewResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

func (r *RoomService) Join(ctx context.Context, joinOpts joinRoomOpts, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if joinOpts.RoomID == "" {
		return ErrMissingIDParameter
	}
	if joinOpts.JoinCode == "" {
		return ErrMissingJoinCodeParameter
	}
	if joinOpts.Username == "" {
		return ErrMissingUsername
	}

	path := fmt.Sprintf("/rooms/%s/join", joinOpts.RoomID)
	query := url.Values{}
	query.Set("joinCode", joinOpts.JoinCode)
	query.Set("username", joinOpts.Username)
	fullURL := fmt.Sprintf("%s?%s", path, query.Encode())

	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, fullURL, nil, nil, opts...)

	return err
}

func (r *RoomService) Boot(ctx context.Context, id string, body bootUserParams, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return ErrMissingIDParameter
	}

	path := fmt.Sprintf("/rooms/%s/boot", id)
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, nil, opts...)

	return err
}

type roomNewParams struct {
	Persistent bool   `json:"persistent"`
	Username   string `json:"username"`
}

func (r roomNewParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type roomNewResponse struct {
	RoomID     string    `json:"roomId"`
	JoinCode   string    `json:"joinCode"`
	CreatedAt  time.Time `json:"createdAt"`
	Persistent bool      `json:"persistent"`
}

func (r roomNewResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type joinRoomOpts struct {
	RoomID   string
	JoinCode string
	Username string
}

type bootUserParams struct {
	MemberID string `json:"memberId"`
}
