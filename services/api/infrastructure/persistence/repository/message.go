package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/cache"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// maxMemberRaceRetries bounds how often Update/Delete re-read a message that
// was changed concurrently between reading and writing it.
const maxMemberRaceRetries = 3

var errConcurrentModification = errors.New("message was modified concurrently, try again")

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

	msg, _, err := r.find(ctx, roomID, messageID)
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.Bool("message.found", false))
		span.SetStatus(codes.Error, "message not found")
		return nil, err
	}

	span.SetAttributes(
		attribute.Bool("message.found", true),
		attribute.String("message.user_id", msg.UserID),
		attribute.Bool("message.encrypted", msg.Encrypted),
	)
	span.SetStatus(codes.Ok, "message retrieved successfully")
	return msg, nil
}

// find returns the message together with the exact sorted-set member it is
// stored as. Removing a member requires the original bytes: re-marshalling the
// decoded struct is not guaranteed to reproduce them.
func (r *messageRepository) find(ctx context.Context, roomID, messageID string) (*model.Message, string, error) {
	key := fmt.Sprintf("room:%s:messages", roomID)

	results, err := r.cache.ZRange(ctx, key, 0, -1)
	if err != nil {
		return nil, "", err
	}

	for _, data := range results {
		var msg model.Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			continue
		}
		if msg.ID == messageID {
			return &msg, data, nil
		}
	}

	return nil, "", repository.ErrNotFound
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

	// The stored member is replaced only if it is still the version we read:
	// otherwise an edit racing with a delete resurrected the deleted message,
	// and two racing edits left two copies of it.
	for range maxMemberRaceRetries {
		oldMessage, oldData, err := r.find(ctx, message.RoomID, message.ID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to get old message")
			return err
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

		replaced, err := r.cache.ZReplace(ctx, key, oldData, string(newData), float64(oldMessage.CreatedAt.Unix()))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to replace message")
			return fmt.Errorf("failed to update message: %w", err)
		}
		if replaced {
			span.SetStatus(codes.Ok, "message updated successfully")
			return nil
		}
	}

	span.SetStatus(codes.Error, "message changed concurrently")
	return errConcurrentModification
}

func (r *messageRepository) Delete(ctx context.Context, roomID string, messageID string) error {
	ctx, span := r.tracer.Start(ctx, "messageRepository.Delete")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", roomID),
		attribute.String("message.id", messageID),
	)

	key := fmt.Sprintf("room:%s:messages", roomID)

	// If the message was edited between find and ZREM, the old member is gone
	// and nothing is removed; look it up again instead of reporting success.
	for range maxMemberRaceRetries {
		_, data, err := r.find(ctx, roomID, messageID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to get message for deletion")
			return err
		}

		removed, err := r.cache.ZRem(ctx, key, data)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to remove message from sorted set")
			return fmt.Errorf("failed to delete message: %w", err)
		}
		if removed > 0 {
			span.SetStatus(codes.Ok, "message deleted successfully")
			return nil
		}
	}

	span.SetStatus(codes.Error, "message changed concurrently")
	return errConcurrentModification
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

	messages, unmarshalErrors := decodeMessages(results)

	span.SetAttributes(
		attribute.Int("messages.parsed_count", len(messages)),
		attribute.Int("messages.unmarshal_errors", unmarshalErrors),
	)

	// Scores only have one-second resolution and Redis orders equal scores by
	// member bytes (i.e. by message ID), so sort by the precise timestamp.
	sortChronologically(messages)

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

	// Scores are whole Unix seconds. Messages in after's own second need a
	// precise timestamp comparison (an inclusive score range returned the
	// message the caller passed as "after" again); later seconds all qualify.
	afterSec := strconv.FormatInt(after.Unix(), 10)

	sameSecond, err := r.cache.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: afterSec,
		Max: afterSec,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get messages by score range")
		return nil, err
	}

	later, err := r.cache.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:   "(" + afterSec,
		Max:   "+inf",
		Count: limit,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get messages by score range")
		return nil, err
	}

	span.SetAttributes(attribute.Int("messages.fetched_count", len(sameSecond)+len(later)))

	boundary, boundaryErrors := decodeMessages(sameSecond)
	rest, restErrors := decodeMessages(later)

	messages := messagesAfter(boundary, rest, after, limit)

	span.SetAttributes(
		attribute.Int("messages.parsed_count", len(messages)),
		attribute.Int("messages.unmarshal_errors", boundaryErrors+restErrors),
	)

	span.SetStatus(codes.Ok, "messages retrieved successfully")
	return messages, nil
}

// messagesAfter merges the messages stored in after's own second (boundary)
// with those of later seconds (rest), keeping only messages created strictly
// after "after", in chronological order and at most limit of them (limit <= 0
// means no limit).
func messagesAfter(boundary, rest []*model.Message, after time.Time, limit int64) []*model.Message {
	messages := make([]*model.Message, 0, len(boundary)+len(rest))
	for _, msg := range boundary {
		if msg.CreatedAt.After(after) {
			messages = append(messages, msg)
		}
	}
	messages = append(messages, rest...)

	sortChronologically(messages)

	if limit > 0 && int64(len(messages)) > limit {
		messages = messages[:limit]
	}
	return messages
}

// decodeMessages unmarshals sorted-set members, skipping (and counting) the
// ones that are not valid messages.
func decodeMessages(results []string) ([]*model.Message, int) {
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

	return messages, unmarshalErrors
}

func sortChronologically(messages []*model.Message) {
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})
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
