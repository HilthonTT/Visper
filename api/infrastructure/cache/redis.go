package cache

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hilthontt/visper/api/infrastructure/config"
	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

func InitRedis(cfg *config.Config) error {
	db := 0
	if cfg.Redis.Db != "" {
		parsed, err := strconv.Atoi(cfg.Redis.Db)
		if err != nil {
			return fmt.Errorf("invalid redis.db %q: %w", cfg.Redis.Db, err)
		}
		db = parsed
	}

	// The timeouts are already time.Duration values parsed from the config
	// (e.g. "5s"); they must not be multiplied by time.Second again.
	redisClient = redis.NewClient(&redis.Options{
		Addr:         cfg.GetRedisAddress(),
		Password:     cfg.Redis.Password,
		DB:           db,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
		PoolSize:     cfg.Redis.PoolSize,
		PoolTimeout:  cfg.Redis.PoolTimeout,
	})

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return err
	}

	return nil
}

func GetRedis() *redis.Client {
	return redisClient
}

func CloseRedis() {
	if redisClient != nil {
		_ = redisClient.Close()
	}
}
