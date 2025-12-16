package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hilthontt/visper/internal/infrastructure/env"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	RoomAuditLogsCollection = "room_audit_logs"

	DefaultDatabase          = "visper"
	DefaultConnectionTimeout = 20 * time.Second
)

type MongoConfig struct {
	URI               string
	Database          string
	ConnectionTimeout time.Duration
}

func NewMongoDefaultConfig() *MongoConfig {
	uri := env.GetString("MONGODB_URI", "mongodb://localhost:27017")
	database := env.GetString("MONGODB_DATABASE", DefaultDatabase)

	return &MongoConfig{
		URI:               uri,
		Database:          database,
		ConnectionTimeout: DefaultConnectionTimeout,
	}
}

func NewMongoClient(ctx context.Context, cfg *MongoConfig) (*mongo.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mongodb config is required")
	}
	if cfg.URI == "" {
		return nil, fmt.Errorf("mongodb URI is required")
	}
	if cfg.Database == "" {
		return nil, fmt.Errorf("mongodb database is required")
	}

	connectCtx, cancel := context.WithTimeout(ctx, cfg.ConnectionTimeout)
	defer cancel()

	clientOpts := options.Client().
		ApplyURI(cfg.URI).
		SetServerSelectionTimeout(cfg.ConnectionTimeout).
		SetConnectTimeout(cfg.ConnectionTimeout)

	client, err := mongo.Connect(connectCtx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb: %w", err)
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, cfg.ConnectionTimeout)
	defer pingCancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("failed to ping mongodb: %w", err)
	}

	log.Printf("Successfully connected to the MongoDB database: %s", cfg.Database)
	return client, nil
}

func GetDatabase(client *mongo.Client, cfg *MongoConfig) *mongo.Database {
	if client == nil || cfg == nil {
		return nil
	}
	return client.Database(cfg.Database)
}

func DisconnectMongo(ctx context.Context, client *mongo.Client) error {
	if client == nil {
		return nil
	}

	disconnectCtx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	if err := client.Disconnect(disconnectCtx); err != nil {
		return fmt.Errorf("failed to disconnect from mongodb: %w", err)
	}

	log.Println("Disconnected from MongoDB")
	return nil
}
