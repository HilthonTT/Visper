package message

import "time"

type SendMessageRequest struct {
	Content   string `json:"content" binding:"required,max=1000"`
	Encrypted bool   `json:"encrypted"`
	ReplyToID string `json:"reply_to_id" binding:"omitempty,max=64"`
}

// SetReactionRequest sets the caller's reaction; the emoji is encrypted by
// the client like message content.
type SetReactionRequest struct {
	Emoji     string `json:"emoji" binding:"required,max=256"`
	Encrypted bool   `json:"encrypted"`
}

type ReactionResponse struct {
	UserID    string `json:"user_id"`
	Emoji     string `json:"emoji"`
	Encrypted bool   `json:"encrypted"`
}

type UpdateMessageRequest struct {
	Content   string `json:"content" binding:"required,max=1000"`
	Encrypted bool   `json:"encrypted"`
}

type MessageResponse struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	Encrypted bool      `json:"encrypted"`
	CreatedAt time.Time `json:"created_at"`
	ReplyToID string    `json:"reply_to_id,omitempty"`
	// Reactions are always present (possibly empty) so clients can replace
	// their list.
	Reactions []ReactionResponse `json:"reactions"`
}

type MessagesResponse struct {
	Messages []MessageResponse `json:"messages"`
	Count    int               `json:"count"`
	RoomID   string            `json:"room_id"`
}

type MessageCountResponse struct {
	RoomID string `json:"room_id"`
	Count  int64  `json:"count"`
}

type MessageUpdatedResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
	Encrypted bool   `json:"encrypted"`
}

type MessageDeletedResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id"`
}
