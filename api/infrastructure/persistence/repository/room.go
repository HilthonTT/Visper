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

type roomRepository struct {
	client         *redis.Client
	userRepository repository.UserRepository
}

func NewRoomRepository(client *redis.Client, userRepository repository.UserRepository) repository.RoomRepository {
	return &roomRepository{
		client:         client,
		userRepository: userRepository,
	}
}

func (r *roomRepository) AddUser(ctx context.Context, roomID string, user model.User) error {
	userKey := fmt.Sprintf("room:%s:users", roomID)
	if err := r.client.SAdd(ctx, userKey, user.ID).Err(); err != nil {
		return err
	}

	roomKey := fmt.Sprintf("room:%s", roomID)
	data, err := r.client.Get(ctx, roomKey).Bytes()
	if err != nil {
		return err
	}

	var room model.Room
	if err := json.Unmarshal(data, &room); err != nil {
		return err
	}

	// Check if user already exists in members
	if room.IsMember(user.ID) {
		return nil
	}

	room.Members = append(room.Members, user)

	updatedData, err := json.Marshal(room)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, roomKey, updatedData, 0).Err()
}

func (r *roomRepository) Create(ctx context.Context, room *model.Room) error {
	room.CreatedAt = time.Now()
	data, err := json.Marshal(room)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("room:%s", room.ID)
	if err := r.client.Set(ctx, key, data, 0).Err(); err != nil {
		return err
	}

	return r.client.SAdd(ctx, "rooms", room.ID).Err()
}

func (r *roomRepository) Delete(ctx context.Context, id string) error {
	key := fmt.Sprintf("room:%s", id)
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return err
	}

	return r.client.SRem(ctx, "rooms", id).Err()
}

func (r *roomRepository) GetAll(ctx context.Context) ([]*model.Room, error) {
	roomIDs, err := r.client.SMembers(ctx, "rooms").Result()
	if err != nil {
		return nil, err
	}

	rooms := make([]*model.Room, 0, len(roomIDs))
	for _, id := range roomIDs {
		room, err := r.GetByID(ctx, id)
		if err != nil {
			continue // Skip rooms that can't be retrieved
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

func (r *roomRepository) GetByID(ctx context.Context, id string) (*model.Room, error) {
	key := fmt.Sprintf("room:%s", id)
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var room model.Room
	if err := json.Unmarshal(data, &room); err != nil {
		return nil, err
	}

	userIDs, err := r.GetUsers(ctx, id)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	room.Members = make([]model.User, 0, len(userIDs))
	for _, userID := range userIDs {
		user, err := r.userRepository.GetByID(ctx, userID)
		if err != nil {
			// User might have been deleted, skip them
			continue
		}
		room.Members = append(room.Members, *user)
	}

	return &room, nil
}

func (r *roomRepository) GetUsers(ctx context.Context, roomID string) ([]string, error) {
	key := fmt.Sprintf("room:%s:users", roomID)
	return r.client.SMembers(ctx, key).Result()
}

func (r *roomRepository) RemoveUser(ctx context.Context, roomID string, userID string) error {
	userKey := fmt.Sprintf("room:%s:users", roomID)
	if err := r.client.SRem(ctx, userKey, userID).Err(); err != nil {
		return err
	}

	roomKey := fmt.Sprintf("room:%s", roomID)
	data, err := r.client.Get(ctx, roomKey).Bytes()
	if err != nil {
		return err
	}

	var room model.Room
	if err := json.Unmarshal(data, &room); err != nil {
		return err
	}

	for i, member := range room.Members {
		if member.ID == userID {
			room.Members = append(room.Members[:i], room.Members[i+1:]...)
			break
		}
	}

	updatedData, err := json.Marshal(room)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, roomKey, updatedData, 0).Err()
}

func (r *roomRepository) Update(ctx context.Context, room *model.Room) error {
	key := fmt.Sprintf("room:%s", room.ID)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("room with id %s does not exist", room.ID)
	}

	data, err := json.Marshal(room)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, data, 0).Err()
}
