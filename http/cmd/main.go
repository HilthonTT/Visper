package main

import (
	"context"
	"expvar"
	"log"
	"runtime"
	"time"

	"github.com/hilthontt/visper/internal/infrastructure/configs"
	"github.com/hilthontt/visper/internal/infrastructure/env"
	"github.com/hilthontt/visper/internal/infrastructure/events"
	"github.com/hilthontt/visper/internal/infrastructure/logging"
	"github.com/hilthontt/visper/internal/infrastructure/messaging"
	"github.com/hilthontt/visper/internal/infrastructure/ratelimiter"
	"github.com/hilthontt/visper/internal/infrastructure/tracing"
	"github.com/hilthontt/visper/internal/infrastructure/ws"
	"github.com/hilthontt/visper/internal/persistence/db"
	"github.com/hilthontt/visper/internal/persistence/repository"
	"github.com/hilthontt/visper/internal/presentation/api"
	"github.com/hilthontt/visper/internal/presentation/handler/health"
	"github.com/hilthontt/visper/internal/presentation/handler/messages"
	"github.com/hilthontt/visper/internal/presentation/handler/rooms"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	serviceName = "visper-api"
	version     = "1.0.0"
)

// @title           Visper API
// @version         1.0
// @description     Anonymous chat room API with WebSocket support
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api

// @securityDefinitions.apikey MemberAuth
// @in cookie
// @name member_id

// @tag.name rooms
// @tag.description Room management operations

// @tag.name messages
// @tag.description Message operations

// @tag.name health
// @tag.description Health check endpoints
func main() {
	tracerCfg := tracing.NewDefaultConfig(serviceName)
	sh, err := tracing.InitTracer(tracerCfg)
	if err != nil {
		log.Fatalf("Failed to initialize the tracer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer sh(ctx)

	logger := logging.NewLogger(logging.NewDefaultConfig())
	cfg := configs.Load()

	mongoConfig := db.NewMongoDefaultConfig()
	mongoClient, err := db.NewMongoClient(ctx, mongoConfig)
	if err != nil {
		log.Fatal(err)
	}

	mongoDatabase := db.GetDatabase(mongoClient, mongoConfig)
	auditRepo := repository.NewRoomAuditLogRepository(mongoDatabase)

	roomRepository := repository.NewRoomRepository(cfg.RoomStore.Capacity, time.Hour)
	messageRepository := repository.NewMessageRepository(cfg.MessageStore.Capacity)

	roomManager := ws.NewRoomManager()
	wsCore := ws.NewCore(roomRepository, messageRepository)
	go wsCore.Run()

	rabbitMqURI := env.GetString("RABBITMQ_URI", "amqp://guest:guest@localhost:5672/")
	rabbitmq, err := messaging.NewRabbitMQ(rabbitMqURI)
	if err != nil {
		log.Fatal(err)
	}
	defer rabbitmq.Close()

	logger.Info(logging.RabbitMQ, logging.Startup, "Started", nil)

	roomPublisher := events.NewRoomPublisher(rabbitmq)

	// Start Room Consumer
	roomConsumer := events.NewRoomConsumer(rabbitmq, auditRepo)
	go roomConsumer.Listen()

	roomHandler := rooms.NewHandler(roomRepository, messageRepository, roomManager, wsCore, roomPublisher)
	healthHandler := health.NewHandler()
	messageHandler := messages.NewHandler(roomRepository, messageRepository, roomManager, wsCore)

	rl := ratelimiter.New(ratelimiter.Options{
		MaxRatePerSecond: cfg.RateLimiter.MaxRatePerSecond,
		MaxBurst:         cfg.RateLimiter.MaxBurst,
		SourceHeaderKey:  cfg.RateLimiter.SourceHeaderKey,
	})
	app := api.NewApplication(*cfg, *roomHandler, *healthHandler, *messageHandler, logger, rl)

	// Metrics collected
	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))
	expvar.Publish("database", expvar.Func(func() any {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var result bson.M
		err := mongoClient.Database("admin").RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&result)
		if err != nil {
			return map[string]any{
				"error": err.Error(),
			}
		}
		return result
	}))
	expvar.NewString("version").Set(version)

	mux := app.Mount()

	if err := app.Run(mux, version); err != nil {
		logger.Fatal(logging.General, logging.Startup, err.Error(), nil)
	}
}
