package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	Cors     CorsConfig
	Logger   LoggerConfig
	Jaeger   JaegerConfig
	Sentry   SentryConfig
}

type ServerConfig struct {
	InternalPort string
	ExternalPort string
	RunMode      string
	Domain       string
	FrontEndURL  string
}

type LoggerConfig struct {
	FilePath string
	Encoding string
	Level    string
	Logger   string
}

type PostgresConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	DbName          string
	SSLMode         string
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Host               string
	Port               string
	Password           string
	Db                 string
	DialTimeout        time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleCheckFrequency time.Duration
	PoolSize           int
	PoolTimeout        time.Duration
}

type CorsConfig struct {
	AllowOrigins string
}

type JaegerConfig struct {
	ServiceName    string
	ServiceVersion string
	Endpoint       string
}

type SentryConfig struct {
	Dsn            string
	Debug          bool
	SendDefaultPII bool
}

func GetConfig() *Config {
	cfgPath := getConfigPath(os.Getenv("APP_ENV"))
	v, err := LoadConfig(cfgPath, "yml")
	if err != nil {
		log.Fatalf("Error in load config %v", err)
	}

	cfg, err := ParseConfig(v)
	if err != nil {
		log.Fatalf("Error in parse config %v", err)
	}

	if envPort := os.Getenv("PORT"); envPort != "" {
		cfg.Server.ExternalPort = envPort
		log.Printf("Set external port from environment -> %s", cfg.Server.ExternalPort)
	} else {
		log.Printf("Using external port from config -> %s", cfg.Server.ExternalPort)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	return cfg
}

func ParseConfig(v *viper.Viper) (*Config, error) {
	var cfg Config
	err := v.Unmarshal(&cfg)
	if err != nil {
		log.Printf("Unable to parse config: %v", err)
		return nil, err
	}
	return &cfg, nil
}

func LoadConfig(filename string, fileType string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType(fileType)
	v.SetConfigName(filename)

	v.AddConfigPath(".")                        // Current directory
	v.AddConfigPath("./config")                 // ./config
	v.AddConfigPath("./infrastructure/config")  // ./infrastructure/config
	v.AddConfigPath("../config")                // ../config
	v.AddConfigPath("../infrastructure/config") // ../infrastructure/config (from cmd)
	v.AddConfigPath("../../config")             // ../../config

	if wd, err := os.Getwd(); err == nil {
		v.AddConfigPath(filepath.Join(wd, "config"))
		v.AddConfigPath(filepath.Join(wd, "infrastructure", "config"))
	}

	v.AutomaticEnv()

	err := v.ReadInConfig()
	if err != nil {
		log.Printf("Unable to read config: %v", err)
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			return nil, errors.New("config file not found")
		}
		return nil, err
	}

	log.Printf("Using config file: %s", v.ConfigFileUsed())
	return v, nil
}

func getConfigPath(env string) string {
	switch env {
	case "docker":
		return "config-docker"
	case "production":
		return "config-production"
	default:
		return "config-development"
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Server.InternalPort == "" {
		return errors.New("server.internalPort is required")
	}
	if c.Server.ExternalPort == "" {
		return errors.New("server.externalPort is required")
	}
	if c.Server.Domain == "" {
		return errors.New("server.domain is required")
	}

	if c.Postgres.Host == "" {
		return errors.New("postgres.host is required")
	}
	if c.Postgres.Port == "" {
		return errors.New("postgres.port is required")
	}
	if c.Postgres.DbName == "" {
		return errors.New("postgres.dbName is required")
	}

	if c.Redis.Host == "" {
		return errors.New("redis.host is required")
	}
	if c.Redis.Port == "" {
		return errors.New("redis.port is required")
	}

	return nil
}

func (c *Config) IsDevelopment() bool {
	return c.Server.RunMode == "debug" || c.Server.RunMode == "development"
}

func (c *Config) IsProduction() bool {
	return c.Server.RunMode == "release" || c.Server.RunMode == "production"
}

func (c *Config) GetPostgresConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Postgres.Host,
		c.Postgres.Port,
		c.Postgres.User,
		c.Postgres.Password,
		c.Postgres.DbName,
		c.Postgres.SSLMode,
	)
}

func (c *Config) GetRedisAddress() string {
	return fmt.Sprintf("%s:%s", c.Redis.Host, c.Redis.Port)
}

func (c *Config) GetServerAddress() string {
	return fmt.Sprintf(":%s", c.Server.InternalPort)
}

func (c *Config) GetFrontEndURL() string {
	return c.Server.FrontEndURL
}
