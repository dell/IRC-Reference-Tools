package logging

import (
	"fmt"
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
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if isSensitiveKey(a.Key) {
				a.Value = slog.StringValue("[REDACTED]")
				return a
			}
			if a.Value.Kind() == slog.KindAny {
				a.Value = slog.AnyValue(sanitizeAny(a.Value.Any()))
				return a
			}
			return a
		},
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

func isSensitiveKey(k string) bool {
	key := strings.ToLower(strings.TrimSpace(k))
	switch key {
	case "password", "passwd", "pass", "secret", "token", "authorization", "auth", "apikey", "api_key":
		return true
	default:
		return strings.Contains(key, "password") ||
			strings.Contains(key, "secret") ||
			strings.Contains(key, "token") ||
			strings.Contains(key, "authorization") ||
			strings.Contains(key, "api_key") ||
			strings.Contains(key, "apikey")
	}
}

func sanitizeAny(v any) any {
	if v == nil {
		return nil
	}

	switch t := v.(type) {
	case map[string]string:
		return sanitizeStringMap(t)
	case map[string]any:
		return sanitizeAnyMap(t)
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			out = append(out, sanitizeAny(e))
		}
		return out
	default:
		// For types that implement Stringer (e.g. auth.Service already
		// redacts sensitive fields in its String() method). This avoids
		// expensive reflection on every log call.
		if s, ok := v.(fmt.Stringer); ok {
			return s.String()
		}
		return v
	}
}

func sanitizeStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if isSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = v
	}
	return out
}

func sanitizeAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if isSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = sanitizeAny(v)
	}
	return out
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
