package domain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/internal/infrastructure/validate"
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

func NewMessage(member *Member, rawContent string, roomID string) (*Message, error) {
	validateContent := validate.Compose(
		validate.Required(),
		validate.MinLength(1),
		validate.MaxLength(2000),
	)

	if err := validateContent(rawContent); err != nil {
		return nil, err
	}

	content := strings.TrimSpace(rawContent)
	if content == "" {
		return nil, fmt.Errorf("message cannot be empty or only whitespace")
	}

	now := time.Now().UTC()

	return &Message{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		User:      member.User,
		Content:   content,
		CreatedAt: now,
	}, nil
}
