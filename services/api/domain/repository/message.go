package repository

import (
	"context"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
)

type MessageRepository interface {
	GetByID(ctx context.Context, roomID, messageID string) (*model.Message, error)
	Update(ctx context.Context, message *model.Message) error
	// Mutate applies fn to a stored message and saves the result, keeping its
	// timestamps (it is not an edit). It returns the saved message.
	Mutate(ctx context.Context, roomID, messageID string, fn func(*model.Message) error) (*model.Message, error)
	Delete(ctx context.Context, roomID, messageID string) error
	Create(ctx context.Context, message *model.Message) error
	GetByRoom(ctx context.Context, roomID string, limit int64) ([]*model.Message, error)
	GetByRoomAfter(ctx context.Context, roomID string, after time.Time, limit int64) ([]*model.Message, error)
	Count(ctx context.Context, roomID string) (int64, error)
}
