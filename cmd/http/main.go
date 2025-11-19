package main

import (
	"expvar"
	"log"
	"runtime"
	"time"

	"github.com/hilthontt/visper/internal/infrastructure/configs"
	"github.com/hilthontt/visper/internal/infrastructure/ratelimiter"
	"github.com/hilthontt/visper/internal/infrastructure/repository"
	"github.com/hilthontt/visper/internal/infrastructure/ws"
	"github.com/hilthontt/visper/internal/presentation/api"
	"github.com/hilthontt/visper/internal/presentation/handler/health"
	"github.com/hilthontt/visper/internal/presentation/handler/rooms"
	"go.uber.org/zap"
)

func main() {
	logger := zap.Must(zap.NewProduction()).Sugar()
	defer logger.Sync()

	configPath := configs.DetermineConfigPath()
	cfg, err := configs.Load(configPath)
	if err != nil {
		log.Fatal(err)
	}

	// TODO: Add these magic numbers to a config file
	roomRepository := repository.NewRoomRepository(100, time.Hour)
	messageRepository := repository.NewMessageRepository(100)

	roomManager := ws.NewRoomManager()
	wsCore := ws.NewCore(roomRepository, messageRepository)
	go wsCore.Run()

	roomHandler := rooms.NewHandler(roomRepository, messageRepository, roomManager, wsCore)
	healthHandler := health.NewHandler()
	rateLimiter := ratelimiter.NewFixedWindowRateLimiter(cfg.RateLimiter.RequestsPerTimeFrame, cfg.RateLimiter.TimeFrame)
	app := api.NewApplication(*cfg, *roomHandler, *healthHandler, logger, rateLimiter)

	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))

	mux := app.Mount()
	logger.Fatal(app.Run(mux))
}
