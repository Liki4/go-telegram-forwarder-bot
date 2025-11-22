package logger

import (
	"os"
	"time"

	"go-telegram-forwarder-bot/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func New(cfg config.LogConfig) (*zap.Logger, error) {
	var level zapcore.Level
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// JSON encoder config for file output
	jsonEncoderConfig := zap.NewProductionEncoderConfig()
	jsonEncoderConfig.TimeKey = "timestamp"
	jsonEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder

	// Console encoder config for stdout output
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
	consoleEncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.DateTime)
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	var core zapcore.Core
	switch cfg.Output {
	case "file":
		// File output: use JSON encoder
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    100,
			MaxBackups: 3,
			MaxAge:     28,
			Compress:   true,
		}
		core = zapcore.NewCore(
			zapcore.NewJSONEncoder(jsonEncoderConfig),
			zapcore.AddSync(fileWriter),
			level,
		)
	case "both":
		// Both output: use Console encoder for stdout, JSON encoder for file
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    100,
			MaxBackups: 3,
			MaxAge:     28,
			Compress:   true,
		}
		stdoutCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoderConfig),
			zapcore.AddSync(os.Stdout),
			level,
		)
		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(jsonEncoderConfig),
			zapcore.AddSync(fileWriter),
			level,
		)
		// Use Tee to write to both cores
		core = zapcore.NewTee(stdoutCore, fileCore)
	default: // "stdout" or any other value defaults to stdout
		// Stdout output: use Console encoder
		core = zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoderConfig),
			zapcore.AddSync(os.Stdout),
			level,
		)
	}

	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return logger, nil
}
