package configs

import (
	"time"

	"github.com/hilthontt/visper/internal/infrastructure/env"
)

type Config struct {
	HTTP         HTTPConfig
	RateLimiter  RateLimiterConfig
	MessageStore MessageStoreConfig
	RoomStore    RoomStoreConfig
	Environment  string
	Swagger      SwaggerConfig
}

type SwaggerConfig struct {
	Host string
}

type HTTPConfig struct {
	Host           string
	Port           uint16
	AllowedOrigins []string
	AllowedHeaders []string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
}

type RateLimiterConfig struct {
	MaxRatePerSecond int
	MaxBurst         int
	CacheTTL         time.Duration
	SourceHeaderKey  string
}

type MessageStoreConfig struct {
	Capacity uint
}

type RoomStoreConfig struct {
	Capacity uint
}

func Load() *Config {
	return &Config{
		HTTP: HTTPConfig{
			Host:           env.GetString("HTTP_HOST", "0.0.0.0"),
			Port:           uint16(env.GetInt("HTTP_PORT", 8080)),
			ReadTimeout:    time.Duration(env.GetInt("HTTP_READ_TIMEOUT_SECONDS", 10)) * time.Second,
			WriteTimeout:   time.Duration(env.GetInt("HTTP_WRITE_TIMEOUT_SECONDS", 30)) * time.Second,
			AllowedOrigins: []string{"*"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
		},
		RateLimiter: RateLimiterConfig{
			MaxRatePerSecond: env.GetInt("RATE_LIMIT_MAX_RATE_PER_SECOND", 10),
			MaxBurst:         env.GetInt("RATE_LIMIT_MAX_BURST", 20),
			CacheTTL:         time.Duration(env.GetInt("RATE_LIMIT_CACHE_TTL_MINUTES", 5)) * time.Minute,
			SourceHeaderKey:  env.GetString("RATE_LIMIT_SOURCE_HEADER_KEY", "X-Forwarded-For"),
		},
		MessageStore: MessageStoreConfig{
			Capacity: uint(env.GetInt("MESSAGE_STORE_CAPACITY", 100)),
		},
		RoomStore: RoomStoreConfig{
			Capacity: uint(env.GetInt("ROOM_STORE_CAPACITY", 100)),
		},
		Environment: env.GetString("ENVIRONMENT", "development"),
		Swagger: SwaggerConfig{
			Host: env.GetString("SWAGGER_HOST", "localhost:8080"),
		},
	}
}
