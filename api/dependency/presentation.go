package dependency

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/hilthontt/visper/api/infrastructure/cache"
	"github.com/hilthontt/visper/api/infrastructure/metrics"
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
	c.RoomController = room.NewRoomController(c.RoomUC, c.UserUC, c.WSRoomManager, c.WSCore)
	c.WebsocketController = wsCtrl.NewWebSocketController(c.RoomUC, c.UserUC, c.WSRoomManager, c.WSCore)

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

		routes.MessageRoutes(v1, c.MessageController)
		routes.RoomRoutes(v1, c.RoomController)
		routes.WebsocketRoutes(v1, c.WebsocketController)
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

	if err := c.Logger.Log.Sync(); err != nil {
		c.Logger.Error("failed to sync logger", zap.Error(err))
	}

	c.Logger.Info("Dependencies shut down successfully")

	return nil
}
