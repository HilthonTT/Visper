package configs

import (
	"time"

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
	RequestsPerTimeFrame int           `koanf:"requestPerTimeFrame"`
	TimeFrame            time.Duration `koanf:"timeFrame"`
}

type MessageStoreConfig struct {
	Capacity uint `koanf:"capacity"`
}

type RoomStoreConfig struct {
	Capacity uint `koanf:"capacity"`
}

func Load(path string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, err
	}

	// Set defaults
	k.Set("http.host", "0.0.0.0")
	k.Set("http.port", 8080)
	k.Set("http.read_timeout", 10*time.Second)
	k.Set("http.write_timeout", 30*time.Second)
	k.Set("rateLimiter.requestPerTimeFrame", 100)
	k.Set("rateLimiter.timeFrame", time.Second*20)
	k.Set("room_store.capacity", 100)
	k.Set("message_store.capacity", 100)

	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "koanf"}); err != nil {
		return nil, err
	}

	return &cfg, nil
}
