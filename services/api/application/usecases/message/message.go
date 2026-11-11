package message

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/services/api/domain/apperr"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/events"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"go.uber.org/zap"
)

const (
	maxMessageLength = 2000

	defaultMessageLimit = 50
	maxMessageLimit     = 200
)

var (
	ErrRoomIDRequired    = apperr.New(apperr.Invalid, "room ID cannot be empty")
	ErrMessageIDRequired = apperr.New(apperr.Invalid, "message ID cannot be empty")
	ErrUserIDRequired    = apperr.New(apperr.Invalid, "user ID cannot be empty")
	ErrUsernameRequired  = apperr.New(apperr.Invalid, "username cannot be empty")
	ErrEmptyMessage      = apperr.New(apperr.Invalid, "message cannot be empty")
	ErrWhitespaceMessage = apperr.New(apperr.Invalid, "message cannot contain only whitespace")

	ErrMessageNotFound = apperr.New(apperr.NotFound, "message not found")
	ErrNotEditor       = apperr.New(apperr.Forbidden, "unauthorized: you can only edit your own messages")
	ErrNotDeleter      = apperr.New(apperr.Forbidden, "unauthorized: you can only delete your own messages")
)

type MessageUseCase interface {
	Send(ctx context.Context, roomID, userID, username, content string, encrypted bool) (*model.Message, error)
	Update(ctx context.Context, roomID, messageID, userID, content string, encrypted bool) error
	Delete(ctx context.Context, roomID, messageID, userID string) error
	GetRoomMessages(ctx context.Context, roomID string, limit int64) ([]*model.Message, error)
	GetMessagesAfter(ctx context.Context, roomID string, after time.Time, limit int64) ([]*model.Message, error)
	GetMessageCount(ctx context.Context, roomID string) (int64, error)
}

type messageUseCase struct {
	repository     repository.MessageRepository
	eventPublisher *events.EventPublisher
	logger         *logger.Logger
}

func NewMessageUseCase(
	repository repository.MessageRepository,
	eventPublisher *events.EventPublisher,
	logger *logger.Logger,
) MessageUseCase {
	return &messageUseCase{
		repository:     repository,
		eventPublisher: eventPublisher,
		logger:         logger,
	}
}

