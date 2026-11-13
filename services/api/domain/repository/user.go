package repository

import (
	"context"

	"github.com/hilthontt/visper/services/api/domain/model"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	// Update saves changes to an existing user, keeping its creation time.
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id string) error
}
