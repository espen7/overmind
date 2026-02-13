package log

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger 是对 zap.Logger 的封装，提供统一的日志接口。
// 它支持结构化日志记录，并根据环境（开发/生产）自动调整输出格式。
type Logger struct {
	*zap.Logger
}

var (
	// 全局 Logger 实例
	globalLogger *Logger
)

// Init 初始化全局 Logger。
// serviceName: 服务名称，用于标示日志来源。
// isProduction: 是否为生产环境。如果是，则输出 JSON 格式；否则输出彩色 Console 格式。
func Init(serviceName string, isProduction bool) {
	var config zap.Config

	if isProduction {
		// 生产环境配置: JSON 格式, Info 级别, 输出到 Stdout
		config = zap.NewProductionConfig()
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		// 开发环境配置: Console 格式, Debug 级别, 彩色输出
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.DateTime)
	}

	// 添加公共字段
	config.InitialFields = map[string]interface{}{
		"service": serviceName,
	}

	// 构建 Logger
	zapLogger, err := config.Build(zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	if err != nil {
		panic(err)
	}

	globalLogger = &Logger{Logger: zapLogger}

	// 替换全局标准库 Logger，以便第三方库的日志也能被捕获 (可选)
	zap.ReplaceGlobals(zapLogger)
}

// Get 获取全局 Logger 实例
func Get() *Logger {
	if globalLogger == nil {
		// 如果未初始化，提供一个默认的开发 Logger，避免空指针
		Init("default", false)
	}
	return globalLogger
}

// WithTraceID 创建一个带有 TraceID 的子 Logger context
func (l *Logger) WithTraceID(traceID string) *Logger {
	return &Logger{Logger: l.Logger.With(zap.String("trace_id", traceID))}
}

// Info 包装 Zap 的 Info 方法
func Info(msg string, fields ...zap.Field) {
	Get().Info(msg, fields...)
}

// Error 包装 Zap 的 Error 方法
func Error(msg string, fields ...zap.Field) {
	Get().Error(msg, fields...)
}

// Debug 包装 Zap 的 Debug 方法
func Debug(msg string, fields ...zap.Field) {
	Get().Debug(msg, fields...)
}
