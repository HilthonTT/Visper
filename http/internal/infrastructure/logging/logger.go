package logging

import (
	"sync"

	"github.com/hilthontt/visper/cli/pkg/env"
)

var once sync.Once

type Logger interface {
	Init()

	Debug(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any)
	Debugf(template string, args ...any)

	Info(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any)
	Infof(template string, args ...any)

	Warn(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any)
	Warnf(template string, args ...any)

	Error(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any)
	Errorf(template string, args ...any)

	Fatal(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any)
	Fatalf(template string, args ...any)
}

type LoggerConfig struct {
	FilePath string
	Encoding string
	Level    string
	Logger   string
}

func NewDefaultConfig() *LoggerConfig {
	return &LoggerConfig{
		FilePath: env.GetString("LOGGER_FILE_PATH", "./logs/"),
		Encoding: env.GetString("LOGGER_ENCODING", "json"),
		Level:    env.GetString("LOGGER_LEVEL", "debug"),
		Logger:   env.GetString("LOGGER_LOGGER", "zap"),
	}
}

func NewLogger(cfg *LoggerConfig) Logger {
	switch cfg.Logger {
	case "zap":
		return newZapLogger(cfg)
	case "zerolog":
		return newZeroLogger(cfg)
	}

	panic("logger not supported: supported loggers: [zap, zerolog]")
}
