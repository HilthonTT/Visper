package logging

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var zapSinLogger *zap.SugaredLogger

type zapLogger struct {
	cfg    *LoggerConfig
	logger *zap.SugaredLogger
}

var zapLogLevelMapping = map[string]zapcore.Level{
	"debug": zapcore.DebugLevel,
	"info":  zapcore.InfoLevel,
	"warn":  zapcore.WarnLevel,
	"error": zapcore.ErrorLevel,
	"fatal": zapcore.FatalLevel,
}

func newZapLogger(cfg *LoggerConfig) *zapLogger {
	logger := &zapLogger{cfg: cfg}
	logger.Init()
	return logger
}

func (l *zapLogger) Init() {
	once.Do(func() {
		now := time.Now()
		fileName := fmt.Sprintf("%s%s-%s.%s", l.cfg.FilePath, now.Format("2006-01-02"), uuid.NewString(), "log")
		w := zapcore.AddSync(&lumberjack.Logger{
			Filename:   fileName,
			MaxSize:    1,
			MaxAge:     20,
			LocalTime:  true,
			MaxBackups: 5,
			Compress:   true,
		})

		config := zap.NewProductionEncoderConfig()
		config.EncodeTime = zapcore.ISO8601TimeEncoder

		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(config),
			w,
			l.getLogLevel(),
		)

		logger := zap.New(core, zap.AddCaller(),
			zap.AddCallerSkip(1),
			zap.AddStacktrace(zapcore.ErrorLevel)).Sugar()

		zapSinLogger = logger.With("AppName", "MyApp", "LoggerName", "Zaplog")
	})

	l.logger = zapSinLogger
}

func (l *zapLogger) getLogLevel() zapcore.Level {
	level, exists := zapLogLevelMapping[l.cfg.Level]
	if !exists {
		return zapcore.DebugLevel
	}
	return level
}

func (l *zapLogger) Debug(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any) {
	params := prepareLogInfo(cat, sub, extra)

	l.logger.Debugw(msg, params...)
}

func (l *zapLogger) Debugf(template string, args ...any) {
	l.logger.Debugf(template, args)
}

func (l *zapLogger) Info(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any) {
	params := prepareLogInfo(cat, sub, extra)
	l.logger.Infow(msg, params...)
}

func (l *zapLogger) Infof(template string, args ...any) {
	l.logger.Infof(template, args)
}

func (l *zapLogger) Warn(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any) {
	params := prepareLogInfo(cat, sub, extra)
	l.logger.Warnw(msg, params...)
}

func (l *zapLogger) Warnf(template string, args ...any) {
	l.logger.Warnf(template, args)
}

func (l *zapLogger) Error(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any) {
	params := prepareLogInfo(cat, sub, extra)
	l.logger.Errorw(msg, params...)
}

func (l *zapLogger) Errorf(template string, args ...any) {
	l.logger.Errorf(template, args)
}

func (l *zapLogger) Fatal(cat Category, sub SubCategory, msg string, extra map[ExtraKey]any) {
	params := prepareLogInfo(cat, sub, extra)
	l.logger.Fatalw(msg, params...)
}

func (l *zapLogger) Fatalf(template string, args ...any) {
	l.logger.Fatalf(template, args)
}

func prepareLogInfo(cat Category, sub SubCategory, extra map[ExtraKey]any) []any {
	if extra == nil {
		extra = make(map[ExtraKey]any)
	}
	extra["Category"] = cat
	extra["SubCategory"] = sub

	return logParamsToZapParams(extra)
}
