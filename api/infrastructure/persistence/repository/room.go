package repository

import (
	"context"
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

type roomRepository struct {
	cache          *cache.DistributedCache
	userRepository repository.UserRepository
	tracer         trace.Tracer
}

func NewRoomRepository(
	cache *cache.DistributedCache,
	userRepository repository.UserRepository,
	tracer trace.Tracer,
) repository.RoomRepository {
	return &roomRepository{
		cache:          cache,
		userRepository: userRepository,
		tracer:         tracer,
	}
}

func (r *roomRepository) AddUser(ctx context.Context, roomID string, user model.User) error {
	ctx, span := r.tracer.Start(ctx, "roomRepository.AddUser")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", roomID),
		attribute.String("user.id", user.ID),
		attribute.String("user.username", user.Username),
	)

	// Use cache for set operations
	userKey := fmt.Sprintf("room:%s:users", roomID)
	if err := r.cache.SAdd(ctx, userKey, user.ID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to add user to room set")
		return err
	}

	// Get room from cache
	roomKey := fmt.Sprintf("room:%s", roomID)
	var room model.Room
	found, err := r.cache.Get(roomKey, &room)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get room from cache")
		return err
	}
	if !found {
		span.SetAttributes(attribute.Bool("room.found", false))
		span.SetStatus(codes.Error, "room not found")
		return fmt.Errorf("room not found")
	}

	span.SetAttributes(
		attribute.Bool("room.found", true),
	)

	// Check if user already exists in members
	if room.IsMember(user.ID) {
		span.SetAttributes(attribute.Bool("user.already_member", true))
		span.SetStatus(codes.Ok, "user already a member")
		return nil
	}

	span.SetAttributes(attribute.Bool("user.already_member", false))
	room.Members = append(room.Members, user)

	// Update cache
	if err := r.cache.Set(roomKey, &room, 0); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to update room in cache")
		return err
	}

	span.SetStatus(codes.Ok, "user added to room successfully")
	return nil
}

func (r *roomRepository) Create(ctx context.Context, room *model.Room) error {
	ctx, span := r.tracer.Start(ctx, "roomRepository.Create")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", room.ID),
		attribute.Int("room.members_count", len(room.Members)),
	)

	room.CreatedAt = time.Now()

	key := fmt.Sprintf("room:%s", room.ID)
	if err := r.cache.Set(key, room, 0); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create room in cache")
		return err
	}

	// Use cache for maintaining the rooms set
	if err := r.cache.SAdd(ctx, "rooms", room.ID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to add room to rooms set")
		return err
	}

	span.SetStatus(codes.Ok, "room created successfully")
	return nil
}

func (r *roomRepository) Delete(ctx context.Context, id string) error {
	ctx, span := r.tracer.Start(ctx, "roomRepository.Delete")
	defer span.End()

	span.SetAttributes(attribute.String("room.id", id))

	key := fmt.Sprintf("room:%s", id)
	if err := r.cache.Delete(key); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete room from cache")
		return err
	}

	if err := r.cache.SRem(ctx, "rooms", id); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to remove room from rooms set")
		return err
	}

	span.SetStatus(codes.Ok, "room deleted successfully")
	return nil
}

func (r *roomRepository) GetAll(ctx context.Context) ([]*model.Room, error) {
	ctx, span := r.tracer.Start(ctx, "roomRepository.GetAll")
	defer span.End()

	roomIDs, err := r.cache.SMembers(ctx, "rooms")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get room IDs from set")
		return nil, err
	}

	span.SetAttributes(attribute.Int("rooms.total_count", len(roomIDs)))

	rooms := make([]*model.Room, 0, len(roomIDs))
	skippedCount := 0

	for _, id := range roomIDs {
		room, err := r.GetByID(ctx, id)
		if err != nil {
			skippedCount++
			continue // Skip rooms that can't be retrieved
		}
		rooms = append(rooms, room)
	}

	span.SetAttributes(
		attribute.Int("rooms.retrieved_count", len(rooms)),
		attribute.Int("rooms.skipped_count", skippedCount),
	)

	if skippedCount > 0 {
		span.SetStatus(codes.Ok, fmt.Sprintf("retrieved %d rooms, skipped %d", len(rooms), skippedCount))
	} else {
		span.SetStatus(codes.Ok, "all rooms retrieved successfully")
	}

	return rooms, nil
}

