package message

import "time"

type SendMessageRequest struct {
	Content   string `json:"content" binding:"required,max=1000"`
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

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
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
