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

func (m *MessageService) Create(ctx context.Context, roomID string, body createMessageParam, opts ...option.RequestOption) (*createMessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	path := fmt.Sprintf("rooms/%s/messages", roomID)

	res := &createMessageResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)

	return res, err
}

func (m *MessageService) Delete(ctx context.Context, roomID, messageID string, opts ...option.RequestOption) error {
	opts = slices.Concat(m.Options, opts)
	path := fmt.Sprintf("rooms/%s/messages/%s", roomID, messageID)

	err := requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, nil, opts...)

	return err
}

type createMessageParam struct {
	RoomID  string `json:"roomId"`
	Content string `json:"content"`
}

type createMessageResponse struct {
	ID      string `json:"id"`
	RoomID  string `json:"roomId"`
	Content string `json:"content"`
}
