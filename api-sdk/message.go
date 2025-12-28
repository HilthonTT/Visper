package apisdk

import (
	"context"
	"fmt"
	"net/http"
	"slices"

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

func (m *MessageService) Create(ctx context.Context, roomID string, body CreateMessageParam, opts ...option.RequestOption) (*CreateMessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	path := fmt.Sprintf("api/rooms/%s/messages", roomID)

	res := &CreateMessageResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

func (m *MessageService) Delete(ctx context.Context, roomID, messageID string, opts ...option.RequestOption) error {
	opts = slices.Concat(m.Options, opts)
	path := fmt.Sprintf("api/rooms/%s/messages/%s", roomID, messageID)

	err := requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)

	return err
}

type CreateMessageParam struct {
	RoomID  string `json:"roomId"`
	Content string `json:"content"`
}

type CreateMessageResponse struct {
	ID      string `json:"id"`
	RoomID  string `json:"roomId"`
	Content string `json:"content"`
}
