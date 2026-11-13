package apisdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/hilthontt/visper/sdk/go/internal/apijson"
	"github.com/hilthontt/visper/sdk/go/internal/requestconfig"
	"github.com/hilthontt/visper/sdk/go/option"
)

type MessageService struct {
	Options []option.RequestOption

	mu            sync.RWMutex
	encryptionKey string // Room encryption key (set when joining room)
	sender        string // the current user's public ID, sealed into what we encrypt
}

func NewMessageService(opts ...option.RequestOption) *MessageService {
	m := &MessageService{Options: opts}
	return m
}

// SetEncryptionKey sets the room's encryption key for automatic
// encrypt/decrypt. Pass "" to disable encryption (e.g. after leaving a room).
// It is safe to call concurrently with the other methods.
func (m *MessageService) SetEncryptionKey(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.encryptionKey = key
}

// SetSender sets the current user's public ID (see UserIDFromToken). It is
// sealed into everything encrypted, so readers can check who wrote it, and
// must be set before sending to an encrypted room.
func (m *MessageService) SetSender(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sender = userID
}

func (m *MessageService) key() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.encryptionKey
}

func (m *MessageService) keyAndSender() (string, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.encryptionKey, m.sender
}

// Send encrypts and sends a message to a room
func (m *MessageService) Send(ctx context.Context, roomID string, body SendMessageParams, opts ...option.RequestOption) (*MessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	key, sender := m.keyAndSender()
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	// Encrypt the message content before sending
	if key != "" {
		encryptedContent, err := SealMessage(body.Content, sender, "", key)
		if err != nil {
			return nil, fmt.Errorf("encryption failed: %w", err)
		}
		body.Content = encryptedContent
		body.Encrypted = true
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages", url.PathEscape(roomID))
	res := &MessageResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)
	if err != nil {
		return nil, err
	}

	// Decrypt response for convenience
	if key != "" && res.Encrypted {
		decrypted, err := OpenMessage(res.Content, key, res.UserID, res.ID)
		if err != nil {
			return res, fmt.Errorf("decryption failed: %w", err)
		}
		res.Content = decrypted
		res.Encrypted = false
	}

	return res, nil
}

// SetReaction sets the current user's reaction to a message, replacing any
// previous one. The emoji is encrypted with the room key like message
// content. It returns the message with its new reactions, decrypted.
func (m *MessageService) SetReaction(ctx context.Context, roomID, messageID, emoji string, opts ...option.RequestOption) (*MessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	key, sender := m.keyAndSender()
	if roomID == "" || messageID == "" {
		return nil, ErrMissingIDParameter
	}

	body := SetReactionParams{Emoji: emoji}
	if key != "" {
		encrypted, err := SealReaction(emoji, sender, messageID, key)
		if err != nil {
			return nil, fmt.Errorf("encryption failed: %w", err)
		}
		body.Emoji, body.Encrypted = encrypted, true
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/%s/reaction", url.PathEscape(roomID), url.PathEscape(messageID))
	res := &MessageResponse{}
	if err := requestconfig.ExecuteNewRequest(ctx, http.MethodPut, path, body, &res, opts...); err != nil {
		return nil, err
	}
	decryptMessage(res, key)
	return res, nil
}

// RemoveReaction removes the current user's reaction to a message.
func (m *MessageService) RemoveReaction(ctx context.Context, roomID, messageID string, opts ...option.RequestOption) (*MessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	key := m.key()
	if roomID == "" || messageID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/%s/reaction", url.PathEscape(roomID), url.PathEscape(messageID))
	res := &MessageResponse{}
	if err := requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, &res, opts...); err != nil {
		return nil, err
	}
	decryptMessage(res, key)
	return res, nil
}

// decryptMessage decrypts a message's content and reactions in place,
// checking each against its sender (see OpenMessage). Failures are shown in
// place of the text rather than failing the call.
func decryptMessage(msg *MessageResponse, key string) {
	msg.Content, msg.Encrypted = decryptField(msg.Content, msg.Encrypted, key, func(ct string) (string, error) {
		return OpenMessage(ct, key, msg.UserID, msg.ID)
	})
	for i := range msg.Reactions {
		r := &msg.Reactions[i]
		r.Emoji, r.Encrypted = decryptField(r.Emoji, r.Encrypted, key, func(ct string) (string, error) {
			return OpenReaction(ct, key, r.UserID, msg.ID)
		})
	}
}

// decryptField decrypts one encrypted field with open; it reports whether
// the result is still encrypted (no key). In an encrypted room, plaintext
// can only have come from the server, so it is not shown as the sender's.
func decryptField(value string, encrypted bool, key string, open func(string) (string, error)) (string, bool) {
	if key == "" {
		return value, encrypted
	}
	if !encrypted {
		return FailedText(ErrUnverified), false
	}
	decrypted, err := open(value)
	if err != nil {
		return FailedText(err), false
	}
	return decrypted, false
}

