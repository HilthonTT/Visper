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

type userRepository struct {
	client *redis.Client
}

func NewUserRepository(client *redis.Client) repository.UserRepository {
	return &userRepository{
		client: client,
	}
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	user.CreatedAt = time.Now()
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("user:%s", user.ID)
	return r.client.Set(ctx, key, data, 0).Err()
}

func (r *userRepository) Delete(ctx context.Context, id string) error {
	key := fmt.Sprintf("user:%s", id)
	return r.client.Del(ctx, key).Err()
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	key := fmt.Sprintf("user:%s", id)
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var user model.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	key := fmt.Sprintf("user:username:%s", username)
	userID, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	return r.GetByID(ctx, userID)
}

func (r *userRepository) SetUsernameIndex(ctx context.Context, username string, userID string) error {
	key := fmt.Sprintf("user:username:%s", username)
	return r.client.Set(ctx, key, userID, 0).Err()
}
