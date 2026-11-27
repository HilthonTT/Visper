package repository

import (
	"context"

	"github.com/hilthontt/visper/services/api/domain/model"
)

// RoomCodes are the codes a joiner presented. An empty field is not checked.
type RoomCodes struct {
	JoinCode   string
	SecureCode string
}

type RoomRepository interface {
	Create(ctx context.Context, room *model.Room) error
	GetByID(ctx context.Context, id string) (*model.Room, error)
	GetAll(ctx context.Context) ([]*model.Room, error)
	// GetByJoinCode returns the room whose join code is joinCode, or
	// ErrNotFound.
	GetByJoinCode(ctx context.Context, joinCode string) (*model.Room, error)
	Delete(ctx context.Context, id string) error
	// AddUser adds user to the room, atomically checking that the room still
	// has the codes the joiner presented (an empty code is not checked). It
	// returns ErrNotFound if the room is gone and ErrCodesChanged if a code
	// was rotated, e.g. by a kick, since the caller read the room.
	AddUser(ctx context.Context, roomID string, user model.User, codes RoomCodes) error
	RemoveUser(ctx context.Context, roomID, userID string) error
	GetUsers(ctx context.Context, roomID string) ([]string, error)
	Update(ctx context.Context, room *model.Room) error
}
