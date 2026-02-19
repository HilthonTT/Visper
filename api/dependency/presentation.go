package dependency

import (
	"context"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/hilthontt/visper/api/infrastructure/cache"
	"github.com/hilthontt/visper/api/infrastructure/metrics"
	"github.com/hilthontt/visper/api/infrastructure/persistence/database"
	"github.com/hilthontt/visper/api/presentation/controllers/file"
	"github.com/hilthontt/visper/api/presentation/controllers/message"
	"github.com/hilthontt/visper/api/presentation/controllers/room"
	wsCtrl "github.com/hilthontt/visper/api/presentation/controllers/websocket"
	"github.com/hilthontt/visper/api/presentation/middlewares"
	"github.com/hilthontt/visper/api/presentation/routes"
	"go.uber.org/zap"
)

func (c *Container) initMiddleware() {
	c.ETagStore = middlewares.NewInMemoryETagStore()

	c.Logger.Info("Middleware components initialized successfully")
}

func (c *Container) initControllers() {
	c.MessageController = message.NewMessageController(c.MessageUC, c.RoomUC, c.WSRoomManager, c.WSCore)
	c.RoomController = room.NewRoomController(c.RoomUC, c.UserUC, c.WSRoomManager, c.WSCore, c.Config)
	c.WebsocketController = wsCtrl.NewWebSocketController(c.RoomUC, c.UserUC, c.WSRoomManager, c.WSCore)
	c.FilesController = file.NewFilesController(c.FileUC, c.Storage)
	c.UserNotificationController = wsCtrl.NewUserNotificationController(c.UserUC, c.RoomUC, c.NotificationCore)

	c.Logger.Info("Controllers initialized successfully")
}

func (c *Container) SetupRouter() *gin.Engine {
	switch c.Config.Server.RunMode {
	case "release", "production":
		gin.SetMode(gin.ReleaseMode)
	case "test":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	binding.Validator = new(middlewares.DefaultValidator)

	router := gin.Default()

	router.Use(sentrygin.New(sentrygin.Options{
		Repanic:         true,
		WaitForDelivery: false,
		Timeout:         5 * time.Second,
	}))

	if c.Config.IsProduction() {
		router.Use(middlewares.ForceHttps(c.Config))
	}

	router.Use(middlewares.GinLogger(c.Logger))
	router.Use(middlewares.CorsMiddleware(c.Config))

	router.GET("/health", c.healthCheckHandler)

	c.registerObservabilityRoutes(router)

	c.registerAPIRoutes(router)

	c.Logger.Info("Router configured successfully")

	return router
}

func (c *Container) registerAPIRoutes(router *gin.Engine) {
	v1 := router.Group("/api/v1")
	{

		v1.Use(middlewares.RateLimiterMiddleware(cache.GetRedis(), c.Logger, middlewares.ModerateRateLimiterConfig()))
		v1.Use(middlewares.ETagMiddleware(c.ETagStore))
		v1.Use(middlewares.UserMiddleware(c.UserUC, c.Logger))

		v1.Use(func(c *gin.Context) {
			if hub := sentrygin.GetHubFromContext(c); hub != nil {
				user, exists := middlewares.GetUserFromContext(c)
				if !exists {
					hub.Scope().SetUser(sentry.User{
						ID:        user.ID,
						Username:  user.Username,
						IPAddress: c.ClientIP(),
					})
				}

				hub.Scope().SetTag("user_type", "anonymous")
			}
			c.Next()
		})

		routes.FilesRoute(v1, c.FilesController, c.Logger)
		routes.MessageRoutes(v1, c.MessageController)
		routes.RoomRoutes(v1, c.RoomController)
		routes.WebsocketRoutes(v1, c.WebsocketController, c.UserNotificationController)
	}
}

func (c *Container) healthCheckHandler(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (c *Container) registerObservabilityRoutes(router *gin.Engine) {
	metricsGroup := router.Group("/observability")
	{
		metrics.GetHandler(metricsGroup, c.MetricsManager)
	}
}

func (c *Container) Shutdown() error {
	c.Logger.Info("Shutting down dependencies...")

	if c.FileCleanupJob != nil {
		c.FileCleanupJob.Stop()
	}

	// Cancel WebSocket context
	if c.cancel != nil {
		c.cancel()
	}

	if c.TracerProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.TracerProvider.Shutdown(ctx); err != nil {
			c.Logger.Error("failed to shutdown tracer provider", zap.Error(err))
		}
	}

	cache.CloseRedis()
	c.DistributedCache.Close()

	if err := c.Logger.Log.Sync(); err != nil {
		c.Logger.Error("failed to sync logger", zap.Error(err))
	}

	c.Logger.Info("Dependencies shut down successfully")

	database.CloseDb()

	return nil
}
