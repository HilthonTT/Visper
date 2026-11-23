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

	mu             sync.RWMutex
	crypto         *RoomCrypto // the current room's keys (set when entering a room)
	allowPlaintext bool
}

// ErrNoRoomCrypto is returned by Send, Update and SetReaction when no room
// crypto state is set for the room (see SetRoomCrypto). Without it the
// content would go to the server unencrypted, so nothing is sent.
var ErrNoRoomCrypto = errors.New("no room key is set for this room; refusing to send unencrypted content")

func NewMessageService(opts ...option.RequestOption) *MessageService {
	m := &MessageService{Options: opts}
	return m
}

// SetRoomCrypto sets the current room's crypto state, which encrypts and
// signs what is sent and decrypts and verifies what is read. Pass nil to stop
// (e.g. after leaving a room). It is safe to call concurrently with the other
// methods.
func (m *MessageService) SetRoomCrypto(rc *RoomCrypto) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.crypto = rc
}

func (m *MessageService) roomCrypto() *RoomCrypto {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.crypto
}

// AllowPlaintext lets Send, Update and SetReaction send unencrypted content
// when no room crypto state is set. Off by default: forgetting SetRoomCrypto,
// or racing it with SetRoomCrypto(nil) when leaving a room, would otherwise
// disclose the content to the server. Only turn it on for rooms that are
// deliberately unencrypted.
func (m *MessageService) AllowPlaintext(allow bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowPlaintext = allow
}

// sealingCrypto returns the crypto state to seal content for roomID with. It
// fails closed: with no state, or state for another room, nothing may be
// sent (unless plaintext was allowed and no state is set). A nil result with
// a nil error means plaintext was allowed.
func (m *MessageService) sealingCrypto(roomID string) (*RoomCrypto, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	switch {
	case m.crypto == nil && m.allowPlaintext:
		return nil, nil
	case m.crypto == nil || m.crypto.roomID != roomID:
		return nil, ErrNoRoomCrypto
	}
	return m.crypto, nil
}

// Send encrypts and sends a message to a room
func (m *MessageService) Send(ctx context.Context, roomID string, body SendMessageParams, opts ...option.RequestOption) (*MessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" {
		return nil, ErrMissingIDParameter
	}
	rc, err := m.sealingCrypto(roomID)
	if err != nil {
		return nil, err
	}

	// Encrypt the message content before sending
	if rc != nil {
		encryptedContent, err := rc.SealMessage(body.Content, body.FileHash)
		if err != nil {
			return nil, fmt.Errorf("encryption failed: %w", err)
		}
		body.Content = encryptedContent
		body.Encrypted = true
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages", url.PathEscape(roomID))
	res := &MessageResponse{}
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPost, path, body, &res, opts...)
	if err != nil {
		return nil, err
	}

	// Decrypt response for convenience
	if rc != nil && !res.Encrypted {
		// In an encrypted room, plaintext can only have come from the server.
		res.Content = FailedText(ErrUnverified)
		return res, fmt.Errorf("decryption failed: %w", ErrUnverified)
	}
	if rc != nil {
		opened, err := rc.OpenMessage(res.Content, res.UserID, res.ID, res.CreatedAt, res.UpdatedAt)
		if err != nil {
			return res, fmt.Errorf("decryption failed: %w", err)
		}
		res.Content, res.FileHash, res.Encrypted = opened.Text, opened.FileHash, false
	}

	return res, nil
}

// SetReaction sets the current user's reaction to a message, replacing any
// previous one. The emoji is encrypted with the room key like message
// content. It returns the message with its new reactions, decrypted.
func (m *MessageService) SetReaction(ctx context.Context, roomID, messageID, emoji string, opts ...option.RequestOption) (*MessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" || messageID == "" {
		return nil, ErrMissingIDParameter
	}
	rc, err := m.sealingCrypto(roomID)
	if err != nil {
		return nil, err
	}

	body := SetReactionParams{Emoji: emoji}
	if rc != nil {
		encrypted, err := rc.SealReaction(messageID, emoji)
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
	DecryptMessage(res, rc)
	return res, nil
}

