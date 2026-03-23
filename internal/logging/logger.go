package logging

import (
	"log/slog"
	"os"
	"strings"
)

const defaultLogLevel = slog.LevelWarn
const LOG_LEVEL_ENV = "LOG_LEVEL"

type Logger interface {
	Info(msg string, keyvals ...any)
	Warn(msg string, keyvals ...any)
	Error(msg string, keyvals ...any)
	Debug(msg string, keyvals ...any)
}

type logger struct {
	logger *slog.Logger
}

func New(level slog.Level) Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewTextHandler(os.Stdout, opts)

	return &logger{
		logger: slog.New(handler),
	}
}

func (l *logger) Info(msg string, keyvals ...any) {
	l.logger.Info(msg, keyvals...)
}

func (l *logger) Warn(msg string, keyvals ...any) {
	l.logger.Warn(msg, keyvals...)
}

func (l *logger) Error(msg string, keyvals ...any) {
	l.logger.Error(msg, keyvals...)
}

func (l *logger) Debug(msg string, keyvals ...any) {
	l.logger.Debug(msg, keyvals...)
}

func getLogLevel() slog.Level {
	levelStr := os.Getenv(LOG_LEVEL_ENV)
	if levelStr == "" {
		return defaultLogLevel
	}

	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return defaultLogLevel
	}
}