func (uc *messageUseCase) Send(ctx context.Context, roomID, userID, username, content string, encrypted bool) (*model.Message, error) {
	switch {
	case roomID == "":
		return nil, ErrRoomIDRequired
	case userID == "":
		return nil, ErrUserIDRequired
	case username == "":
		return nil, ErrUsernameRequired
	}
	content, err := validateContent(content)
	if err != nil {
		return nil, err
	}

	message := &model.Message{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		UserID:    userID,
		Username:  username,
		Content:   content,
		Encrypted: encrypted,
		CreatedAt: time.Now(),
	}

	if err := uc.repository.Create(ctx, message); err != nil {
		uc.logger.Error("failed to create message", zap.Error(err), zap.String("roomID", roomID), zap.String("userID", userID))
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	if uc.eventPublisher != nil {
		go func() {
			if err := uc.eventPublisher.PublishMessageSent(roomID, userID, message.ID, len(message.Content)); err != nil {
				uc.logger.Error("failed to publish message sent event", zap.Error(err))
			}
		}()
	}

	uc.logger.Info("message sent", zap.String("roomID", roomID), zap.String("userID", userID), zap.String("username", username))
	return message, nil
}

func (uc *messageUseCase) Update(ctx context.Context, roomID, messageID, userID, content string, encrypted bool) error {
	if err := validateIDs(roomID, messageID, userID); err != nil {
		return err
	}
	content, err := validateContent(content)
	if err != nil {
		return err
	}

	message, err := uc.getOwnMessage(ctx, roomID, messageID, userID, ErrNotEditor)
	if err != nil {
		return err
	}

	message.Content = content
	message.Encrypted = encrypted
	message.UpdatedAt = time.Now()

	if err := uc.repository.Update(ctx, message); err != nil {
		uc.logger.Error("failed to update message", zap.Error(err), zap.String("messageID", messageID))
		return fmt.Errorf("failed to update message: %w", err)
	}

	uc.logger.Info("message updated", zap.String("messageID", messageID), zap.String("roomID", roomID), zap.String("userID", userID))
	return nil
}

func (uc *messageUseCase) Delete(ctx context.Context, roomID, messageID, userID string) error {
	if err := validateIDs(roomID, messageID, userID); err != nil {
		return err
	}
	if _, err := uc.getOwnMessage(ctx, roomID, messageID, userID, ErrNotDeleter); err != nil {
		return err
	}

	if err := uc.repository.Delete(ctx, roomID, messageID); err != nil {
		uc.logger.Error("failed to delete message", zap.Error(err), zap.String("messageID", messageID))
		return fmt.Errorf("failed to delete message: %w", err)
	}

	uc.logger.Info("message deleted", zap.String("messageID", messageID), zap.String("roomID", roomID), zap.String("userID", userID))
	return nil
}

func (uc *messageUseCase) GetRoomMessages(ctx context.Context, roomID string, limit int64) ([]*model.Message, error) {
	if roomID == "" {
		return nil, ErrRoomIDRequired
	}
	messages, err := uc.repository.GetByRoom(ctx, roomID, normalizeLimit(limit))
	if err != nil {
		uc.logger.Error("failed to get messages", zap.Error(err), zap.String("roomID", roomID))
		return nil, fmt.Errorf("failed to retrieve messages: %w", err)
	}
	return messages, nil
}

func (uc *messageUseCase) GetMessagesAfter(ctx context.Context, roomID string, after time.Time, limit int64) ([]*model.Message, error) {
	if roomID == "" {
		return nil, ErrRoomIDRequired
	}
	messages, err := uc.repository.GetByRoomAfter(ctx, roomID, after, normalizeLimit(limit))
	if err != nil {
		uc.logger.Error("failed to get messages after timestamp", zap.Error(err), zap.String("roomID", roomID), zap.Time("after", after))
		return nil, fmt.Errorf("failed to retrieve messages: %w", err)
	}
	return messages, nil
}

func (uc *messageUseCase) GetMessageCount(ctx context.Context, roomID string) (int64, error) {
	if roomID == "" {
		return 0, ErrRoomIDRequired
	}
	count, err := uc.repository.Count(ctx, roomID)
	if err != nil {
		uc.logger.Error("failed to get message count", zap.Error(err), zap.String("roomID", roomID))
		return 0, fmt.Errorf("failed to get message count: %w", err)
	}
	return count, nil
}

// getOwnMessage loads a message that userID must have written; notAuthor is
// returned when they didn't.
func (uc *messageUseCase) getOwnMessage(ctx context.Context, roomID, messageID, userID string, notAuthor error) (*model.Message, error) {
	message, err := uc.repository.GetByID(ctx, roomID, messageID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		uc.logger.Error("failed to get message", zap.Error(err), zap.String("messageID", messageID))
		return nil, fmt.Errorf("failed to get message: %w", err)
	}
	if message.UserID != userID {
		return nil, notAuthor
	}
	return message, nil
}

func validateIDs(roomID, messageID, userID string) error {
	switch {
	case roomID == "":
		return ErrRoomIDRequired
	case messageID == "":
		return ErrMessageIDRequired
	case userID == "":
		return ErrUserIDRequired
	}
	return nil
}

// validateContent checks a message and returns it trimmed.
func validateContent(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	switch {
	case trimmed == "":
		return "", ErrEmptyMessage
	case len(trimmed) > maxMessageLength:
		return "", apperr.Newf(apperr.Invalid, "message cannot exceed %d characters (got %d)", maxMessageLength, len(trimmed))
	case isOnlyWhitespace(trimmed):
		return "", ErrWhitespaceMessage
	}
	return trimmed, nil
}

func normalizeLimit(limit int64) int64 {
	if limit <= 0 {
		return defaultMessageLimit
	}
	return min(limit, maxMessageLimit)
}

// isOnlyWhitespace reports whether s has no printable ASCII (or any non-ASCII)
// character.
func isOnlyWhitespace(s string) bool {
	for _, char := range s {
		if char > ' ' {
			return false
		}
	}
	return true
}
