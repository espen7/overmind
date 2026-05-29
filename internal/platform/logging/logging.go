package logging

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

// Init 初始化全局日志入口。
// 当前仓库统一从这里拿 logger，避免 platform 与 kit 再各自维护一套封装。
func Init(serviceName string, level string, encoding string) {
	cfg := zap.NewDevelopmentConfig()
	if encoding == "json" {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.DateTime)
	}

	if parsed, err := zapcore.ParseLevel(level); err == nil {
		cfg.Level = zap.NewAtomicLevelAt(parsed)
	}

	cfg.InitialFields = map[string]interface{}{
		"service": serviceName,
	}

	built, err := cfg.Build(zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	if err != nil {
		panic(err)
	}

	logger = built
	zap.ReplaceGlobals(logger)
}

func L() *zap.Logger {
	if logger == nil {
		Init("default", "debug", "console")
	}
	return logger
}

func With(fields ...zap.Field) *zap.Logger {
	return L().With(fields...)
}

func WithTraceID(traceID string) *zap.Logger {
	return L().With(String("trace_id", traceID))
}

func String(key string, value string) zap.Field {
	return zap.String(key, value)
}

func Int64(key string, value int64) zap.Field {
	return zap.Int64(key, value)
}

func Bool(key string, value bool) zap.Field {
	return zap.Bool(key, value)
}

func Error(err error) zap.Field {
	return zap.Error(err)
}
