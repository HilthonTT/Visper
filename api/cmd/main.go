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
	messageUseCase "github.com/hilthontt/visper/api/application/usecases/message"
	roomUseCase "github.com/hilthontt/visper/api/application/usecases/room"
	userUseCase "github.com/hilthontt/visper/api/application/usecases/user"
	"github.com/hilthontt/visper/api/infrastructure/cache"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	repositories "github.com/hilthontt/visper/api/infrastructure/persistence/repository"
	"github.com/hilthontt/visper/api/presentation/controllers/message"
	"github.com/hilthontt/visper/api/presentation/controllers/room"
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

	router := gin.New()

	router.Use(gin.Recovery())
	router.Use(GinLogger(loggerInstance))
	router.Use(CorsMiddleware(cfg))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	messageRepo := repositories.NewMessageRepository(cache.GetRedis())
	userRepo := repositories.NewUserRepository(cache.GetRedis())
	roomRepo := repositories.NewRoomRepository(cache.GetRedis(), userRepo)

	messageUC := messageUseCase.NewMessageUseCase(messageRepo, loggerInstance)
	roomUC := roomUseCase.NewRoomUseCase(roomRepo, loggerInstance)
	userUC := userUseCase.NewUserUseCase(userRepo, loggerInstance)

	v1 := router.Group("/api/v1")
	{
		v1.Use(middlewares.UserMiddleware(userUC, loggerInstance))

		messageController := message.NewMessageController(messageUC, roomUC)
		roomController := room.NewRoomController(roomUC)

		routes.MessageRoutes(v1, messageController)
		routes.RoomRoutes(v1, roomController)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		loggerInstance.Fatal("Server forced to shutdown", zap.Error(err))
	}

	loggerInstance.Info("Server exited successfully")
}

func GinLogger(logger *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		clientIP := c.ClientIP()

		if len(c.Errors) > 0 {
			logger.Error("Request error",
				zap.String("method", method),
				zap.String("path", path),
				zap.String("query", query),
				zap.Int("status", statusCode),
				zap.Duration("latency", latency),
				zap.String("ip", clientIP),
				zap.String("errors", c.Errors.String()),
			)
		} else {
			logger.Info("Request",
				zap.String("method", method),
				zap.String("path", path),
				zap.String("query", query),
				zap.Int("status", statusCode),
				zap.Duration("latency", latency),
				zap.String("ip", clientIP),
			)
		}
	}
}

func CorsMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", cfg.Cors.AllowOrigins)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
