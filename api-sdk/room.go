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

func (r *RoomService) New(ctx context.Context, body RoowNewParams, opts ...option.RequestOption) (*RoomNewResponse, error) {
	opts = slices.Concat(r.Options, opts)
	path := "api/rooms"

	res := &RoomNewResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

func (r *RoomService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*RoomResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/rooms/%s", id)
	res := &RoomResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

func (r *RoomService) Delete(ctx context.Context, id string, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/rooms/%s", id)
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)

	return err
}

func (r *RoomService) Join(ctx context.Context, joinOpts JoinRoomOpts, opts ...option.RequestOption) error {
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

	path := fmt.Sprintf("api/rooms/%s/join", joinOpts.RoomID)
	query := url.Values{}
	query.Set("joinCode", joinOpts.JoinCode)
	query.Set("username", joinOpts.Username)
	fullURL := fmt.Sprintf("%s?%s", path, query.Encode())

	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, fullURL, nil, nil, opts...)

	return err
}

func (r *RoomService) Leave(ctx context.Context, id string, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/rooms/%s/leave", id)
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, nil, nil, opts...)

	return err
}

func (r *RoomService) Boot(ctx context.Context, id string, body BootUserParams, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/rooms/%s/boot", id)
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, nil, opts...)

	return err
}

type RoowNewParams struct {
	Persistent bool   `json:"persistent"`
	Username   string `json:"username"`
}

func (r *RoowNewParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type RoomNewResponse struct {
	RoomID      string         `json:"roomId"`
	JoinCode    string         `json:"joinCode"`
	CreatedAt   time.Time      `json:"createdAt"`
	Persistent  bool           `json:"persistent"`
	Members     []UserResponse `json:"members"`
	MemberToken string         `json:"memberToken"`
}

func (r *RoomNewResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type JoinRoomOpts struct {
	RoomID   string
	JoinCode string
	Username string
}

type BootUserParams struct {
	MemberID string `json:"memberId"`
}

type UserResponse struct {
	ID   string `json:"id"`
	Name string `json:"name" example:"john_doe"`
}

type MessageResponse struct {
	ID        string       `json:"id"`
	User      UserResponse `json:"user"`
	Content   string       `json:"content"`
	CreatedAt time.Time    `json:"createdAt"`
}

type RoomResponse struct {
	ID         string            `json:"id"`
	JoinCode   string            `json:"joinCode"`
	Owner      UserResponse      `json:"owner"`
	Persistent bool              `json:"persistent"`
	CreatedAt  time.Time         `json:"createdAt"`
	Messages   []MessageResponse `json:"messages"`
	Members    []UserResponse    `json:"members"`
}
