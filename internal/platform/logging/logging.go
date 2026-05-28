package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

func Init(level string, encoding string) {
	cfg := zap.NewDevelopmentConfig()
	if encoding == "json" {
		cfg = zap.NewProductionConfig()
	}

	if parsed, err := zapcore.ParseLevel(level); err == nil {
		cfg.Level = zap.NewAtomicLevelAt(parsed)
	}

	built, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	logger = built
	zap.ReplaceGlobals(logger)
}

func L() *zap.Logger {
	if logger == nil {
		Init("debug", "console")
	}
	return logger
}

func String(key string, value string) zap.Field {
	return zap.String(key, value)
}
