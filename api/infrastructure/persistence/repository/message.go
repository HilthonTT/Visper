package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/redis/go-redis/v9"
)

type messageRepository struct {
	client *redis.Client
}

func NewMessageRepository(client *redis.Client) repository.MessageRepository {
	return &messageRepository{
		client: client,
	}
}

func (r *messageRepository) GetByID(ctx context.Context, roomID, messageID string) (*model.Message, error) {
	key := fmt.Sprintf("room:%s:messages", roomID)

	results, err := r.client.ZRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	for _, data := range results {
		var msg model.Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			continue
		}
		if msg.ID == messageID {
			return &msg, nil
		}
	}

	return nil, fmt.Errorf("message not found")
}

func (r *messageRepository) Create(ctx context.Context, message *model.Message) error {
	message.CreatedAt = time.Now()
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// Store message in sorted set by timestamp for ordering
	key := fmt.Sprintf("room:%s:messages", message.RoomID)
	score := float64(message.CreatedAt.Unix())

	return r.client.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: data,
	}).Err()
}

func (r *messageRepository) Update(ctx context.Context, message *model.Message) error {
	key := fmt.Sprintf("room:%s:messages", message.RoomID)

	oldMessage, err := r.GetByID(ctx, message.RoomID, message.ID)
	if err != nil {
		return err
	}

	oldData, err := json.Marshal(oldMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal old message: %w", err)
	}

	message.UpdatedAt = time.Now()
	message.CreatedAt = oldMessage.CreatedAt

	newData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal new message: %w", err)
	}

	pipe := r.client.Pipeline()

	pipe.ZRem(ctx, key, oldData)

	score := float64(oldMessage.CreatedAt.Unix())
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: newData,
	})

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}

	return nil
}

func (r *messageRepository) Delete(ctx context.Context, roomID string, messageID string) error {
	key := fmt.Sprintf("room:%s:messages", roomID)

	message, err := r.GetByID(ctx, roomID, messageID)
	if err != nil {
		return err
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	result := r.client.ZRem(ctx, key, data)
	if result.Err() != nil {
		return fmt.Errorf("failed to delete message: %w", result.Err())
	}

	if result.Val() == 0 {
		return fmt.Errorf("message not found in sorted set")
	}

	return nil
}

func (r *messageRepository) GetByRoom(ctx context.Context, roomID string, limit int64) ([]*model.Message, error) {
	key := fmt.Sprintf("room:%s:messages", roomID)

	// Get messages in reverse chronological order (most recent first)
	results, err := r.client.ZRevRange(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}

	messages := make([]*model.Message, 0, len(results))
	for _, data := range results {
		var msg model.Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			continue
		}
		messages = append(messages, &msg)
	}

	// Reverse to get chronological order
	for i := len(messages)/2 - 1; i >= 0; i-- {
		opp := len(messages) - 1 - i
		messages[i], messages[opp] = messages[opp], messages[i]
	}

	return messages, nil
}

func (r *messageRepository) GetByRoomAfter(ctx context.Context, roomID string, after time.Time, limit int64) ([]*model.Message, error) {
	key := fmt.Sprintf("room:%s:messages", roomID)
	score := float64(after.Unix())

	results, err := r.client.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:   fmt.Sprintf("%f", score),
		Max:   "+inf",
		Count: limit,
	}).Result()

	if err != nil {
		return nil, err
	}

	messages := make([]*model.Message, 0, len(results))
	for _, data := range results {
		var msg model.Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

func (r *messageRepository) DeleteOldMessages(ctx context.Context, roomID string, before time.Time) error {
	key := fmt.Sprintf("room:%s:messages", roomID)
	score := float64(before.Unix())

	return r.client.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%f", score)).Err()
}

// Get message count for a room
func (r *messageRepository) Count(ctx context.Context, roomID string) (int64, error) {
	key := fmt.Sprintf("room:%s:messages", roomID)
	return r.client.ZCard(ctx, key).Result()
}
