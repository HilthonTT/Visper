package repository

import (
	"context"

	"github.com/hilthontt/visper/api/domain/model"
)

type RoomRepository interface {
	Create(ctx context.Context, room *model.Room) error
	GetByID(ctx context.Context, id string) (*model.Room, error)
	GetAll(ctx context.Context) ([]*model.Room, error)
	Delete(ctx context.Context, id string) error
	AddUser(ctx context.Context, roomID string, user model.User) error
	RemoveUser(ctx context.Context, roomID, userID string) error
	GetUsers(ctx context.Context, roomID string) ([]string, error)
	Update(ctx context.Context, room *model.Room) error
}
