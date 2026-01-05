package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	messageUseCase "github.com/hilthontt/visper/api/application/usecases/message"
	roomUseCase "github.com/hilthontt/visper/api/application/usecases/room"
	userUseCase "github.com/hilthontt/visper/api/application/usecases/user"
	"github.com/hilthontt/visper/api/infrastructure/cache"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	repositories "github.com/hilthontt/visper/api/infrastructure/persistence/repository"
	"github.com/hilthontt/visper/api/infrastructure/websocket"
	"github.com/hilthontt/visper/api/presentation/controllers/message"
	"github.com/hilthontt/visper/api/presentation/controllers/room"
	wsCtrl "github.com/hilthontt/visper/api/presentation/controllers/websocket"
	"github.com/hilthontt/visper/api/presentation/middlewares"
	"github.com/hilthontt/visper/api/presentation/routes"
	"go.uber.org/zap"
)

func main() {
	cfg := config.GetConfig()
	loggerInstance, err := logger.NewDevelopmentLogger()
	if err != nil {
		log.Fatal(fmt.Errorf("error initializing logger: %w", err))
	}
	defer func() {
		if err := loggerInstance.Log.Sync(); err != nil {
			loggerInstance.Log.Error("failed to sync logger", zap.Error(err))
		}
	}()

	loggerInstance.Info("Starting Visper API")

	err = cache.InitRedis(cfg)
	if err != nil {
		loggerInstance.Panic("error initializing cache", zap.Error(err))
	}
	defer cache.CloseRedis()

	switch cfg.Server.RunMode {
	case "release", "production":
		gin.SetMode(gin.ReleaseMode)
	case "test":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	binding.Validator = new(middlewares.DefaultValidator)
	router := gin.Default()
	router.Use(middlewares.GinLogger(loggerInstance))
	router.Use(middlewares.CorsMiddleware(cfg))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	messageRepo := repositories.NewMessageRepository(cache.GetRedis())
	userRepo := repositories.NewUserRepository(cache.GetRedis())
	roomRepo := repositories.NewRoomRepository(cache.GetRedis(), userRepo)

	wsRoomManager := websocket.NewRoomManager()
	wsCore := websocket.NewCore(roomRepo, messageRepo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go wsCore.Run(ctx)

	messageUC := messageUseCase.NewMessageUseCase(messageRepo, loggerInstance)
	roomUC := roomUseCase.NewRoomUseCase(roomRepo, loggerInstance)
	userUC := userUseCase.NewUserUseCase(userRepo, loggerInstance)

	v1 := router.Group("/api/v1")
	{
		v1.Use(middlewares.UserMiddleware(userUC, loggerInstance))
		v1.Use(middlewares.RateLimiterMiddleware(cache.GetRedis(), loggerInstance, middlewares.ModerateRateLimiterConfig()))

		messageController := message.NewMessageController(messageUC, roomUC, wsRoomManager, wsCore)
		roomController := room.NewRoomController(roomUC, wsRoomManager, wsCore)
		websocketController := wsCtrl.NewWebSocketController(roomUC, userUC, wsRoomManager, wsCore)

		routes.MessageRoutes(v1, messageController)
		routes.RoomRoutes(v1, roomController)
		routes.WebsocketRoutes(v1, websocketController)
	}

	srv := &http.Server{
		Addr:           fmt.Sprintf(":%s", cfg.Server.ExternalPort),
		Handler:        router,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	go func() {
		loggerInstance.Info("Server starting",
			zap.String("port", cfg.Server.ExternalPort),
			zap.String("mode", cfg.Server.RunMode),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			loggerInstance.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	loggerInstance.Info("Server started successfully",
		zap.String("port", cfg.Server.ExternalPort),
		zap.String("domain", cfg.Server.Domain),
	)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	loggerInstance.Info("Shutting down server...")

	if err := srv.Shutdown(ctx); err != nil {
		loggerInstance.Fatal("Server forced to shutdown", zap.Error(err))
	}

	loggerInstance.Info("Server exited successfully")
}
