package config

import (
	"errors"
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
}

type ServerConfig struct {
	InternalPort string
	ExternalPort string
	RunMode      string
	Domain       string
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

	envPort := os.Getenv("PORT")
	if envPort != "" {
		cfg.Server.ExternalPort = envPort
		log.Printf("Set external port from environment -> %s", cfg.Server.ExternalPort)
	} else {
		cfg.Server.ExternalPort = cfg.Server.InternalPort
		log.Printf("Environment variable PORT not set; using internal port value -> %s", cfg.Server.ExternalPort)
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

	// Add multiple possible config paths
	v.AddConfigPath(".")                        // Current directory
	v.AddConfigPath("./config")                 // ./config
	v.AddConfigPath("./infrastructure/config")  // ./infrastructure/config
	v.AddConfigPath("../config")                // ../config
	v.AddConfigPath("../infrastructure/config") // ../infrastructure/config (from cmd)
	v.AddConfigPath("../../config")             // ../../config

	// Add absolute path if running from project root
	if wd, err := os.Getwd(); err == nil {
		v.AddConfigPath(filepath.Join(wd, "config"))
		v.AddConfigPath(filepath.Join(wd, "infrastructure", "config"))
	}

	v.AutomaticEnv()

	err := v.ReadInConfig()
	if err != nil {
		log.Printf("Unable to read config: %v", err)
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
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
