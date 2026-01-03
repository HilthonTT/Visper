package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type UserUseCase interface {
	GetOrCreateUser(ctx context.Context, id string) (*model.User, error)
	Create(ctx context.Context, username string) (*model.User, error)
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	UpdateUsername(ctx context.Context, userID string, newUsername string) error
	Delete(ctx context.Context, id string) error
	IsUsernameAvailable(ctx context.Context, username string) (bool, error)
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
	if id == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	// Try to get existing user
	user, err := uc.repository.GetByID(ctx, id)
	if err == nil {
		// User exists, return it
		return user, nil
	}

	// Check if error is "not found" (redis.Nil)
	if err == redis.Nil {
		// User doesn't exist, create a new one
		newUser := &model.User{
			ID:        id,
			Username:  generateAnonymousUsername(id),
			IsGuest:   true,
			CreatedAt: time.Now(),
		}

		if err := uc.repository.Create(ctx, newUser); err != nil {
			uc.logger.Error("failed to create user", zap.Error(err), zap.String("userID", id))
			return nil, fmt.Errorf("failed to create user: %w", err)
		}

		uc.logger.Info("created new user", zap.String("userID", newUser.ID), zap.String("username", newUser.Username))
		return newUser, nil
	}

	// Other error occurred
	uc.logger.Error("failed to get user by ID", zap.Error(err), zap.String("userID", id))
	return nil, fmt.Errorf("failed to get user: %w", err)

}

func (uc *userUseCase) Create(ctx context.Context, username string) (*model.User, error) {
	if err := uc.validateUsername(username); err != nil {
		return nil, err
	}

	available, err := uc.IsUsernameAvailable(ctx, username)
	if err != nil {
		uc.logger.Error("failed to check username availability", zap.Error(err), zap.String("username", username))
		return nil, fmt.Errorf("failed to verify username availability: %w", err)
	}

	if !available {
		return nil, fmt.Errorf("username '%s' is already taken", username)
	}

	user := &model.User{
		ID:        uuid.NewString(),
		Username:  username,
		CreatedAt: time.Now(),
	}

	if err := uc.repository.Create(ctx, user); err != nil {
		uc.logger.Error("failed to create user", zap.Error(err), zap.String("username", username))
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	if err := uc.repository.SetUsernameIndex(ctx, username, user.ID); err != nil {
		uc.logger.Error("failed to set username index", zap.Error(err), zap.String("username", username))
		_ = uc.repository.Delete(ctx, user.ID)
		return nil, fmt.Errorf("failed to index username: %w", err)
	}

	uc.logger.Info("user created successfully", zap.String("userID", user.ID), zap.String("username", username))
	return user, nil
}

func (uc *userUseCase) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	user, err := uc.repository.GetByID(ctx, id)
	if err != nil {
		uc.logger.Error("failed to get user by ID", zap.Error(err), zap.String("userID", id))
		return fmt.Errorf("user not found: %w", err)
	}

	if err := uc.repository.Delete(ctx, id); err != nil {
		uc.logger.Error("failed to delete user", zap.Error(err), zap.String("userID", id))
		return fmt.Errorf("failed to delete user: %w", err)
	}

	usernameIndexKey := fmt.Sprintf("user:username:%s", user.Username)
	_ = uc.repository.Delete(ctx, usernameIndexKey)

	uc.logger.Info("user deleted successfully", zap.String("userID", id), zap.String("username", user.Username))
	return nil
}

func (uc *userUseCase) GetByID(ctx context.Context, id string) (*model.User, error) {
	if id == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	user, err := uc.repository.GetByID(ctx, id)
	if err != nil {
		uc.logger.Error("failed to get user by ID", zap.Error(err), zap.String("userID", id))
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return user, nil
}

func (uc *userUseCase) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}

	user, err := uc.repository.GetByUsername(ctx, username)
	if err != nil {
		uc.logger.Error("failed to get user by username", zap.Error(err), zap.String("username", username))
		return nil, fmt.Errorf("username not found: %w", err)
	}

	return user, nil
}

func (uc *userUseCase) IsUsernameAvailable(ctx context.Context, username string) (bool, error) {
	if username == "" {
		return false, fmt.Errorf("username cannot be empty")
	}

	_, err := uc.repository.GetByUsername(ctx, username)
	if err != nil {
		// Username not found means it's available
		return true, nil
	}

	// Username exists
	return false, nil
}

func (uc *userUseCase) UpdateUsername(ctx context.Context, userID string, newUsername string) error {
	if userID == "" {
		return fmt.Errorf("user Id cannot be empty")
	}

	if err := uc.validateUsername(newUsername); err != nil {
		return err
	}

	user, err := uc.repository.GetByID(ctx, userID)
	if err != nil {
		uc.logger.Error("failed to get user by ID", zap.Error(err), zap.String("userID", userID))
		return fmt.Errorf("user not found: %w", err)
	}

	if user.Username == newUsername {
		return nil // no change needed
	}

	available, err := uc.IsUsernameAvailable(ctx, newUsername)
	if err != nil {
		return fmt.Errorf("failed to verify username availability: %w", err)
	}

	if !available {
		return fmt.Errorf("username '%s' is already taken", newUsername)
	}

	oldUsername := user.Username
	user.Username = newUsername

	if err := uc.repository.Create(ctx, user); err != nil {
		uc.logger.Error("failed to update user", zap.Error(err), zap.String("userID", userID))
		return fmt.Errorf("failed to update username: %w", err)
	}

	if err := uc.repository.SetUsernameIndex(ctx, newUsername, userID); err != nil {
		uc.logger.Error("failed to set new username index", zap.Error(err), zap.String("userID", userID), zap.String("newUsername", newUsername))
		// Rollback user update
		user.Username = oldUsername
		_ = uc.repository.Create(ctx, user)
		return fmt.Errorf("failed to index new username: %w", err)
	}

	// Remove old username index
	oldIndexKey := fmt.Sprintf("user:username:%s", oldUsername)
	_ = uc.repository.Delete(ctx, oldIndexKey)

	uc.logger.Info("username updated successfully", zap.String("userID", userID), zap.String("oldUsername", oldUsername), zap.String("newUsername", newUsername))
	return nil
}

func (uc *userUseCase) validateUsername(username string) error {
	username = strings.TrimSpace(username)

	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	if len(username) < 3 {
		return fmt.Errorf("username must be at least 3 characters long")
	}

	if len(username) > 20 {
		return fmt.Errorf("username must be at most 20 characters long")
	}

	// Check for valid characters (alphanumeric, underscore, hyphen)
	for _, char := range username {
		if !isValidUsernameChar(char) {
			return fmt.Errorf("username can only contain letters, numbers, underscores, and hyphens")
		}
	}

	firstChar := rune(username[0])
	if !isAlphanumeric(firstChar) {
		return fmt.Errorf("username must start with a letter or number")
	}

	return nil
}

func isValidUsernameChar(char rune) bool {
	return isAlphanumeric(char) || char == '_' || char == '-'
}

func isAlphanumeric(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9')
}

func generateAnonymousUsername(userID string) string {
	// Use first 8 characters of UUID for uniqueness
	if len(userID) >= 8 {
		return fmt.Sprintf("Guest-%s", userID[:8])
	}
	return fmt.Sprintf("Guest-%s", userID)
}
