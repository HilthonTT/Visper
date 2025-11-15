package domain

import (
	"context"
	"time"
)

type Message struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"roomId"`
	User      *User     `json:"user"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

type MessageRepository interface {
	Create(ctx context.Context, message *Message) error
	GetByRoomID(ctx context.Context, roomID string) ([]Message, error)
	Delete(ctx context.Context, message *Message) error
}
