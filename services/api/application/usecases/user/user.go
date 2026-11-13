package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hilthontt/visper/services/api/domain/apperr"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"go.uber.org/zap"
)

var (
	ErrUserIDRequired = apperr.New(apperr.Invalid, "user ID cannot be empty")
	ErrUserNotFound   = apperr.New(apperr.NotFound, "user not found")
)

// UserUseCase manages anonymous users. Their IDs are derived from private
// user tokens (X-User-Token header or session cookie) by the user middleware.
type UserUseCase interface {
	// GetOrCreateUser returns the user with the given ID, creating a guest
	// user for an ID seen for the first time.
	GetOrCreateUser(ctx context.Context, id string) (*model.User, error)
	GetByID(ctx context.Context, id string) (*model.User, error)
	// Rename stores a new display name for user; an empty or unchanged name
	// is a no-op.
	Rename(ctx context.Context, user *model.User, username string) error
}

type userUseCase struct {
	repository repository.UserRepository
	logger     *logger.Logger
}

func NewUserUseCase(repository repository.UserRepository, logger *logger.Logger) UserUseCase {
	return &userUseCase{
		repository: repository,
		logger:     logger,
	}
}

func (uc *userUseCase) GetOrCreateUser(ctx context.Context, id string) (*model.User, error) {
	user, err := uc.GetByID(ctx, id)
	if err == nil {
		uc.keepAlive(ctx, user)
		return user, nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}

	user = &model.User{
		ID:        id,
		Username:  guestUsername(id),
		IsGuest:   true,
		CreatedAt: time.Now(),
	}
	if err := uc.repository.Create(ctx, user); err != nil {
		uc.logger.Error("failed to create user", zap.Error(err), zap.String("userID", id))
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	uc.logger.Info("created new user", zap.String("userID", user.ID), zap.String("username", user.Username))
	return user, nil
}

func (uc *userUseCase) GetByID(ctx context.Context, id string) (*model.User, error) {
	if id == "" {
		return nil, ErrUserIDRequired
	}

	user, err := uc.repository.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		uc.logger.Error("failed to get user by ID", zap.Error(err), zap.String("userID", id))
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

func (uc *userUseCase) Rename(ctx context.Context, user *model.User, username string) error {
	if username == "" || username == user.Username {
		return nil
	}
	renamed := *user
	renamed.Username = username
	if err := uc.repository.Update(ctx, &renamed); err != nil {
		uc.logger.Error("failed to rename user", zap.Error(err), zap.String("userID", user.ID))
		return fmt.Errorf("failed to rename user: %w", err)
	}
	user.Username = username
	return nil
}

// keepAlive renews the expiry of an active user's record, at most once per
// refreshInterval so that requests don't each write to Redis.
func (uc *userUseCase) keepAlive(ctx context.Context, user *model.User) {
	if time.Since(user.SeenAt) < refreshInterval {
		return
	}
	if err := uc.repository.Update(ctx, user); err != nil {
		// Not fatal: the record is still valid until it expires.
		uc.logger.Warn("failed to refresh user", zap.Error(err), zap.String("userID", user.ID))
	}
}

// refreshInterval is how often an active user's record expiry is renewed.
const refreshInterval = 24 * time.Hour

// guestUsername derives a readable name from the start of the user's ID.
func guestUsername(userID string) string {
	return "Guest-" + userID[:min(len(userID), 8)]
}
