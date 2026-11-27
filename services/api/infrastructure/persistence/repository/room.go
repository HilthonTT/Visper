package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/cache"
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

func (r *roomRepository) AddUser(ctx context.Context, roomID string, user model.User, roomCodes repository.RoomCodes) error {
	ctx, span := r.tracer.Start(ctx, "roomRepository.AddUser")
	defer span.End()

	span.SetAttributes(
		attribute.String("room.id", roomID),
		attribute.String("user.id", user.ID),
		attribute.String("user.username", user.Username),
	)

	// One script checks the room and adds the user. The room must exist:
	// SADD on a deleted room would recreate its membership set, which nothing
	// ever removes again. And its codes must still be the ones the joiner
	// presented, read from Redis at this moment: a kick rotates the codes
	// before removing the member, so a join that raced the kick with the old
	// codes either lands first (and is removed by the kick) or fails here.
	want := make(map[string]string, 2)
	if roomCodes.JoinCode != "" {
		want["joinCode"] = roomCodes.JoinCode
	}
	if roomCodes.SecureCode != "" {
		want["secureCode"] = roomCodes.SecureCode
	}
	result, err := r.cache.SAddIf(ctx,
		fmt.Sprintf("room:%s", roomID), fmt.Sprintf("room:%s:users", roomID), user.ID, want)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to add user to room set")
		return err
	}
	switch result {
	case cache.SAddRecordMissing:
		span.SetStatus(codes.Error, "room not found")
		return repository.ErrNotFound
	case cache.SAddFieldMismatch:
		span.SetStatus(codes.Error, "room codes changed")
		return repository.ErrCodesChanged
	}

	// Membership lives only in the set (GetByID rebuilds Members from it).
	// Rewriting the room record here would race with Update and could restore
	// a join or secure code the owner just rotated.

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
	if err := r.cache.Set(ctx, key, room, 0); err != nil {
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

	if err := r.indexJoinCode(ctx, room.JoinCode, room.ID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to index join code")
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
	var existing model.Room
	found, err := r.cache.Get(ctx, key, &existing)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read room before delete")
		return err
	}

	if err := r.cache.Delete(ctx, key); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete room from cache")
		return err
	}

	// Drop the membership set too; it would otherwise leak forever.
	if err := r.cache.Delete(ctx, fmt.Sprintf("room:%s:users", id)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete room users set")
		return err
	}

	// Same for the message history: room IDs are never reused, so nothing
	// would ever read or expire it again.
	if err := r.cache.Delete(ctx, fmt.Sprintf("room:%s:messages", id)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete room messages")
		return err
	}

	// And the room's key cards and rekeys (see roomKeyRepository).
	for _, key := range []string{roomCardsKey(id), roomRekeysKey(id), roomDepartedKey(id)} {
		if err := r.cache.Delete(ctx, key); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to delete room keys")
			return err
		}
	}

	if err := r.cache.SRem(ctx, "rooms", id); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to remove room from rooms set")
		return err
	}

	if found {
		if err := r.unindexJoinCode(ctx, existing.JoinCode, id); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to unindex join code")
			return err
		}
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

// joinCodeKey is the set of rooms using a join code. It is a set, not a
// single ID, so two rooms that happen to draw the same code don't overwrite
// each other's entry; GetByJoinCode returns the first that still has it.
func joinCodeKey(joinCode string) string { return "joincode:" + joinCode }

func (r *roomRepository) indexJoinCode(ctx context.Context, joinCode, roomID string) error {
	if joinCode == "" {
		return nil
	}
	return r.cache.SAdd(ctx, joinCodeKey(joinCode), roomID)
}

func (r *roomRepository) unindexJoinCode(ctx context.Context, joinCode, roomID string) error {
	if joinCode == "" {
		return nil
	}
	return r.cache.SRem(ctx, joinCodeKey(joinCode), roomID)
}

func (r *roomRepository) GetByJoinCode(ctx context.Context, joinCode string) (*model.Room, error) {
	ctx, span := r.tracer.Start(ctx, "roomRepository.GetByJoinCode")
	defer span.End()

	if joinCode == "" {
		return nil, repository.ErrNotFound
	}
	roomIDs, err := r.cache.SMembers(ctx, joinCodeKey(joinCode))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read join code index")
		return nil, err
	}
	span.SetAttributes(attribute.Int("rooms.candidates", len(roomIDs)))

	for _, id := range roomIDs {
		room, err := r.GetByID(ctx, id)
		if errors.Is(err, repository.ErrNotFound) {
			// Left behind by a delete that failed halfway; drop it.
			_ = r.unindexJoinCode(ctx, joinCode, id)
			continue
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to get indexed room")
			return nil, err
		}
		// The index can lag behind the record (a rotation that failed after
		// saving the room), so the record has the last word.
		if room.JoinCode == joinCode {
			span.SetStatus(codes.Ok, "room found by join code")
			return room, nil
		}
	}

	span.SetStatus(codes.Error, "room not found")
	return nil, repository.ErrNotFound
}

// IndexJoinCodes adds every stored room to the join code index. It is run
// once at startup so rooms created before the index existed stay joinable
// by code.
func (r *roomRepository) IndexJoinCodes(ctx context.Context) error {
	roomIDs, err := r.cache.SMembers(ctx, "rooms")
	if err != nil {
		return err
	}
	for _, id := range roomIDs {
		var room model.Room
		found, err := r.cache.Get(ctx, fmt.Sprintf("room:%s", id), &room)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if err := r.indexJoinCode(ctx, room.JoinCode, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *roomRepository) GetByID(ctx context.Context, id string) (*model.Room, error) {
	ctx, span := r.tracer.Start(ctx, "roomRepository.GetByID")
	defer span.End()

	span.SetAttributes(attribute.String("room.id", id))

	key := fmt.Sprintf("room:%s", id)
	var room model.Room

	found, err := r.cache.Get(ctx, key, &room)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get room from cache")
		return nil, err
	}
	if !found {
		span.SetAttributes(attribute.Bool("room.found", false))
		span.SetStatus(codes.Error, "room not found")
		return nil, repository.ErrNotFound
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
		// The record holds a copy of the owner taken at creation; use the
		// current user so a rename shows up in the owner's name too.
		if user.ID == room.Owner.ID {
			room.Owner = *user
		}
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

	// As in AddUser, the room record is not rewritten: membership is the set.

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
	found, err := r.cache.Get(ctx, key, &existingRoom)
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

	if err := r.cache.Set(ctx, key, room, 0); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to update room in cache")
		return err
	}

	if existingRoom.JoinCode != room.JoinCode {
		// Index the new code before dropping the old one: a stale entry is
		// harmless (GetByJoinCode checks the record), a missing one is not.
		if err := r.indexJoinCode(ctx, room.JoinCode, room.ID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to index new join code")
			return err
		}
		if err := r.unindexJoinCode(ctx, existingRoom.JoinCode, room.ID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to unindex old join code")
			return err
		}
	}

	span.SetStatus(codes.Ok, "room updated successfully")
	return nil
}
