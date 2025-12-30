package repository

import (
	"context"

	"github.com/hilthontt/visper/api/domain/model"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	SetUsernameIndex(ctx context.Context, username, userID string) error
	Delete(ctx context.Context, id string) error
}