// RemoveReaction removes the current user's reaction to a message.
func (m *MessageService) RemoveReaction(ctx context.Context, roomID, messageID string, opts ...option.RequestOption) (*MessageResponse, error) {
	opts = slices.Concat(m.Options, opts)
	rc := m.roomCrypto()
	if roomID == "" || messageID == "" {
		return nil, ErrMissingIDParameter
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/%s/reaction", url.PathEscape(roomID), url.PathEscape(messageID))
	res := &MessageResponse{}
	if err := requestconfig.ExecuteNewRequest(ctx, http.MethodDelete, path, nil, &res, opts...); err != nil {
		return nil, err
	}
	DecryptMessage(res, rc)
	return res, nil
}

// DecryptMessage decrypts and verifies a message's content and reactions in
// place (see RoomCrypto.OpenMessage). Failures are shown in place of the
// text rather than returned. A nil rc leaves the message as it is.
func DecryptMessage(msg *MessageResponse, rc *RoomCrypto) {
	if rc == nil {
		return
	}
	if !msg.Encrypted {
		// In an encrypted room, plaintext can only have come from the server.
		msg.Content = FailedText(ErrUnverified)
	} else if opened, err := rc.OpenMessage(msg.Content, msg.UserID, msg.ID, msg.CreatedAt, msg.UpdatedAt); err != nil {
		msg.Content = FailedText(err)
	} else {
		msg.Content, msg.FileHash = opened.Text, opened.FileHash
	}
	msg.Encrypted = false
	for i := range msg.Reactions {
		r := &msg.Reactions[i]
		if !r.Encrypted {
			r.Emoji = FailedText(ErrUnverified)
		} else if emoji, err := rc.OpenReaction(r.Emoji, r.UserID, msg.ID); err != nil {
			r.Emoji = FailedText(err)
		} else {
			r.Emoji = emoji
		}
		r.Encrypted = false
	}
}

// FailedText is what to show in place of content that failed to decrypt or
// verify.
func FailedText(err error) string {
	switch {
	case errors.Is(err, ErrReplayed):
		return "[Replayed message]"
	case errors.Is(err, ErrUnverified):
		return "[Unverified message]"
	}
	return "[Decryption failed]"
}

// Update encrypts and updates a message
func (m *MessageService) Update(ctx context.Context, roomID, messageID string, body UpdateMessageParams, opts ...option.RequestOption) (*MessageUpdatedResponse, error) {
	opts = slices.Concat(m.Options, opts)
	if roomID == "" || messageID == "" {
		return nil, ErrMissingIDParameter
	}
	rc, err := m.sealingCrypto(roomID)
	if err != nil {
		return nil, err
	}

	// Encrypt before updating, bound to this message
	if rc != nil {
		encryptedContent, err := rc.SealEdit(messageID, body.Content)
		if err != nil {
			return nil, fmt.Errorf("encryption failed: %w", err)
		}
		body.Content = encryptedContent
		body.Encrypted = true
	}

	path := fmt.Sprintf("api/v1/rooms/%s/messages/%s", url.PathEscape(roomID), url.PathEscape(messageID))
	res := &MessageUpdatedResponse{}
	err = requestconfig.ExecuteNewRequest(ctx, http.MethodPut, path, body, &res, opts...)
	if err != nil {
		return nil, err
	}

	// Decrypt response
	if rc != nil && !res.Encrypted {
		// In an encrypted room, plaintext can only have come from the server.
		res.Content = FailedText(ErrUnverified)
		return res, fmt.Errorf("decryption failed: %w", ErrUnverified)
	}
	if rc != nil {
		opened, err := rc.OpenEdit(res.Content, rc.Identity().UserID(), messageID, time.Time{})
		if err != nil {
			return res, fmt.Errorf("decryption failed: %w", err)
		}
		res.Content = opened.Text
		res.Encrypted = false
	}

	return res, nil
}

// List retrieves and decrypts messages from a room
func (m *MessageService) List(ctx context.Context, roomID string, query MessageListParams, opts ...option.RequestOption) (*MessagesResponse, error) {
	opts = slices.Concat(m.Options, opts)
	rc := m.roomCrypto()
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
		DecryptMessage(&res.Messages[i], rc)
	}

	return res, nil
}

// ListAfter retrieves and decrypts messages after a specific timestamp
func (m *MessageService) ListAfter(ctx context.Context, roomID string, query MessageListAfterParams, opts ...option.RequestOption) (*MessagesResponse, error) {
	opts = slices.Concat(m.Options, opts)
	rc := m.roomCrypto()
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
		DecryptMessage(&res.Messages[i], rc)
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
	// FileHash is the hash of the encrypted file an image message points
	// to (see RoomCrypto.EncryptFile). It is sealed into the content, never
	// sent in the clear.
	FileHash string `json:"-"`
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
	UpdatedAt time.Time  `json:"updated_at"`
	ReplyToID string     `json:"reply_to_id,omitempty"`
	Reactions []Reaction `json:"reactions,omitempty"`
	// FileHash is the verified hash of the file an image message points to,
	// filled in by decryption (see RoomCrypto.DecryptFile).
	FileHash string `json:"-"`
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
