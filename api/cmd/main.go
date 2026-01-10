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
	"github.com/hilthontt/visper/api/infrastructure/metrics"
	"github.com/hilthontt/visper/api/infrastructure/metrics/exporters"
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

	tracerProvider, err := exporters.InitJaegerExporter(cfg)
	if err != nil {
		loggerInstance.Error("failed to initialize Jaeger exporter", zap.Error(err))
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := tracerProvider.Shutdown(ctx); err != nil {
				loggerInstance.Error("failed to shutdown tracer provider", zap.Error(err))
			}
		}()
		loggerInstance.Info("Jaeger exporter initialized successfully",
			zap.String("endpoint", cfg.Jaeger.Endpoint),
			zap.String("service", cfg.Jaeger.ServiceName),
		)

		// Send startup telemetry
		go exporters.SendTelemetryTrace(cfg)
	}

	meter := exporters.Prometheus(cfg.Jaeger.ServiceName, cfg.Jaeger.ServiceVersion)
	if meter == nil {
		loggerInstance.Panic("failed to initialize Prometheus exporter")
	}

	metricsManager := metrics.NewMetricsManager(meter, loggerInstance)

	// Register system metrics gauges
	metricsManager.NewGauge("app_go_routines", "Number of goroutines")
	metricsManager.NewGauge("app_sys_memory_alloc", "Bytes allocated and in use")
	metricsManager.NewGauge("app_sys_total_alloc", "Total bytes allocated")
	metricsManager.NewGauge("app_go_numGC", "Number of completed GC cycles")
	metricsManager.NewGauge("app_go_sys", "Total bytes of memory obtained from OS")

	// Register application metrics
	metricsManager.NewCounter("http_requests_total", "Total number of HTTP requests")
	metricsManager.NewHistogram("http_request_duration_seconds", "HTTP request duration in seconds",
		0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0)
	metricsManager.NewUpDownCounter("active_websocket_connections", "Number of active WebSocket connections")
	metricsManager.NewCounter("websocket_messages_sent", "Total number of WebSocket messages sent")
	metricsManager.NewCounter("websocket_messages_received", "Total number of WebSocket messages received")

	loggerInstance.Info("Metrics initialized successfully")

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

	metricsGroup := router.Group("/observability")
	{
		metrics.GetHandler(metricsGroup, metricsManager)
	}

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

	eTagStore := middlewares.NewInMemoryETagStore()

	v1 := router.Group("/api/v1")
	{
		v1.Use(middlewares.RateLimiterMiddleware(cache.GetRedis(), loggerInstance, middlewares.ModerateRateLimiterConfig()))
		v1.Use(middlewares.ETagMiddleware(eTagStore))
		v1.Use(middlewares.UserMiddleware(userUC, loggerInstance))

		messageController := message.NewMessageController(messageUC, roomUC, wsRoomManager, wsCore)
		roomController := room.NewRoomController(roomUC, userUC, wsRoomManager, wsCore)
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
		zap.String("metrics_url", fmt.Sprintf("http://%s:%s/observability/metrics", cfg.Server.Domain, cfg.Server.ExternalPort)),
		zap.String("pprof_url", fmt.Sprintf("http://%s:%s/observability/debug/pprof/", cfg.Server.Domain, cfg.Server.ExternalPort)),
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
