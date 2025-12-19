package configs

import (
	"fmt"
	"time"

	"github.com/hilthontt/visper/internal/infrastructure/env"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	HTTP         HTTPConfig         `koanf:"http"`
	RateLimiter  RateLimiterConfig  `koanf:"rateLimiter"`
	MessageStore MessageStoreConfig `koanf:"message_store"`
	RoomStore    RoomStoreConfig    `koanf:"room_store"`
}

type HTTPConfig struct {
	Host           string        `koanf:"host"`
	Port           uint16        `koanf:"port"`
	AllowedOrigins []string      `koanf:"allowed_origins"`
	AllowedHeaders []string      `koanf:"allowed_headers"`
	ReadTimeout    time.Duration `koanf:"read_timeout"`
	WriteTimeout   time.Duration `koanf:"write_timeout"`
}

type RateLimiterConfig struct {
	MaxRatePerSecond int           `koanf:"maxRatePerSecond"`
	MaxBurst         int           `koanf:"maxBurst"`
	CacheTTL         time.Duration `koanf:"cacheTTL"`
	SourceHeaderKey  string        `koanf:"sourceHeaderKey"`
}

type MessageStoreConfig struct {
	Capacity uint `koanf:"capacity"`
}

type RoomStoreConfig struct {
	Capacity uint `koanf:"capacity"`
}

func Load(path string) (*Config, error) {
	k := koanf.New(".")

	// Load from YAML file if it exists
	if path != "" {
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			// Only return error if file was explicitly provided but failed to load
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Apply defaults and environment variable overrides
	applyDefaults(k)
	applyEnvOverrides(k)

	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "koanf"}); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(k *koanf.Koanf) {
	// HTTP defaults
	setDefault(k, "http.host", "0.0.0.0")
	setDefault(k, "http.port", 8080)
	setDefault(k, "http.read_timeout", 10*time.Second)
	setDefault(k, "http.write_timeout", 30*time.Second)
	setDefault(k, "http.allowed_origins", []string{"*"})
	setDefault(k, "http.allowed_headers", []string{"Content-Type", "Authorization"})

	// Rate limiter defaults
	setDefault(k, "rateLimiter.maxRatePerSecond", 10)
	setDefault(k, "rateLimiter.maxBurst", 20)
	setDefault(k, "rateLimiter.cacheTTL", 5*time.Minute)
	setDefault(k, "rateLimiter.sourceHeaderKey", "X-Forwarded-For")

	// Store defaults
	setDefault(k, "room_store.capacity", 100)
	setDefault(k, "message_store.capacity", 100)
}

func applyEnvOverrides(k *koanf.Koanf) {
	// HTTP config from env
	if host := env.GetString("HTTP_HOST", ""); host != "" {
		k.Set("http.host", host)
	}
	if port := env.GetInt("HTTP_PORT", 0); port > 0 {
		k.Set("http.port", port)
	}
	if readTimeout := env.GetInt("HTTP_READ_TIMEOUT_SECONDS", 0); readTimeout > 0 {
		k.Set("http.read_timeout", time.Duration(readTimeout)*time.Second)
	}
	if writeTimeout := env.GetInt("HTTP_WRITE_TIMEOUT_SECONDS", 0); writeTimeout > 0 {
		k.Set("http.write_timeout", time.Duration(writeTimeout)*time.Second)
	}

	// Rate limiter config from env
	if maxRate := env.GetInt("RATE_LIMIT_MAX_RATE_PER_SECOND", 0); maxRate > 0 {
		k.Set("rateLimiter.maxRatePerSecond", maxRate)
	}
	if maxBurst := env.GetInt("RATE_LIMIT_MAX_BURST", 0); maxBurst > 0 {
		k.Set("rateLimiter.maxBurst", maxBurst)
	}
	if cacheTTL := env.GetInt("RATE_LIMIT_CACHE_TTL_MINUTES", 0); cacheTTL > 0 {
		k.Set("rateLimiter.cacheTTL", time.Duration(cacheTTL)*time.Minute)
	}
	if sourceKey := env.GetString("RATE_LIMIT_SOURCE_HEADER_KEY", ""); sourceKey != "" {
		k.Set("rateLimiter.sourceHeaderKey", sourceKey)
	}

	// Store config from env
	if roomCapacity := env.GetInt("ROOM_STORE_CAPACITY", 0); roomCapacity > 0 {
		k.Set("room_store.capacity", uint(roomCapacity))
	}
	if messageCapacity := env.GetInt("MESSAGE_STORE_CAPACITY", 0); messageCapacity > 0 {
		k.Set("message_store.capacity", uint(messageCapacity))
	}
}

// setDefault only sets the value if the key doesn't already exist
func setDefault(k *koanf.Koanf, key string, value interface{}) {
	if !k.Exists(key) {
		k.Set(key, value)
	}
}
