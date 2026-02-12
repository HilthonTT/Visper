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

type userRepository struct {
	cache  *cache.DistributedCache
	tracer trace.Tracer
}

func NewUserRepository(cache *cache.DistributedCache, tracer trace.Tracer) repository.UserRepository {
	return &userRepository{
		cache:  cache,
		tracer: tracer,
	}
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	ctx, span := r.tracer.Start(ctx, "userRepository.Create")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.id", user.ID),
		attribute.String("user.username", user.Username),
	)

	user.CreatedAt = time.Now()
	key := fmt.Sprintf("user:%s", user.ID)

	if err := r.cache.Set(key, user, 0); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create user")
		return err
	}

	span.SetStatus(codes.Ok, "user created successfully")
	return nil
}

func (r *userRepository) Delete(ctx context.Context, id string) error {
	ctx, span := r.tracer.Start(ctx, "userRepository.Delete")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", id))

	key := fmt.Sprintf("user:%s", id)

	if err := r.cache.Delete(key); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete user")
		return err
	}

	span.SetStatus(codes.Ok, "user deleted successfully")
	return nil
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	ctx, span := r.tracer.Start(ctx, "userRepository.GetByID")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", id))

	key := fmt.Sprintf("user:%s", id)
	var user model.User

	found, err := r.cache.Get(key, &user)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get user from cache")
		return nil, err
	}

	if !found {
		span.SetAttributes(attribute.Bool("user.found", false))
		span.SetStatus(codes.Error, "user not found")
		return nil, redis.Nil
	}

	span.SetAttributes(
		attribute.Bool("user.found", true),
		attribute.String("user.username", user.Username),
	)
	span.SetStatus(codes.Ok, "user retrieved successfully")
	return &user, nil
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	ctx, span := r.tracer.Start(ctx, "userRepository.GetByUsername")
	defer span.End()

	span.SetAttributes(attribute.String("user.username", username))

	indexKey := fmt.Sprintf("user:username:%s", username)
	var userID string

	found, err := r.cache.Get(indexKey, &userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get username index from cache")
		return nil, err
	}

	if !found {
		span.SetAttributes(attribute.Bool("username.index.found", false))
		span.SetStatus(codes.Error, "username index not found")
		return nil, fmt.Errorf("user not found")
	}

	span.SetAttributes(
		attribute.Bool("username.index.found", true),
		attribute.String("user.id", userID),
	)

	// GetByID will create its own span
	user, err := r.GetByID(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get user by ID after username lookup")
		return nil, err
	}

	span.SetStatus(codes.Ok, "user retrieved by username successfully")
	return user, nil
}

func (r *userRepository) SetUsernameIndex(ctx context.Context, username string, userID string) error {
	ctx, span := r.tracer.Start(ctx, "userRepository.SetUsernameIndex")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.username", username),
		attribute.String("user.id", userID),
	)

	key := fmt.Sprintf("user:username:%s", username)

	if err := r.cache.Set(key, userID, 0); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to set username index")
		return err
	}

	span.SetStatus(codes.Ok, "username index set successfully")
	return nil
}
