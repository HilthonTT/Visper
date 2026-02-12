package dependency

import (
	"github.com/hilthontt/visper/api/infrastructure/cache"
	"github.com/hilthontt/visper/api/infrastructure/persistence/repository"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	// Cache
	CacheKeyPrefix = "visper:"

	// Tracer
	RepoTracerName = "github.com/hilthontt/visper/api/repository"
)

func (c *Container) initRepositories() {
	redisClient := cache.GetRedis()
	distributedCache := cache.NewDistributedCache(redisClient, CacheKeyPrefix, cache.DefaultOptions())
	c.DistributedCache = distributedCache

	// Create tracer for repositories with fallback to noop tracer
	var tracer trace.Tracer
	if c.TracerProvider != nil {
		tracer = c.TracerProvider.Tracer(
			RepoTracerName,
			trace.WithInstrumentationVersion("1.0.0"),
		)
	} else {
		// Use noop tracer if TracerProvider is nil
		c.Logger.Warn("TracerProvider is nil, using noop tracer for repositories")
		tracer = noop.NewTracerProvider().Tracer(RepoTracerName)
		// Or use global tracer provider as fallback
		// tracer = otel.GetTracerProvider().Tracer(RepoTracerName)
	}

	c.MessageRepo = repository.NewMessageRepository(distributedCache, tracer)
	c.UserRepo = repository.NewUserRepository(distributedCache, tracer)
	c.RoomRepo = repository.NewRoomRepository(distributedCache, c.UserRepo, tracer)
	c.FileRepo = repository.NewFileRepository(redisClient, c.RoomRepo)

	c.Logger.Info("Repositories initialized successfully")
}
