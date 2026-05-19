package logger

import (
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

func colorTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(fmt.Sprintf("%s%s%s", colorGray, t.Format("15:04:05.000"), colorReset))
}

func colorCallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(fmt.Sprintf("%s%s%s", colorCyan, caller.TrimmedPath(), colorReset))
}

func Init(level, format, output string) error {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	var encoder zapcore.Encoder
	if format == "json" {
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "ts",
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

	if format == "json" {
		globalLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	} else {
		globalLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	}

	return nil
}

func Get() *zap.Logger {
	if globalLogger == nil {
		globalLogger = zap.NewNop()
	}
	return globalLogger
}

func With(fields ...zap.Field) *zap.Logger {
	return Get().With(fields...)
}

func Debug(msg string, fields ...zap.Field) {
	Get().Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	Get().Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	Get().Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	Get().Error(msg, fields...)
}

func ErrorWithStack(msg string, fields ...zap.Field) {
	fields = append(fields, zap.Stack("stacktrace"))
	Get().Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	Get().Fatal(msg, fields...)
}

func Sync() error {
	if globalLogger != nil {
		return globalLogger.Sync()
	}
	return nil
}
