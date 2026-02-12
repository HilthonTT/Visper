package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hilthontt/visper/api/domain/model"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/hilthontt/visper/api/infrastructure/cache"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type messageRepository struct {
	cache  *cache.DistributedCache
	tracer trace.Tracer
}

func NewMessageRepository(cache *cache.DistributedCache, tracer trace.Tracer) repository.MessageRepository {
	return &messageRepository{
		cache:  cache,
		tracer: tracer,
	}
}

func (r *messageRepository) GetByID(ctx context.Context, roomID, messageID string) (*model.Message, error) {
	ctx, span := r.tracer.Start(ctx, "messageRepository.GetByID")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", roomID),
		attribute.String("message.id", messageID),
	)

	key := fmt.Sprintf("room:%s:messages", roomID)

	results, err := r.cache.ZRange(ctx, key, 0, -1)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get messages from sorted set")
		return nil, err
	}

	span.SetAttributes(attribute.Int("messages.scanned_count", len(results)))

	for _, data := range results {
		var msg model.Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			continue
		}
		if msg.ID == messageID {
			span.SetAttributes(
				attribute.Bool("message.found", true),
				attribute.String("message.user_id", msg.UserID),
				attribute.Bool("message.encrypted", msg.Encrypted),
			)
			span.SetStatus(codes.Ok, "message retrieved successfully")
			return &msg, nil
		}
	}

	span.SetAttributes(attribute.Bool("message.found", false))
	span.SetStatus(codes.Error, "message not found")
	return nil, redis.Nil
}

func (r *messageRepository) Create(ctx context.Context, message *model.Message) error {
	ctx, span := r.tracer.Start(ctx, "messageRepository.Create")
	defer span.End()

	span.SetAttributes(
		attribute.String("message.id", message.ID),
		attribute.String("room.id", message.RoomID),
		attribute.String("message.user_id", message.UserID),
		attribute.Bool("message.encrypted", message.Encrypted),
	)

	message.CreatedAt = time.Now()
	data, err := json.Marshal(message)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal message")
		return err
	}

	span.SetAttributes(attribute.Int("message.size_bytes", len(data)))

	// Store message in sorted set by timestamp for ordering
	key := fmt.Sprintf("room:%s:messages", message.RoomID)
	score := float64(message.CreatedAt.Unix())

	if err := r.cache.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: data,
	}); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to add message to sorted set")
		return err
	}

	span.SetStatus(codes.Ok, "message created successfully")
	return nil
}

func (r *messageRepository) Update(ctx context.Context, message *model.Message) error {
	ctx, span := r.tracer.Start(ctx, "messageRepository.Update")
	defer span.End()

	span.SetAttributes(
		attribute.String("message.id", message.ID),
		attribute.String("room.id", message.RoomID),
		attribute.String("message.user_id", message.UserID),
	)

	key := fmt.Sprintf("room:%s:messages", message.RoomID)

	oldMessage, err := r.GetByID(ctx, message.RoomID, message.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get old message")
		return err
	}

	oldData, err := json.Marshal(oldMessage)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal old message")
		return fmt.Errorf("failed to marshal old message: %w", err)
	}

	message.UpdatedAt = time.Now()
	message.CreatedAt = oldMessage.CreatedAt

	newData, err := json.Marshal(message)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal new message")
		return fmt.Errorf("failed to marshal new message: %w", err)
	}

	span.SetAttributes(
		attribute.Int("message.old_size_bytes", len(oldData)),
		attribute.Int("message.new_size_bytes", len(newData)),
	)

	// Use pipeline for atomic update
	pipe := r.cache.Pipeline()

	redisKey := r.cache.GetRedisKey(key)
	pipe.ZRem(ctx, redisKey, oldData)

	score := float64(oldMessage.CreatedAt.Unix())
	pipe.ZAdd(ctx, redisKey, redis.Z{
		Score:  score,
		Member: newData,
	})

	err = r.cache.ExecPipeline(ctx, pipe)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to execute pipeline for update")
		return fmt.Errorf("failed to update message: %w", err)
	}

	span.SetStatus(codes.Ok, "message updated successfully")
	return nil
}

