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

type MessageService struct {
	Options []option.RequestOption
}

func NewMessageService(opts ...option.RequestOption) *MessageService {
	m := &MessageService{opts}
	return m
}

// Send sends a message to a room
func (m *MessageService) Send(ctx context.Context, roomID string, body SendMessageParams, opts ...option.RequestOption) (*MessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages", roomID)
	res := &MessageResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

// List retrieves messages from a room with optional limit
func (m *MessageService) List(ctx context.Context, roomID string, query MessageListParams, opts ...option.RequestOption) (*MessagesResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages", roomID)
	if query.Limit > 0 {
		path = fmt.Sprintf("%s?limit=%d", path, query.Limit)
	}

	res := &MessagesResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

// ListAfter retrieves messages after a specific timestamp
func (m *MessageService) ListAfter(ctx context.Context, roomID string, query MessageListAfterParams, opts ...option.RequestOption) (*MessagesResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/after?timestamp=%s", roomID, query.Timestamp.Format(time.RFC3339))
	if query.Limit > 0 {
		path = fmt.Sprintf("%s&limit=%d", path, query.Limit)
	}

	res := &MessagesResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

// Count retrieves the message count for a room
func (m *MessageService) Count(ctx context.Context, roomID string, opts ...option.RequestOption) (*MessageCountResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/count", roomID)
	res := &MessageCountResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

// Request/Response types

type SendMessageParams struct {
	Content string `json:"content"` // Max 1000 characters
}

func (r *SendMessageParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type MessageListParams struct {
	Limit int64 // Optional, defaults to 50 on server
}

type MessageListAfterParams struct {
	Timestamp time.Time
	Limit     int64 // Optional, defaults to 100 on server
}

type MessageResponse struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (r *MessageResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MessagesResponse struct {
	Messages []MessageResponse `json:"messages"`
	Count    int               `json:"count"`
	RoomID   string            `json:"room_id"`
}

func (r *MessagesResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MessageCountResponse struct {
	RoomID string `json:"room_id"`
	Count  int64  `json:"count"`
}

func (r *MessageCountResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}
