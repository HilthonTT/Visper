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

type fileRepository struct {
	client         *redis.Client
	roomRepository repository.RoomRepository
}

func NewFileRepository(client *redis.Client, roomRepository repository.RoomRepository) repository.FileRepository {
	return &fileRepository{
		client:         client,
		roomRepository: roomRepository,
	}
}

func (r *fileRepository) Create(ctx context.Context, file *model.File) error {
	file.CreatedAt = time.Now()
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("file:%s", file.ID)
	if err := r.client.Set(ctx, key, data, 0).Err(); err != nil {
		return err
	}

	roomFileKey := fmt.Sprintf("room:%s:files", file.RoomID)
	if err := r.client.SAdd(ctx, roomFileKey, file.ID).Err(); err != nil {
		return err
	}

	return r.client.SAdd(ctx, "files", file.ID).Err()
}

func (r *fileRepository) GetByID(ctx context.Context, id string) (*model.File, error) {
	key := fmt.Sprintf("file:%s", id)
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var file model.File
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	return &file, nil
}

func (r *fileRepository) GetByRoomID(ctx context.Context, roomID string) ([]*model.File, error) {
	roomFileKey := fmt.Sprintf("room:%s:files", roomID)
	fileIDs, err := r.client.SMembers(ctx, roomFileKey).Result()
	if err != nil {
		return nil, err
	}

	files := make([]*model.File, 0, len(fileIDs))
	for _, id := range fileIDs {
		file, err := r.GetByID(ctx, id)
		if err != nil {
			continue // Skip files that can't be retrieved
		}
		files = append(files, file)
	}

	return files, nil
}

func (r *fileRepository) Delete(ctx context.Context, id string) error {
	file, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}

	roomFileKey := fmt.Sprintf("room:%s:files", file.RoomID)
	if err := r.client.SRem(ctx, roomFileKey, id).Err(); err != nil {
		return err
	}

	if err := r.client.SRem(ctx, "files", id).Err(); err != nil {
		return err
	}

	key := fmt.Sprintf("file:%s", id)
	return r.client.Del(ctx, key).Err()
}

func (r *fileRepository) DeleteByRoomID(ctx context.Context, roomID string) error {
	roomFileKey := fmt.Sprintf("room:%s:files", roomID)
	fileIDs, err := r.client.SMembers(ctx, roomFileKey).Result()
	if err != nil {
		return err
	}

	for _, id := range fileIDs {
		if err := r.Delete(ctx, id); err != nil {

			continue
		}
	}

	return r.client.Del(ctx, roomFileKey).Err()
}

func (r *fileRepository) GetOrphanedFiles(ctx context.Context) ([]*model.File, error) {
	fileIDs, err := r.client.SMembers(ctx, "files").Result()
	if err != nil {
		return nil, err
	}

	orphanedFiles := make([]*model.File, 0)
	for _, id := range fileIDs {
		file, err := r.GetByID(ctx, id)
		if err != nil {
			continue
		}

		// Check if room still exists
		_, err = r.roomRepository.GetByID(ctx, file.RoomID)
		if err != nil {
			// Room doesn't exist, file is orphaned
			orphanedFiles = append(orphanedFiles, file)
		}
	}

	return orphanedFiles, nil
}
