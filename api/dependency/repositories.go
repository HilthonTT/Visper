package dependency

import (
	"github.com/hilthontt/visper/api/infrastructure/cache"
	"github.com/hilthontt/visper/api/infrastructure/persistence/repository"
)

func (c *Container) initRepositories() {
	redisClient := cache.GetRedis()

	c.MessageRepo = repository.NewMessageRepository(redisClient)
	c.UserRepo = repository.NewUserRepository(redisClient)
	c.RoomRepo = repository.NewRoomRepository(redisClient, c.UserRepo)
	c.FileRepo = repository.NewFileRepository(redisClient, c.RoomRepo)

	c.Logger.Info("Repositories initialized successfully")
}
