package dependency

import (
	"context"
	"fmt"

	fileUseCase "github.com/hilthontt/visper/api/application/usecases/file"
	messageUseCase "github.com/hilthontt/visper/api/application/usecases/message"
	roomUseCase "github.com/hilthontt/visper/api/application/usecases/room"
	userUseCase "github.com/hilthontt/visper/api/application/usecases/user"
	"github.com/hilthontt/visper/api/domain/repository"
	"github.com/hilthontt/visper/api/infrastructure/cache"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"github.com/hilthontt/visper/api/infrastructure/events"
	"github.com/hilthontt/visper/api/infrastructure/jobs"
	"github.com/hilthontt/visper/api/infrastructure/logger"
	"github.com/hilthontt/visper/api/infrastructure/metrics"
	"github.com/hilthontt/visper/api/infrastructure/profiler"
	"github.com/hilthontt/visper/api/infrastructure/storage"
	"github.com/hilthontt/visper/api/infrastructure/websocket"
	"github.com/hilthontt/visper/api/presentation/controllers/file"
	"github.com/hilthontt/visper/api/presentation/controllers/message"
	"github.com/hilthontt/visper/api/presentation/controllers/room"
	wsCtrl "github.com/hilthontt/visper/api/presentation/controllers/websocket"
	"github.com/hilthontt/visper/api/presentation/middlewares"
	"go.opentelemetry.io/otel/sdk/trace"
)

type Container struct {
	Config *config.Config
	Logger *logger.Logger

	TracerProvider *trace.TracerProvider
	MetricsManager metrics.Manager

	MessageRepo repository.MessageRepository
	UserRepo    repository.UserRepository
	RoomRepo    repository.RoomRepository
	FileRepo    repository.FileRepository

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
	Profiler         *profiler.AdaptiveProfiler
	DistributedCache *cache.DistributedCache

	EventConsumer  *events.EventConsumer
	EventPublisher *events.EventPublisher

	ctx    context.Context
	cancel context.CancelFunc
}

func NewContainer(ctx context.Context) (*Container, error) {
	c := &Container{}

	if err := c.initBroker(); err != nil {
		return nil, err
	}

	c.Config = config.GetConfig()

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

	c.initWebSocket()

	c.initUseCases()

	c.initMiddleware()

	c.initControllers()

	wsCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.initBackgroundJobs(wsCtx)

	c.initProfile()

	c.Logger.Info("All dependencies initialized successfully")

	return c, nil
}
