package logger

import (
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ANSI 颜色代码
const (
	colorReset     = "\033[0m"
	colorRed       = "\033[31m"
	colorGreen     = "\033[32m"
	colorYellow    = "\033[33m"
	colorBlue      = "\033[34m"
	colorMagenta   = "\033[35m"
	colorCyan      = "\033[36m"
	colorGray      = "\033[90m"
	colorBoldRed   = "\033[1;31m"
	colorBoldGreen = "\033[1;32m"
)

var globalLogger *zap.Logger

// colorLevelEncoder 彩色日志级别编码器
func colorLevelEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var coloredLevel string
	switch level {
	case zapcore.DebugLevel:
		coloredLevel = fmt.Sprintf("%s%-5s%s", colorGray, "DEBUG", colorReset)
	case zapcore.InfoLevel:
		coloredLevel = fmt.Sprintf("%s%-5s%s", colorBoldGreen, "INFO", colorReset)
	case zapcore.WarnLevel:
		coloredLevel = fmt.Sprintf("%s%-5s%s", colorYellow, "WARN", colorReset)
	case zapcore.ErrorLevel:
		coloredLevel = fmt.Sprintf("%s%-5s%s", colorBoldRed, "ERROR", colorReset)
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		coloredLevel = fmt.Sprintf("%s%-5s%s", colorRed, level.CapitalString(), colorReset)
	default:
		coloredLevel = level.CapitalString()
	}
	enc.AppendString(coloredLevel)
}

// colorTimeEncoder 彩色时间编码器（仅含时间，适合开发环境）
func colorTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(fmt.Sprintf("%s%s%s", colorGray, t.Format("15:04:05.000"), colorReset))
}

// colorCallerEncoder 彩色调用者编码器
func colorCallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(fmt.Sprintf("%s%s%s", colorCyan, caller.TrimmedPath(), colorReset))
}

// Init 初始化全局日志器。
// level: debug / info / warn / error
// format: console（彩色终端） / json（结构化）
// output: stdout / 文件路径
func Init(level, format, output string) error {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	var encoder zapcore.Encoder
	if format == "json" {
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		// console 模式使用彩色输出，适合开发环境
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    colorLevelEncoder,
			EncodeTime:     colorTimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   colorCallerEncoder,
		}
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	var writeSyncer zapcore.WriteSyncer
	if output == "stdout" {
		writeSyncer = zapcore.AddSync(os.Stdout)
	} else {
		file, err := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		writeSyncer = zapcore.AddSync(file)
	}

	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)

	// console 模式下禁用自动堆栈跟踪，保持开发日志简洁
	// json 模式下保留堆栈跟踪，便于生产环境调试
	if format == "json" {
		globalLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	} else {
		globalLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	}

	return nil
}

// InitFromEnv 从环境变量初始化日志器，并提供合理的默认值。
//
//	LOG_LEVEL  日志级别，默认 "info"
//	LOG_FORMAT 输出格式 "console" 或 "json"，默认 "console"
//	LOG_OUTPUT 输出目标 "stdout" 或文件路径，默认 "stdout"
func InitFromEnv() error {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}

	format := os.Getenv("LOG_FORMAT")
	if format == "" {
		format = "console"
	}

	output := os.Getenv("LOG_OUTPUT")
	if output == "" {
		output = "stdout"
	}

	return Init(level, format, output)
}

// Get 获取全局日志器。未初始化时返回 NopLogger（静默丢弃）。
func Get() *zap.Logger {
	if globalLogger == nil {
		globalLogger = zap.NewNop()
	}
	return globalLogger
}

// With 创建带预设字段的日志器。
// 用于在函数/模块入口创建带上下文（date、session 等）的日志器实例。
func With(fields ...zap.Field) *zap.Logger {
	return Get().With(fields...)
}

// Debug 调试日志
func Debug(msg string, fields ...zap.Field) {
	Get().Debug(msg, fields...)
}

// Info 信息日志
func Info(msg string, fields ...zap.Field) {
	Get().Info(msg, fields...)
}

// Warn 警告日志
func Warn(msg string, fields ...zap.Field) {
	Get().Warn(msg, fields...)
}

// Error 错误日志
func Error(msg string, fields ...zap.Field) {
	Get().Error(msg, fields...)
}

// ErrorWithStack 带堆栈跟踪的错误日志
func ErrorWithStack(msg string, fields ...zap.Field) {
	fields = append(fields, zap.Stack("stacktrace"))
	Get().Error(msg, fields...)
}

// Fatal 致命错误日志（记录后调用 os.Exit(1)）
func Fatal(msg string, fields ...zap.Field) {
	Get().Fatal(msg, fields...)
}

// Sync 刷新缓冲区。在所有日志输出完成后（如服务关闭时）调用。
func Sync() error {
	if globalLogger != nil {
		return globalLogger.Sync()
	}
	return nil
}
