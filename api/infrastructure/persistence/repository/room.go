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
	client *redis.Client
}

func NewRoomRepository(client *redis.Client) repository.RoomRepository {
	return &roomRepository{client: client}
}

func (r *roomRepository) AddUser(ctx context.Context, roomID string, userID string) error {
	key := fmt.Sprintf("room:%s:users", roomID)
	return r.client.SAdd(ctx, key, userID).Err()
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

	return &room, nil
}

func (r *roomRepository) GetUsers(ctx context.Context, roomID string) ([]string, error) {
	key := fmt.Sprintf("room:%s:users", roomID)
	return r.client.SMembers(ctx, key).Result()
}

func (r *roomRepository) RemoveUser(ctx context.Context, roomID string, userID string) error {
	key := fmt.Sprintf("room:%s:users", roomID)
	return r.client.SRem(ctx, key, userID).Err()
}