func (r *messageRepository) Delete(ctx context.Context, roomID string, messageID string) error {
	ctx, span := r.tracer.Start(ctx, "messageRepository.Delete")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", roomID),
		attribute.String("message.id", messageID),
	)

	key := fmt.Sprintf("room:%s:messages", roomID)

	message, err := r.GetByID(ctx, roomID, messageID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get message for deletion")
		return err
	}

	data, err := json.Marshal(message)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal message for deletion")
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := r.cache.ZRem(ctx, key, data); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to remove message from sorted set")
		return fmt.Errorf("failed to delete message: %w", err)
	}

	span.SetStatus(codes.Ok, "message deleted successfully")
	return nil
}

func (r *messageRepository) GetByRoom(ctx context.Context, roomID string, limit int64) ([]*model.Message, error) {
	ctx, span := r.tracer.Start(ctx, "messageRepository.GetByRoom")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", roomID),
		attribute.Int64("query.limit", limit),
	)

	key := fmt.Sprintf("room:%s:messages", roomID)

	// Get messages in reverse chronological order (most recent first)
	results, err := r.cache.ZRevRange(ctx, key, 0, limit-1)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get messages from sorted set")
		return nil, err
	}

	span.SetAttributes(attribute.Int("messages.fetched_count", len(results)))

	messages := make([]*model.Message, 0, len(results))
	unmarshalErrors := 0

	for _, data := range results {
		var msg model.Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			unmarshalErrors++
			continue
		}
		messages = append(messages, &msg)
	}

	span.SetAttributes(
		attribute.Int("messages.parsed_count", len(messages)),
		attribute.Int("messages.unmarshal_errors", unmarshalErrors),
	)

	// Reverse to get chronological order
	for i := len(messages)/2 - 1; i >= 0; i-- {
		opp := len(messages) - 1 - i
		messages[i], messages[opp] = messages[opp], messages[i]
	}

	span.SetStatus(codes.Ok, "messages retrieved successfully")
	return messages, nil
}

func (r *messageRepository) GetByRoomAfter(ctx context.Context, roomID string, after time.Time, limit int64) ([]*model.Message, error) {
	ctx, span := r.tracer.Start(ctx, "messageRepository.GetByRoomAfter")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", roomID),
		attribute.String("query.after", after.Format(time.RFC3339)),
		attribute.Int64("query.limit", limit),
	)

	key := fmt.Sprintf("room:%s:messages", roomID)
	score := float64(after.Unix())

	results, err := r.cache.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:   fmt.Sprintf("%f", score),
		Max:   "+inf",
		Count: limit,
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get messages by score range")
		return nil, err
	}

	span.SetAttributes(attribute.Int("messages.fetched_count", len(results)))

	messages := make([]*model.Message, 0, len(results))
	unmarshalErrors := 0

	for _, data := range results {
		var msg model.Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			unmarshalErrors++
			continue
		}
		messages = append(messages, &msg)
	}

	span.SetAttributes(
		attribute.Int("messages.parsed_count", len(messages)),
		attribute.Int("messages.unmarshal_errors", unmarshalErrors),
	)

	span.SetStatus(codes.Ok, "messages retrieved successfully")
	return messages, nil
}

func (r *messageRepository) DeleteOldMessages(ctx context.Context, roomID string, before time.Time) error {
	ctx, span := r.tracer.Start(ctx, "messageRepository.DeleteOldMessages")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", roomID),
		attribute.String("query.before", before.Format(time.RFC3339)),
	)

	key := fmt.Sprintf("room:%s:messages", roomID)
	score := float64(before.Unix())

	if err := r.cache.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%f", score)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete old messages")
		return err
	}

	span.SetStatus(codes.Ok, "old messages deleted successfully")
	return nil
}

// Get message count for a room
func (r *messageRepository) Count(ctx context.Context, roomID string) (int64, error) {
	ctx, span := r.tracer.Start(ctx, "messageRepository.Count")
	defer span.End()

	span.SetAttributes(attribute.String("room.id", roomID))

	key := fmt.Sprintf("room:%s:messages", roomID)
	count, err := r.cache.ZCard(ctx, key)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get message count")
		return 0, err
	}

	span.SetAttributes(attribute.Int64("messages.count", count))
	span.SetStatus(codes.Ok, "message count retrieved successfully")
	return count, nil
}
