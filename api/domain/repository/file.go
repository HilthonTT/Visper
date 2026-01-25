package repository

import (
	"context"

	"github.com/hilthontt/visper/api/domain/model"
)

type FileRepository interface {
	Create(ctx context.Context, file *model.File) error
	GetByID(ctx context.Context, id string) (*model.File, error)
	GetByRoomID(ctx context.Context, roomID string) ([]*model.File, error)
	Delete(ctx context.Context, id string) error
	DeleteByRoomID(ctx context.Context, roomID string) error
	GetOrphanedFiles(ctx context.Context) ([]*model.File, error)
}
