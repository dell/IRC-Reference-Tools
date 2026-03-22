package logging

import (
	"fmt"
	"strings"
)

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
