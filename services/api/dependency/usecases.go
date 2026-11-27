package dependency

import (
	"fmt"
	"strings"

	"github.com/hilthontt/visper/services/api/application/ports"
	fileUseCase "github.com/hilthontt/visper/services/api/application/usecases/file"
	messageUseCase "github.com/hilthontt/visper/services/api/application/usecases/message"
	roomUseCase "github.com/hilthontt/visper/services/api/application/usecases/room"
	roomKeyUseCase "github.com/hilthontt/visper/services/api/application/usecases/roomkey"
	userUseCase "github.com/hilthontt/visper/services/api/application/usecases/user"
	"github.com/hilthontt/visper/services/api/infrastructure/websocket"
)

func (c *Container) initUseCases() {
	c.Events = ports.NewAsyncEvents(c.EventPublisher, c.Logger)
	notifier := websocket.NewNotifier(c.WSCore)

	c.MessageUC = messageUseCase.NewMessageUseCase(c.MessageRepo, notifier, c.Events, c.Logger)
	c.RoomUC = roomUseCase.NewRoomUseCase(c.RoomRepo, c.RoomKeyRepo, notifier, c.Events, c.Logger)
	c.RoomKeyUC = roomKeyUseCase.New(c.RoomUC, c.RoomKeyRepo, notifier)
	c.UserUC = userUseCase.NewUserUseCase(c.UserRepo, c.Logger)
	c.FileUC = fileUseCase.NewFileUseCase(c.FileRepo, c.RoomRepo, c.UserRepo, c.Storage, c.getServerURL())

	c.Logger.Info("Use cases initialized successfully")
}

func (c *Container) getServerURL() string {
	domain := c.Config.Server.Domain
	port := c.Config.Server.ExternalPort

	// If domain already has a scheme, use it as-is with the port
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		return fmt.Sprintf("%s:%s", strings.TrimRight(domain, "/"), port)
	}

	// Links must use the scheme the API is actually served on. "release" mode
	// (docker-compose) serves plain HTTP; only "production" sits behind TLS.
	// Keying this on debug mode made every file link https:// under compose,
	// so shared images could not be downloaded or previewed.
	scheme := "http"
	if c.Config.RequiresHTTPS() {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s:%s", scheme, domain, port)
}
