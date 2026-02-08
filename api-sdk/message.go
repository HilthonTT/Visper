// api-sdk/message.go
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
	Options       []option.RequestOption
	encryptionKey string // Room encryption key (set when joining room)
}

func NewMessageService(opts ...option.RequestOption) *MessageService {
	m := &MessageService{Options: opts}
	return m
}

// SetEncryptionKey sets the room's encryption key for automatic encrypt/decrypt
func (m *MessageService) SetEncryptionKey(key string) {
	m.encryptionKey = key
}

// Send encrypts and sends a message to a room
func (m *MessageService) Send(ctx context.Context, roomID string, body SendMessageParams, opts ...option.RequestOption) (*MessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	// Encrypt the message content before sending
	if m.encryptionKey != "" {
		encryptedContent, err := EncryptWithKeyB64(body.Content, m.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("encryption failed: %w", err)
		}
		body.Content = encryptedContent
		body.Encrypted = true
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages", roomID)
	res := &MessageResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)
	if err != nil {
		return nil, err
	}

	// Decrypt response for convenience
	if m.encryptionKey != "" && res.Encrypted {
		decrypted, err := DecryptWithKeyB64(res.Content, m.encryptionKey)
		if err != nil {
			return res, fmt.Errorf("decryption failed: %w", err)
		}
		res.Content = decrypted
		res.Encrypted = false
	}

	return res, nil
}

// Update encrypts and updates a message
func (m *MessageService) Update(ctx context.Context, roomID, messageID string, body UpdateMessageParams, opts ...option.RequestOption) (*MessageUpdatedResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" || messageID == "" {
		return nil, ErrMissingIDParameter
	}

	// Encrypt before updating
	if m.encryptionKey != "" {
		encryptedContent, err := EncryptWithKeyB64(body.Content, m.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("encryption failed: %w", err)
		}
		body.Content = encryptedContent
		body.Encrypted = true
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/%s", roomID, messageID)
	res := &MessageUpdatedResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPut, path, body, &res, opts...)
	if err != nil {
		return nil, err
	}

	// Decrypt response
	if m.encryptionKey != "" && res.Encrypted {
		decrypted, err := DecryptWithKeyB64(res.Content, m.encryptionKey)
		if err != nil {
			return res, fmt.Errorf("decryption failed: %w", err)
		}
		res.Content = decrypted
		res.Encrypted = false
	}

	return res, nil
}

// List retrieves and decrypts messages from a room
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
	if err != nil {
		return nil, err
	}

	// Decrypt all messages
	if m.encryptionKey != "" {
		for i := range res.Messages {
			if res.Messages[i].Encrypted {
				decrypted, err := DecryptWithKeyB64(res.Messages[i].Content, m.encryptionKey)
				if err != nil {
					// Log but don't fail - show encrypted content
					res.Messages[i].Content = "[Decryption failed]"
					continue
				}
				res.Messages[i].Content = decrypted
				res.Messages[i].Encrypted = false
			}
		}
	}

	return res, nil
}

// ListAfter retrieves and decrypts messages after a specific timestamp
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
	if err != nil {
		return nil, err
	}

	if m.encryptionKey != "" {
		for i := range res.Messages {
			if res.Messages[i].Encrypted {
				decrypted, err := DecryptWithKeyB64(res.Messages[i].Content, m.encryptionKey)
				if err != nil {
					res.Messages[i].Content = "[Decryption failed]"
					continue
				}
				res.Messages[i].Content = decrypted
				res.Messages[i].Encrypted = false
			}
		}
	}

	return res, nil
}

func (m *MessageService) Delete(ctx context.Context, roomID, messageID string, opts ...option.RequestOption) (*MessageDeletedResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" || messageID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/%s", roomID, messageID)
	res := &MessageDeletedResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, &res, opts...)

	return res, err
}

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

type SendMessageParams struct {
	Content   string `json:"content"`
	Encrypted bool   `json:"encrypted,omitempty"`
}

func (r *SendMessageParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type UpdateMessageParams struct {
	Content   string `json:"content"`
	Encrypted bool   `json:"encrypted,omitempty"`
}

func (r *UpdateMessageParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

type MessageResponse struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	Encrypted bool      `json:"encrypted"`
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

type MessageUpdatedResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
	Encrypted bool   `json:"encrypted"`
}

func (r *MessageUpdatedResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MessageDeletedResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id"`
}

func (r *MessageDeletedResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MessageCountResponse struct {
	RoomID string `json:"room_id"`
	Count  int64  `json:"count"`
}

func (r *MessageCountResponse) UnmarshalJSON(data []byte) error {
	return apijson.UnmarshalRoot(data, r)
}

type MessageListParams struct {
	Limit int64 // Optional, defaults to 50 on server
}

type MessageListAfterParams struct {
	Timestamp time.Time
	Limit     int64 // Optional, defaults to 100 on server
}
