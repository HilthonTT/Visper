package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/cache"
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

	if err := r.cache.Set(ctx, key, user, 0); err != nil {
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

	if err := r.cache.Delete(ctx, key); err != nil {
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

	found, err := r.cache.Get(ctx, key, &user)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get user from cache")
		return nil, err
	}

	if !found {
		span.SetAttributes(attribute.Bool("user.found", false))
		span.SetStatus(codes.Error, "user not found")
		return nil, repository.ErrNotFound
	}

	span.SetAttributes(
		attribute.Bool("user.found", true),
		attribute.String("user.username", user.Username),
	)
	span.SetStatus(codes.Ok, "user retrieved successfully")
	return &user, nil
}