// FailedText is what to show in place of content that failed OpenMessage or
// OpenReaction.
func FailedText(err error) string {
	if errors.Is(err, ErrUnverified) {
		return "[Unverified message]"
	}
	return "[Decryption failed]"
}

// Update encrypts and updates a message
func (m *MessageService) Update(ctx context.Context, roomID, messageID string, body UpdateMessageParams, opts ...option.RequestOption) (*MessageUpdatedResponse, error) {
	opts = slices.Concat(m.Options, opts)
	key, sender := m.keyAndSender()
	if roomID == "" || messageID == "" {
		return nil, ErrMissingIDParameter
	}

	// Encrypt before updating, bound to this message
	if key != "" {
		encryptedContent, err := SealMessage(body.Content, sender, messageID, key)
		if err != nil {
			return nil, fmt.Errorf("encryption failed: %w", err)
		}
		body.Content = encryptedContent
		body.Encrypted = true
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/%s", url.PathEscape(roomID), url.PathEscape(messageID))
	res := &MessageUpdatedResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodPut, path, body, &res, opts...)
	if err != nil {
		return nil, err
	}

	// Decrypt response
	if key != "" && res.Encrypted {
		decrypted, err := OpenEdit(res.Content, key, sender, messageID)
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
	key := m.key()
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages", url.PathEscape(roomID))
	if query.Limit > 0 {
		path += "?" + url.Values{"limit": {strconv.FormatInt(query.Limit, 10)}}.Encode()
	}

	res := &MessagesResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)
	if err != nil {
		return nil, err
	}

	for i := range res.Messages {
		decryptMessage(&res.Messages[i], key)
	}

	return res, nil
}

// ListAfter retrieves and decrypts messages after a specific timestamp
func (m *MessageService) ListAfter(ctx context.Context, roomID string, query MessageListAfterParams, opts ...option.RequestOption) (*MessagesResponse, error) {
	opts = slices.Concat(m.Options, opts)
	key := m.key()
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	// RFC3339 offsets contain "+", which must be escaped in a query string.
	// RFC3339Nano keeps sub-second precision (the server parses it as RFC3339);
	// truncating to seconds made ListAfter(lastMsg.CreatedAt) return lastMsg again.
	q := url.Values{"timestamp": {query.Timestamp.Format(time.RFC3339Nano)}}
	if query.Limit > 0 {
		q.Set("limit", strconv.FormatInt(query.Limit, 10))
	}
	path := fmt.Sprintf("api/v1/rooms/%s/messages/after?%s", url.PathEscape(roomID), q.Encode())

	res := &MessagesResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)
	if err != nil {
		return nil, err
	}

	for i := range res.Messages {
		decryptMessage(&res.Messages[i], key)
	}

	return res, nil
}

func (m *MessageService) Delete(ctx context.Context, roomID, messageID string, opts ...option.RequestOption) (*MessageDeletedResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" || messageID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/%s", url.PathEscape(roomID), url.PathEscape(messageID))
	res := &MessageDeletedResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, &res, opts...)

	return res, err
}

func (m *MessageService) Count(ctx context.Context, roomID string, opts ...option.RequestOption) (*MessageCountResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/count", url.PathEscape(roomID))
	res := &MessageCountResponse{}
	err := requestconfig.ExecuteNewRequest(ctx, http.MethodGet, path, nil, &res, opts...)

	return res, err
}

type SendMessageParams struct {
	Content   string `json:"content"`
	Encrypted bool   `json:"encrypted,omitempty"`
	// ReplyToID is the message this one answers, if any.
	ReplyToID string `json:"reply_to_id,omitempty"`
}

type SetReactionParams struct {
	Emoji     string `json:"emoji"`
	Encrypted bool   `json:"encrypted,omitempty"`
}

func (r *SetReactionParams) MarshalJSON() ([]byte, error) {
	return apijson.MarshalRoot(r)
}

// Reaction is one member's reaction to a message (one per member).
type Reaction struct {
	UserID    string `json:"user_id"`
	Emoji     string `json:"emoji"`
	Encrypted bool   `json:"encrypted"`
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
	ID        string     `json:"id"`
	RoomID    string     `json:"room_id"`
	UserID    string     `json:"user_id"`
	Username  string     `json:"username"`
	Content   string     `json:"content"`
	Encrypted bool       `json:"encrypted"`
	CreatedAt time.Time  `json:"created_at"`
	ReplyToID string     `json:"reply_to_id,omitempty"`
	Reactions []Reaction `json:"reactions,omitempty"`
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
