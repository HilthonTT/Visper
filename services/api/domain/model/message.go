package model

import "time"

type Message struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"room_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Encrypted bool      `json:"encrypted"`
	// ReplyToID is the message this one answers, if any.
	ReplyToID string `json:"reply_to_id,omitempty"`
	// Reactions holds at most one reaction per member.
	Reactions []Reaction `json:"reactions,omitempty"`
}

// Reaction is one member's reaction to a message. Like message content, the
// emoji is encrypted by the client, so the server can't read it.
type Reaction struct {
	UserID    string `json:"user_id"`
	Emoji     string `json:"emoji"`
	Encrypted bool   `json:"encrypted"`
}
