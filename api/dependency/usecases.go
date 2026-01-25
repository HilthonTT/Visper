package dependency

import (
	fileUseCase "github.com/hilthontt/visper/api/application/usecases/file"
	messageUseCase "github.com/hilthontt/visper/api/application/usecases/message"
	roomUseCase "github.com/hilthontt/visper/api/application/usecases/room"
	userUseCase "github.com/hilthontt/visper/api/application/usecases/user"
)

func (c *Container) initUseCases() {
	c.MessageUC = messageUseCase.NewMessageUseCase(c.MessageRepo, c.Logger)
	c.RoomUC = roomUseCase.NewRoomUseCase(c.RoomRepo, c.Logger)
	c.UserUC = userUseCase.NewUserUseCase(c.UserRepo, c.Logger)
	c.FileUC = fileUseCase.NewFileUseCase(c.FileRepo, c.RoomRepo, c.Storage, c.Config.GetServerAddress())

	c.Logger.Info("Use cases initialized successfully")
}
