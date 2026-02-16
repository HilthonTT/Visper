package message

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/hilthontt/visper/api/infrastructure/events"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"go.uber.org/zap"
)

const (
	// Message constraints
	maxMessageLength = 2000
	minMessageLength = 1

	// Default limits
	defaultMessageLimit = 50
	maxMessageLimit     = 200

	// Message retention
	messageRetentionDays = 7
)

type MessageUseCase interface {
	Delete(ctx context.Context, roomID, messageID, userID string) error
	Update(ctx context.Context, roomID, messageID, userID, content string, encrypted bool) error
	Send(ctx context.Context, roomID, userID, username, content string, encrypted bool) (*model.Message, error)
	GetRoomMessages(ctx context.Context, roomID string, limit int64) ([]*model.Message, error)
	GetMessagesAfter(ctx context.Context, roomID string, after time.Time, limit int64) ([]*model.Message, error)
	GetMessageCount(ctx context.Context, roomID string) (int64, error)
	CleanupOldMessages(ctx context.Context, roomID string) error
	CleanupAllOldMessages(ctx context.Context, roomIDs []string) error
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

func (uc *messageUseCase) Update(ctx context.Context, roomID, messageID, userID, content string, encrypted bool) error {
	if roomID == "" {
		return fmt.Errorf("room ID cannot be empty")
	}
	if messageID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	// Validate message content
	if err := uc.validateMessageContent(content); err != nil {
		return err
	}

	existingMessage, err := uc.repository.GetByID(ctx, roomID, messageID)
	if err != nil {
		uc.logger.Error("failed to get message for update", zap.Error(err), zap.String("messageID", messageID))
		return fmt.Errorf("message not found: %w", err)
	}

	// Verify the user owns this message
	if existingMessage.UserID != userID {
		return fmt.Errorf("unauthorized: you can only edit your own messages")
	}

	existingMessage.Content = strings.TrimSpace(content)
	existingMessage.Encrypted = encrypted
	existingMessage.UpdatedAt = time.Now()

	if err := uc.repository.Update(ctx, existingMessage); err != nil {
		uc.logger.Error("failed to update message", zap.Error(err), zap.String("messageID", messageID))
		return fmt.Errorf("failed to update message: %w", err)
	}

	uc.logger.Info("message updated",
		zap.String("messageID", messageID),
		zap.String("roomID", roomID),
		zap.String("userID", userID))

	return nil
}

func (uc *messageUseCase) Delete(ctx context.Context, roomID, messageID, userID string) error {
	if roomID == "" {
		return fmt.Errorf("room ID cannot be empty")
	}
	if messageID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	existingMessage, err := uc.repository.GetByID(ctx, roomID, messageID)
	if err != nil {
		uc.logger.Error("failed to get message for deletion", zap.Error(err), zap.String("messageID", messageID))
		return fmt.Errorf("message not found: %w", err)
	}

	if existingMessage.UserID != userID {
		return fmt.Errorf("unauthorized: you can only delete your own messages")
	}

	if err := uc.repository.Delete(ctx, roomID, messageID); err != nil {
		uc.logger.Error("failed to delete message", zap.Error(err), zap.String("messageID", messageID))
		return fmt.Errorf("failed to delete message: %w", err)
	}

	uc.logger.Info("message deleted",
		zap.String("messageID", messageID),
		zap.String("roomID", roomID),
		zap.String("userID", userID))

	return nil
}

func (uc *messageUseCase) CleanupAllOldMessages(ctx context.Context, roomIDs []string) error {
	if len(roomIDs) == 0 {
		return nil
	}

	cutoffTime := time.Now().Add(-messageRetentionDays * 24 * time.Hour)
	errorCount := 0
	successCount := 0

	for _, roomID := range roomIDs {
		if err := uc.repository.DeleteOldMessages(ctx, roomID, cutoffTime); err != nil {
			uc.logger.Error("failed to cleanup messages for room", zap.Error(err), zap.String("roomID", roomID))
			errorCount++
			continue
		}
		successCount++
	}

	uc.logger.Info("bulk message cleanup completed",
		zap.Int("totalRooms", len(roomIDs)),
		zap.Int("successful", successCount),
		zap.Int("failed", errorCount),
		zap.Time("cutoffTime", cutoffTime))

	if errorCount > 0 {
		return fmt.Errorf("cleanup partially failed: %d/%d rooms had errors", errorCount, len(roomIDs))
	}

	return nil
}

func (uc *messageUseCase) CleanupOldMessages(ctx context.Context, roomID string) error {
	if roomID == "" {
		return fmt.Errorf("room ID cannot be empty")
	}

	cutoffTime := time.Now().Add(-messageRetentionDays * 24 * time.Hour)

	if err := uc.repository.DeleteOldMessages(ctx, roomID, cutoffTime); err != nil {
		uc.logger.Error("failed to cleanup old messages", zap.Error(err), zap.String("roomID", roomID))
		return fmt.Errorf("failed to cleanup old messages: %w", err)
	}

	uc.logger.Info("cleaned up old messages", zap.String("roomID", roomID), zap.Time("cutoffTime", cutoffTime))
	return nil
}

func (uc *messageUseCase) GetMessageCount(ctx context.Context, roomID string) (int64, error) {
	if roomID == "" {
		return 0, fmt.Errorf("room ID cannot be empty")
	}

	count, err := uc.repository.Count(ctx, roomID)
	if err != nil {
		uc.logger.Error("failed to get message count", zap.Error(err), zap.String("roomID", roomID))
		return 0, fmt.Errorf("failed to get message count: %w", err)
	}

	return count, nil
}

func (uc *messageUseCase) GetMessagesAfter(ctx context.Context, roomID string, after time.Time, limit int64) ([]*model.Message, error) {
	if roomID == "" {
		return nil, fmt.Errorf("room ID cannot be empty")
	}

	limit = uc.normalizeLimit(limit)

	messages, err := uc.repository.GetByRoomAfter(ctx, roomID, after, limit)
	if err != nil {
		uc.logger.Error("failed to get messages after timestamp", zap.Error(err), zap.String("roomID", roomID), zap.Time("after", after))
		return nil, fmt.Errorf("failed to retrieve messages: %w", err)
	}

	uc.logger.Debug("retrieved messages after timestamp", zap.String("roomID", roomID), zap.Int("count", len(messages)), zap.Time("after", after))
	return messages, nil
}

func (uc *messageUseCase) GetRoomMessages(ctx context.Context, roomID string, limit int64) ([]*model.Message, error) {
	if roomID == "" {
		return nil, fmt.Errorf("room ID cannot be empty")
	}

	limit = uc.normalizeLimit(limit)

	messages, err := uc.repository.GetByRoom(ctx, roomID, limit)
	if err != nil {
		uc.logger.Error("failed to get messages", zap.Error(err), zap.String("roomID", roomID))
		return nil, fmt.Errorf("failed to retrieve messages: %w", err)
	}

	uc.logger.Debug("retrieved messages", zap.String("roomID", roomID), zap.Int("count", len(messages)))
	return messages, nil
}

func (uc *messageUseCase) Send(
	ctx context.Context,
	roomID string,
	userID string,
	username string,
	content string,
	encrypted bool,
) (*model.Message, error) {
	if roomID == "" {
		return nil, fmt.Errorf("room ID cannot be empty")
	}
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}

	// Validate message content
	if err := uc.validateMessageContent(content); err != nil {
		return nil, err
	}

	message := &model.Message{
		ID:        uuid.NewString(),
		RoomID:    roomID,
		UserID:    userID,
		Username:  username,
		Content:   strings.TrimSpace(content),
		Encrypted: encrypted,
		CreatedAt: time.Now(),
	}

	if err := uc.repository.Create(ctx, message); err != nil {
		uc.logger.Error("failed to create message", zap.Error(err), zap.String("roomID", roomID), zap.String("userID", userID))
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	go func() {
		messageSize := len(message.Content)
		if err := uc.eventPublisher.PublishMessageSent(roomID, userID, message.ID, messageSize); err != nil {
			log.Printf("Failed to publish message sent event: %v", err)
		}
	}()

	uc.logger.Info("message sent", zap.String("userID", userID), zap.String("roomID", roomID), zap.String("userID", userID), zap.String("username", username))
	return message, nil
}

func (uc *messageUseCase) validateMessageContent(content string) error {
	trimmed := strings.TrimSpace(content)

	if len(trimmed) < minMessageLength {
		return fmt.Errorf("message cannot be empty")
	}

	if len(trimmed) > maxMessageLength {
		return fmt.Errorf("message cannot exceed %d characters (got %d)", maxMessageLength, len(trimmed))
	}

	if isOnlyWhitespace(trimmed) {
		return fmt.Errorf("message cannot contain only whitespace")
	}

	return nil
}

func (uc *messageUseCase) normalizeLimit(limit int64) int64 {
	if limit <= 0 {
		return defaultMessageLimit
	}
	if limit > maxMessageLimit {
		return maxMessageLimit
	}
	return limit
}

func isOnlyWhitespace(s string) bool {
	for _, char := range s {
		// ASCII space is 32, anything above is printable
		if char > 32 {
			return false
		}
	}
	return true
}