func (r *roomRepository) GetByID(ctx context.Context, id string) (*model.Room, error) {
	ctx, span := r.tracer.Start(ctx, "roomRepository.GetByID")
	defer span.End()

	span.SetAttributes(attribute.String("room.id", id))

	key := fmt.Sprintf("room:%s", id)
	var room model.Room

	found, err := r.cache.Get(key, &room)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get room from cache")
		return nil, err
	}
	if !found {
		span.SetAttributes(attribute.Bool("room.found", false))
		span.SetStatus(codes.Error, "room not found")
		return nil, redis.Nil
	}

	span.SetAttributes(
		attribute.Bool("room.found", true),
	)

	userIDs, err := r.GetUsers(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get room users")
		return nil, err
	}

	span.SetAttributes(attribute.Int("room.users_count", len(userIDs)))

	room.Members = make([]model.User, 0, len(userIDs))
	skippedUsers := 0

	for _, userID := range userIDs {
		user, err := r.userRepository.GetByID(ctx, userID)
		if err != nil {
			// User might have been deleted, skip them
			skippedUsers++
			continue
		}
		room.Members = append(room.Members, *user)
	}

	span.SetAttributes(
		attribute.Int("room.members_loaded", len(room.Members)),
		attribute.Int("room.members_skipped", skippedUsers),
	)

	span.SetStatus(codes.Ok, "room retrieved successfully")
	return &room, nil
}

func (r *roomRepository) GetUsers(ctx context.Context, roomID string) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "roomRepository.GetUsers")
	defer span.End()

	span.SetAttributes(attribute.String("room.id", roomID))

	key := fmt.Sprintf("room:%s:users", roomID)
	userIDs, err := r.cache.SMembers(ctx, key)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get users from room set")
		return nil, err
	}

	span.SetAttributes(attribute.Int("users.count", len(userIDs)))
	span.SetStatus(codes.Ok, "room users retrieved successfully")
	return userIDs, nil
}

func (r *roomRepository) RemoveUser(ctx context.Context, roomID string, userID string) error {
	ctx, span := r.tracer.Start(ctx, "roomRepository.RemoveUser")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", roomID),
		attribute.String("user.id", userID),
	)

	userKey := fmt.Sprintf("room:%s:users", roomID)
	if err := r.cache.SRem(ctx, userKey, userID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to remove user from room set")
		return err
	}

	roomKey := fmt.Sprintf("room:%s", roomID)
	var room model.Room

	found, err := r.cache.Get(roomKey, &room)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get room from cache")
		return err
	}
	if !found {
		span.SetAttributes(attribute.Bool("room.found", false))
		span.SetStatus(codes.Error, "room not found")
		return fmt.Errorf("room not found")
	}

	span.SetAttributes(
		attribute.Bool("room.found", true),
	)

	userFound := false
	for i, member := range room.Members {
		if member.ID == userID {
			room.Members = append(room.Members[:i], room.Members[i+1:]...)
			userFound = true
			break
		}
	}

	span.SetAttributes(attribute.Bool("user.found_in_members", userFound))

	if err := r.cache.Set(roomKey, &room, 0); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to update room in cache")
		return err
	}

	span.SetStatus(codes.Ok, "user removed from room successfully")
	return nil
}

func (r *roomRepository) Update(ctx context.Context, room *model.Room) error {
	ctx, span := r.tracer.Start(ctx, "roomRepository.Update")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", room.ID),
		attribute.Int("room.members_count", len(room.Members)),
	)

	key := fmt.Sprintf("room:%s", room.ID)

	// Check if room exists in cache
	var existingRoom model.Room
	found, err := r.cache.Get(key, &existingRoom)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get existing room from cache")
		return err
	}
	if !found {
		span.SetAttributes(attribute.Bool("room.exists", false))
		span.SetStatus(codes.Error, "room does not exist")
		return fmt.Errorf("room with id %s does not exist", room.ID)
	}

	span.SetAttributes(attribute.Bool("room.exists", true))

	if err := r.cache.Set(key, room, 0); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to update room in cache")
		return err
	}

	span.SetStatus(codes.Ok, "room updated successfully")
	return nil
}
