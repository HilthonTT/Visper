package repository

import (
	"context"
	"time"

	"github.com/hilthontt/visper/api/domain/model"
)

type MessageRepository interface {
	GetByID(ctx context.Context, roomID, messageID string) (*model.Message, error)
	Update(ctx context.Context, message *model.Message) error
	Delete(ctx context.Context, roomID, messageID string) error
	Create(ctx context.Context, message *model.Message) error
	GetByRoom(ctx context.Context, roomID string, limit int64) ([]*model.Message, error)
	GetByRoomAfter(ctx context.Context, roomID string, after time.Time, limit int64) ([]*model.Message, error)
	DeleteOldMessages(ctx context.Context, roomID string, before time.Time) error
	Count(ctx context.Context, roomID string) (int64, error)
}
