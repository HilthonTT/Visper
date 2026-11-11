package dependency

import (
	"context"
	"fmt"

	fileUseCase "github.com/hilthontt/visper/services/api/application/usecases/file"
	messageUseCase "github.com/hilthontt/visper/services/api/application/usecases/message"
	roomUseCase "github.com/hilthontt/visper/services/api/application/usecases/room"
	userUseCase "github.com/hilthontt/visper/services/api/application/usecases/user"
	"github.com/hilthontt/visper/services/api/domain/repository"
	"github.com/hilthontt/visper/services/api/infrastructure/broker"
	"github.com/hilthontt/visper/services/api/infrastructure/cache"
	"github.com/hilthontt/visper/services/api/infrastructure/config"
	"github.com/hilthontt/visper/services/api/infrastructure/events"
	"github.com/hilthontt/visper/services/api/infrastructure/jobs"
	"github.com/hilthontt/visper/services/api/infrastructure/logger"
	"github.com/hilthontt/visper/services/api/infrastructure/metrics"
	"github.com/hilthontt/visper/services/api/infrastructure/profiler"
	"github.com/hilthontt/visper/services/api/infrastructure/security"
	"github.com/hilthontt/visper/services/api/infrastructure/storage"
	"github.com/hilthontt/visper/services/api/infrastructure/websocket"
	"github.com/hilthontt/visper/services/api/presentation/controllers/file"
	"github.com/hilthontt/visper/services/api/presentation/controllers/message"
	"github.com/hilthontt/visper/services/api/presentation/controllers/room"
	wsCtrl "github.com/hilthontt/visper/services/api/presentation/controllers/websocket"
	"github.com/hilthontt/visper/services/api/presentation/middlewares"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Container struct {
	Config *config.Config
	Logger *logger.Logger

	TracerProvider *trace.TracerProvider
	MetricsManager metrics.Manager

	MessageRepo  repository.MessageRepository
	UserRepo     repository.UserRepository
	RoomRepo     repository.RoomRepository
	FileRepo     repository.FileRepository
	AuditLogRepo repository.AuditLogRepository

	WSRoomManager    *websocket.RoomManager
	WSCore           *websocket.Core
	NotificationCore *websocket.NotificationCore

	MessageUC messageUseCase.MessageUseCase
	RoomUC    roomUseCase.RoomUseCase
	UserUC    userUseCase.UserUseCase
	FileUC    fileUseCase.FileUseCase

	MessageController          message.MessageController
	RoomController             room.RoomController
	WebsocketController        wsCtrl.WebSocketController
	FilesController            file.FilesController
	UserNotificationController wsCtrl.UserNotificationController

	ETagStore middlewares.ETagStore
	Storage   *storage.LocalStorage

	FileCleanupJob   *jobs.FileCleanupJob
	RoomExpiryJob    *jobs.RoomExpiryJob
	Profiler         *profiler.AdaptiveProfiler
	DistributedCache *cache.DistributedCache

	EventConsumer  *events.EventConsumer
	EventPublisher *events.EventPublisher
	Broker         *broker.Broker

	// ctx is cancelled by Shutdown and stops every background goroutine
	// (websocket hubs, profiler, cleanup job).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewContainer wires all dependencies. ctx is the application lifetime
// context: cancelling it (or calling Shutdown) stops background workers.
func NewContainer(ctx context.Context) (*Container, error) {
	c := &Container{}
	c.ctx, c.cancel = context.WithCancel(ctx)

	c.Config = config.GetConfig()

	// Secure cookies everywhere except development, which may run over HTTP.
	// Secure cookies are only sent over HTTPS; marking them Secure on the
	// plain-HTTP release/compose setup made browsers drop the user cookie.
	security.SetSecureCookies(c.Config.RequiresHTTPS())

	loggerInstance, err := logger.NewDevelopmentLogger()
	if err != nil {
		return nil, fmt.Errorf("error initializing logger: %w", err)
	}
	c.Logger = loggerInstance

	c.Logger.Info("Initializing Visper API dependencies")

	if err := cache.InitRedis(c.Config); err != nil {
		return nil, fmt.Errorf("error initializing cache: %w", err)
	}

	if err := c.initInfrastructure(); err != nil {
		return nil, fmt.Errorf("error initializing infrastructure: %w", err)
	}

	c.initRepositories()

	if err := c.initBroker(); err != nil {
		return nil, fmt.Errorf("error initializing broker: %w", err)
	}

	c.initWebSocket()
	c.initUseCases()
	c.initMiddleware()
	c.initControllers()
	c.initBackgroundJobs()
	c.initProfile()

	c.Logger.Info("All dependencies initialized successfully")

	return c, nil
}
