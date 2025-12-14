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
	"github.com/hilthontt/visper/internal/infrastructure/messaging"
	"github.com/hilthontt/visper/internal/infrastructure/ratelimiter"
	"github.com/hilthontt/visper/internal/infrastructure/repository"
	"github.com/hilthontt/visper/internal/infrastructure/tracing"
	"github.com/hilthontt/visper/internal/infrastructure/ws"
	"github.com/hilthontt/visper/internal/presentation/api"
	"github.com/hilthontt/visper/internal/presentation/handler/health"
	"github.com/hilthontt/visper/internal/presentation/handler/messages"
	"github.com/hilthontt/visper/internal/presentation/handler/rooms"
	"go.uber.org/zap"
)

const (
	serviceName = "visper-api"
)

func main() {
	tracerCfg := tracing.NewDefaultConfig(serviceName)

	sh, err := tracing.InitTracer(tracerCfg)
	if err != nil {
		log.Fatalf("Failed to initialize the tracer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer sh(ctx)

	logger := zap.Must(zap.NewProduction()).Sugar()
	defer logger.Sync()

	configPath := configs.DetermineConfigPath()
	cfg, err := configs.Load(configPath)
	if err != nil {
		log.Fatal(err)
	}

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

	log.Println("Starting RabbitMQ connection")

	roomPublisher := events.NewRoomPublisher(rabbitmq)

	// Start Room Consumer
	roomConsumer := events.NewRoomConsumer(rabbitmq)
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

	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))

	mux := app.Mount()
	logger.Fatal(app.Run(mux))
}
