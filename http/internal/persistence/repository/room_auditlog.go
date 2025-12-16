package repository

import (
	"context"
	"time"

	"github.com/hilthontt/visper/internal/domain"
	"github.com/hilthontt/visper/internal/persistence/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type roomAuditLogRepository struct {
	db *mongo.Database
}

func NewRoomAuditLogRepository(db *mongo.Database) domain.RoomAuditRepository {
	return &roomAuditLogRepository{
		db: db,
	}
}

func (r *roomAuditLogRepository) DeleteOlderThan(ctx context.Context, before time.Time) error {
	collection := r.db.Collection(db.RoomAuditLogsCollection)

	filter := bson.M{
		"timestamp": bson.M{
			"$lt": before,
		},
	}

	_, err := collection.DeleteMany(ctx, filter)
	return err
}

func (r *roomAuditLogRepository) GetByEventType(ctx context.Context, eventType domain.RoomEventType, from time.Time, to time.Time) ([]domain.RoomAuditLog, error) {
	collection := r.db.Collection(db.RoomAuditLogsCollection)

	filter := bson.M{
		"event_type": eventType,
		"timestamp": bson.M{
			"$gte": from,
			"$lte": to,
		},
	}

	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []domain.RoomAuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}

	return logs, nil
}

func (r *roomAuditLogRepository) GetByRoomID(ctx context.Context, roomID string, limit int) ([]domain.RoomAuditLog, error) {
	collection := r.db.Collection(db.RoomAuditLogsCollection)

	filter := bson.M{"room_id": roomID}
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []domain.RoomAuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}

	return logs, nil
}

func (r *roomAuditLogRepository) Log(ctx context.Context, log *domain.RoomAuditLog) error {
	collection := r.db.Collection(db.RoomAuditLogsCollection)

	_, err := collection.InsertOne(ctx, log)
	return err
}

func (r *roomAuditLogRepository) EnsureIndexes(ctx context.Context) error {
	collection := r.db.Collection(db.RoomAuditLogsCollection)

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "room_id", Value: 1},
				{Key: "timestamp", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "event_type", Value: 1},
				{Key: "timestamp", Value: -1},
			},
		},
		{
			Keys:    bson.D{{Key: "timestamp", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(7776000), // 90 days TTL
		},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexes)
	return err
}
