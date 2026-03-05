package logging

import (
	"log/slog"
	"os"
	"reflect"
	"strings"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
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
	case auth.Service:
		return sanitizeAuthService(t)
	case *auth.Service:
		if t == nil {
			return (*auth.Service)(nil)
		}
		out := sanitizeAuthService(*t)
		return &out
	case auth.ServiceItem:
		return sanitizeAuthServiceItem(t)
	case *auth.ServiceItem:
		if t == nil {
			return (*auth.ServiceItem)(nil)
		}
		out := sanitizeAuthServiceItem(*t)
		return &out
	case auth.Login:
		out := t
		out.Password = "[REDACTED]"
		return out
	case *auth.Login:
		if t == nil {
			return (*auth.Login)(nil)
		}
		out := *t
		out.Password = "[REDACTED]"
		return &out
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
		rv := reflect.ValueOf(v)
		return sanitizeReflect(rv)
	}
}

func sanitizeReflect(rv reflect.Value) any {
	if !rv.IsValid() {
		return nil
	}

	// Unwrap interface
	if rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		return sanitizeAny(rv.Elem().Interface())
	}

	// Handle pointers
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		s := sanitizeReflect(rv.Elem())
		// Preserve pointer shape where possible
		if s == nil {
			return nil
		}
		return s
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		l := rv.Len()
		out := make([]any, 0, l)
		for i := 0; i < l; i++ {
			out = append(out, sanitizeAny(rv.Index(i).Interface()))
		}
		return out
	case reflect.Map:
		// Best effort for maps (commonly map[string]any / map[string]string)
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			k := iter.Key()
			v := iter.Value()
			ks, ok := k.Interface().(string)
			if !ok {
				continue
			}
			if isSensitiveKey(ks) {
				out[ks] = "[REDACTED]"
				continue
			}
			out[ks] = sanitizeAny(v.Interface())
		}
		return out
	case reflect.Struct:
		// Copy the struct and redact known-sensitive fields.
		// If we can't safely set fields, fall back to original value.
		out := reflect.New(rv.Type()).Elem()
		out.Set(rv)
		for i := 0; i < out.NumField(); i++ {
			ft := out.Type().Field(i)
			// Skip unexported
			if ft.PkgPath != "" {
				continue
			}
			key := ft.Name
			if tag := ft.Tag.Get("json"); tag != "" {
				name := strings.Split(tag, ",")[0]
				if name != "" && name != "-" {
					key = name
				}
			}
			f := out.Field(i)
			if !f.CanSet() {
				continue
			}
			if isSensitiveKey(key) {
				if f.Kind() == reflect.String {
					f.SetString("[REDACTED]")
				}
				continue
			}
			// Sanitize nested composites
			switch f.Kind() {
			case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array, reflect.Ptr, reflect.Interface:
				s := sanitizeReflect(f)
				if s == nil {
					continue
				}
				sv := reflect.ValueOf(s)
				if sv.IsValid() && sv.Type().AssignableTo(f.Type()) {
					f.Set(sv)
				}
			}
		}
		return out.Interface()
	default:
		return rv.Interface()
	}
}

func sanitizeAuthService(s auth.Service) auth.Service {
	out := s
	out.Auth = sanitizeStringMap(s.Auth)
	return out
}

func sanitizeAuthServiceItem(si auth.ServiceItem) auth.ServiceItem {
	out := si
	out.Service = sanitizeAuthService(si.Service)
	out.Auth = sanitizeStringMap(si.Auth)
	return out
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
