package apisdk

import (
	"context"
	"fmt"
	"net/http"
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

// Create creates a new room with specified expiry hours
func (r *RoomService) Create(ctx context.Context, body RoomCreateParams, opts ...option.RequestOption) (*RoomResponse, error) {
	opts = slices.Concat(r.Options, opts)
	path := "api/v1/rooms"

	res := &RoomResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

// Get retrieves a room by ID
func (r *RoomService) Get(ctx context.Context, id string, opts ...option.RequestOption) (*RoomResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s", id)
	res := &RoomResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

// Delete deletes a room (only owner can delete)
func (r *RoomService) Delete(ctx context.Context, id string, opts ...option.RequestOption) error {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s", id)
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)

	return err
}

// GetByJoinCode retrieves a room by join code and joins the user to it
func (r *RoomService) GetByJoinCode(ctx context.Context, body JoinByCodeParams, opts ...option.RequestOption) (*RoomResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if body.JoinCode == "" {
		return nil, ErrMissingJoinCodeParameter
	}

	path := "api/v1/rooms/join-code"
	res := &RoomResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

// Join joins an existing room by room ID
func (r *RoomService) Join(ctx context.Context, id string, body JoinRoomParams, opts ...option.RequestOption) (*SuccessResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/join", id)
	res := &SuccessResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

// Leave leaves a room
func (r *RoomService) Leave(ctx context.Context, id string, opts ...option.RequestOption) (*SuccessResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/leave", id)
	res := &SuccessResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, nil, &res, opts...)

	return res, err
}

// CheckMembership checks if the current user is a member of a room
func (r *RoomService) CheckMembership(ctx context.Context, id string, opts ...option.RequestOption) (*MembershipResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if id == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/membership", id)
	res := &MembershipResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

// KickMember kicks a member from the room (only owner can kick)
func (r *RoomService) KickMember(ctx context.Context, roomID, userID string, opts ...option.RequestOption) (*SuccessResponse, error) {
	opts = slices.Concat(r.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}
	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	path := fmt.Sprintf("api/v1/rooms/%s/membership/%s", roomID, userID)
	res := &SuccessResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, nil, &res, opts...)

	return res, err
}

// Request/Response types

type RoomCreateParams struct {
	ExpiryHours int `json:"expiry_hours"` // 1 to 168 hours (1 hour to 7 days)
}

func (r *RoomCreateParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type JoinByCodeParams struct {
	JoinCode string `json:"join_code"` // 6-character join code
	Username string `json:"username,omitempty"`
}

func (r *JoinByCodeParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type JoinRoomParams struct {
	Username string `json:"username,omitempty"`
}

func (r *JoinRoomParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type RoomResponse struct {
	ID          string         `json:"id"`
	JoinCode    string         `json:"join_code"`
	Owner       UserResponse   `json:"owner"`
	CreatedAt   time.Time      `json:"created_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
	Members     []UserResponse `json:"members"`
	CurrentUser UserResponse   `json:"current_user"`
}

func (r *RoomResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type SuccessResponse struct {
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

func (r *SuccessResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MembershipResponse struct {
	IsMember bool   `json:"is_member"`
	RoomID   string `json:"room_id"`
	UserID   string `json:"user_id,omitempty"`
}

func (r *MembershipResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}
