package room

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type RoomUseCase interface {
	Create(ctx context.Context, owner model.User, expiry time.Duration) (*model.Room, error)
	GetByID(ctx context.Context, id string) (*model.Room, error)
	GetByJoinCode(ctx context.Context, joinCode string) (*model.Room, error)
	Delete(ctx context.Context, id string, userID string) error
	JoinRoom(ctx context.Context, roomID string, user model.User) error
	LeaveRoom(ctx context.Context, roomID string, userID string) error
	IsUserInRoom(ctx context.Context, roomID string, userID string) (bool, error)
}

type roomUseCase struct {
	repository repository.RoomRepository
	logger     *logger.Logger
}

func NewRoomUseCase(repository repository.RoomRepository, logger *logger.Logger) RoomUseCase {
	return &roomUseCase{
		repository: repository,
		logger:     logger,
	}
}

func (uc *roomUseCase) Create(ctx context.Context, owner model.User, expiry time.Duration) (*model.Room, error) {
	room := &model.Room{
		ID:        uuid.NewString(),
		JoinCode:  generateJoinCode(),
		Owner:     owner,
		CreatedAt: time.Now(),
		Expiry:    expiry,
		Members:   []model.User{owner}, // Add the owner as a member for the room (as he technically is)
	}

	if err := uc.repository.Create(ctx, room); err != nil {
		uc.logger.Error("failed to create room", zap.Error(err), zap.String("ownerID", owner.ID))
		return nil, fmt.Errorf("failed to create room: %w", err)
	}

	if err := uc.repository.AddUser(ctx, room.ID, owner); err != nil {
		uc.logger.Error("failed to add owner to room", zap.Error(err), zap.String("roomID", room.ID), zap.String("ownerID", owner.ID))
		// Attempt cleanup
		_ = uc.repository.Delete(ctx, room.ID)
		return nil, fmt.Errorf("failed to add owner to room: %w", err)
	}

	uc.logger.Info("room created successfully", zap.String("roomID", room.ID))
	return room, nil
}

func (uc *roomUseCase) Delete(ctx context.Context, id string, userID string) error {
	if id == "" {
		return fmt.Errorf("room ID cannot be empty")
	}

	room, err := uc.repository.GetByID(ctx, id)
	if err != nil {
		uc.logger.Error("failed to get room for deletion", zap.Error(err), zap.String("roomID", id))
		return fmt.Errorf("failed to get room: %w", err)
	}

	if room == nil {
		return fmt.Errorf("room not found")
	}

	// Only owner can delete the room
	if room.Owner.ID != userID {
		uc.logger.Warn("unauthorized room deletion attempt", zap.String("roomID", id), zap.String("userID", userID), zap.String("ownerID", room.Owner.ID))
		return fmt.Errorf("only the room owner can delete the room")
	}

	if err := uc.repository.Delete(ctx, id); err != nil {
		uc.logger.Error("failed to delete room", zap.Error(err), zap.String("roomID", id))
		return fmt.Errorf("failed to delete room: %w", err)
	}

	uc.logger.Info("room deleted successfully", zap.String("roomID", id), zap.String("ownerID", userID))
	return nil
}

func (uc *roomUseCase) GetByID(ctx context.Context, id string) (*model.Room, error) {
	if id == "" {
		return nil, fmt.Errorf("room ID cannot be empty")
	}

	room, err := uc.repository.GetByID(ctx, id)
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("room not found")
		}
		uc.logger.Error("failed to get room by ID", zap.Error(err), zap.String("roomID", id))
		return nil, fmt.Errorf("failed to get room: %w", err)
	}

	if room == nil {
		return nil, fmt.Errorf("room not found")
	}

	if uc.isRoomExpired(room) {
		uc.logger.Info("room has expired, deleting", zap.String("roomID", room.ID))
		_ = uc.repository.Delete(ctx, room.ID)
		return nil, fmt.Errorf("room has expired")
	}

	return room, nil
}

func (uc *roomUseCase) GetByJoinCode(ctx context.Context, joinCode string) (*model.Room, error) {
	if joinCode == "" {
		return nil, fmt.Errorf("join code cannot be empty")
	}

	rooms, err := uc.repository.GetAll(ctx)
	if err != nil {
		uc.logger.Error("failed to get rooms", zap.Error(err))
		return nil, fmt.Errorf("failed to search for room: %w", err)
	}

	for _, room := range rooms {
		if room.JoinCode == joinCode {
			if uc.isRoomExpired(room) {
				uc.logger.Info("room has expired, deleting", zap.String("roomID", room.ID))
				_ = uc.repository.Delete(ctx, room.ID)
				return nil, fmt.Errorf("room has expired")
			}
			return room, nil
		}
	}

	return nil, fmt.Errorf("room not found with join code: %s", joinCode)
}

func (uc *roomUseCase) IsUserInRoom(ctx context.Context, roomID string, userID string) (bool, error) {
	if roomID == "" || userID == "" {
		return false, fmt.Errorf("room ID and user ID cannot be empty")
	}

	userIDs, err := uc.repository.GetUsers(ctx, roomID)
	if err != nil {
		uc.logger.Error("failed to get room users", zap.Error(err), zap.String("roomID", roomID))
		return false, fmt.Errorf("failed to check room membership: %w", err)
	}

	for _, id := range userIDs {
		if id == userID {
			return true, nil
		}
	}

	return false, nil
}

func (uc *roomUseCase) JoinRoom(ctx context.Context, roomID string, user model.User) error {
	if roomID == "" {
		return fmt.Errorf("room ID cannot be empty")
	}

	room, err := uc.GetByID(ctx, roomID)
	if err != nil {
		return err
	}

	// Check if user is already in the room
	for _, member := range room.Members {
		if member.ID == user.ID {
			uc.logger.Debug("user already in room", zap.String("roomID", roomID), zap.String("userID", user.ID))
			return nil // Already a member, no error
		}
	}

	if err := uc.repository.AddUser(ctx, roomID, user); err != nil {
		uc.logger.Error("failed to add user to room", zap.Error(err), zap.String("roomID", roomID), zap.String("userID", user.ID))
		return fmt.Errorf("failed to join room: %w", err)
	}

	uc.logger.Info("user joined room", zap.String("roomID", roomID), zap.String("userID", user.ID), zap.String("username", user.Username))
	return nil
}

func (uc *roomUseCase) LeaveRoom(ctx context.Context, roomID string, userID string) error {
	if roomID == "" || userID == "" {
		return fmt.Errorf("room ID and user ID cannot be empty")
	}

	room, err := uc.repository.GetByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("failed to get room: %w", err)
	}

	if room == nil {
		return fmt.Errorf("room not found")
	}

	if room.Owner.ID == userID {
		return fmt.Errorf("room owner cannot leave, delete the room instead")
	}

	if err := uc.repository.RemoveUser(ctx, roomID, userID); err != nil {
		uc.logger.Error("failed to remove user from room", zap.Error(err), zap.String("roomID", roomID), zap.String("userID", userID))
		return fmt.Errorf("failed to leave room: %w", err)
	}

	uc.logger.Info("user left room", zap.String("roomID", roomID), zap.String("userID", userID))
	return nil
}

func (uc *roomUseCase) isRoomExpired(room *model.Room) bool {
	if room.Expiry == 0 {
		return false
	}

	expiryTime := room.CreatedAt.Add(room.Expiry)
	return time.Now().After(expiryTime)
}

func generateJoinCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Exclude similar looking chars
	const codeLength = 6

	code := make([]byte, codeLength)
	for i := range code {
		code[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond) // Ensure different values for the time
	}

	return string(code)
}
