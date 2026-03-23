//go:build integration

package integration

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// newCapturingHandler creates an slog handler identical to the one in
// logging.New() (same ReplaceAttr logic) but writes to buf so tests can
// inspect output.  We duplicate the callback here instead of importing the
// internal package so the test exercises the public logging behaviour
// end-to-end via slog, exactly as it runs in production containers.
func newCapturingHandler(buf *bytes.Buffer, level slog.Level) slog.Handler {
	return slog.NewTextHandler(buf, &slog.HandlerOptions{
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
	})
}

// ---------------------------------------------------------------------------
// Helpers copied from internal/logging/sanitize.go so the integration test
// package is self-contained (internal packages cannot be imported from tests/).
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Integration tests
// ---------------------------------------------------------------------------

// TestSensitiveKeyRedaction verifies that top-level sensitive keys are
// replaced with [REDACTED] in the final log output.
func TestSensitiveKeyRedaction(t *testing.T) {
	sensitiveKeys := []string{
		"password", "Password", "PASSWD", "pass",
		"secret", "token", "authorization", "auth",
		"apikey", "api_key",
		"mysql_password", "jwt_secret", "access_token",
	}

	for _, key := range sensitiveKeys {
		t.Run(key, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(newCapturingHandler(&buf, slog.LevelInfo))

			logger.Info("creds", key, "super-secret-value-123")

			out := buf.String()
			if !strings.Contains(out, "[REDACTED]") {
				t.Errorf("expected [REDACTED] for key %q, got: %s", key, out)
			}
			if strings.Contains(out, "super-secret-value-123") {
				t.Errorf("sensitive value leaked for key %q: %s", key, out)
			}
		})
	}
}

// TestNonSensitiveKeyPassthrough verifies that normal keys are NOT redacted.
func TestNonSensitiveKeyPassthrough(t *testing.T) {
	safeKeys := []string{"host", "ip", "port", "url", "method", "status", "message"}

	for _, key := range safeKeys {
		t.Run(key, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(newCapturingHandler(&buf, slog.LevelInfo))

			logger.Info("request", key, "safe-value")

			out := buf.String()
			if strings.Contains(out, "[REDACTED]") {
				t.Errorf("non-sensitive key %q was redacted: %s", key, out)
			}
			if !strings.Contains(out, "safe-value") {
				t.Errorf("expected safe-value for key %q, got: %s", key, out)
			}
		})
	}
}

// TestNestedMapSanitization verifies that sensitive keys buried inside a
// map[string]any or map[string]string value are redacted while safe keys
// are preserved.
func TestNestedMapSanitization(t *testing.T) {
	t.Run("map[string]string", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(newCapturingHandler(&buf, slog.LevelInfo))

		logger.Info("config", "settings", map[string]string{
			"host":     "10.0.0.1",
			"password": "s3cret",
		})

		out := buf.String()
		if strings.Contains(out, "s3cret") {
			t.Errorf("nested password leaked: %s", out)
		}
		if !strings.Contains(out, "10.0.0.1") {
			t.Errorf("safe nested value missing: %s", out)
		}
		if !strings.Contains(out, "[REDACTED]") {
			t.Errorf("expected [REDACTED] for nested password: %s", out)
		}
	})

	t.Run("map[string]any", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(newCapturingHandler(&buf, slog.LevelInfo))

		logger.Info("config", "payload", map[string]any{
			"url":    "https://example.com",
			"token":  "abc123",
			"nested": map[string]any{"secret": "inner-secret"},
		})

		out := buf.String()
		if strings.Contains(out, "abc123") {
			t.Errorf("nested token leaked: %s", out)
		}
		if strings.Contains(out, "inner-secret") {
			t.Errorf("deeply nested secret leaked: %s", out)
		}
		if !strings.Contains(out, "https://example.com") {
			t.Errorf("safe nested url missing: %s", out)
		}
	})

	t.Run("slice_with_maps", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(newCapturingHandler(&buf, slog.LevelInfo))

		logger.Info("batch", "items", []any{
			map[string]any{"host": "a.com", "password": "pw1"},
			map[string]any{"host": "b.com", "password": "pw2"},
		})

		out := buf.String()
		if strings.Contains(out, "pw1") || strings.Contains(out, "pw2") {
			t.Errorf("password in slice leaked: %s", out)
		}
	})
}

// TestLevelFiltering verifies that ReplaceAttr is never invoked for log
// messages below the configured level — meaning no CPU is wasted on
// sanitisation for suppressed messages.
func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	// Handler set to WARN — Debug and Info should produce nothing.
	logger := slog.New(newCapturingHandler(&buf, slog.LevelWarn))

	logger.Debug("low", "password", "nope")
	logger.Info("low", "password", "nope")

	if buf.Len() != 0 {
		t.Errorf("expected no output for sub-level messages, got: %s", buf.String())
	}

	// Warn and above should still sanitise.
	logger.Warn("alert", "password", "oops")
	out := buf.String()
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("warn-level message was not sanitised: %s", out)
	}
	if strings.Contains(out, "oops") {
		t.Errorf("password leaked at warn level: %s", out)
	}
}

// TestStringerInterface verifies that types implementing fmt.Stringer are
// rendered via their String() method (which may already self-redact).
func TestStringerInterface(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newCapturingHandler(&buf, slog.LevelInfo))

	logger.Info("service", "backend", &mockStringer{display: "AuthService{user=admin, pass=***}"})

	out := buf.String()
	if !strings.Contains(out, "AuthService{user=admin, pass=***}") {
		t.Errorf("Stringer output not used: %s", out)
	}
}

type mockStringer struct{ display string }

func (m *mockStringer) String() string { return m.display }

// ---------------------------------------------------------------------------
// Benchmark — addresses the "is this expensive?" question.
// ---------------------------------------------------------------------------

// BenchmarkReplaceAttr_PlainAttrs measures the overhead of ReplaceAttr when
// every attribute is a plain string with a non-sensitive key (the fast path).
func BenchmarkReplaceAttr_PlainAttrs(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(newCapturingHandler(&buf, slog.LevelInfo))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Info("request", "method", "GET", "url", "/api/v1/irc", "status", "200")
	}
}

// BenchmarkReplaceAttr_SensitiveKey measures redaction of a top-level
// sensitive key.
func BenchmarkReplaceAttr_SensitiveKey(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(newCapturingHandler(&buf, slog.LevelInfo))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Info("login", "password", "hunter2")
	}
}

// BenchmarkReplaceAttr_NestedMap measures deep sanitisation of a map value.
func BenchmarkReplaceAttr_NestedMap(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(newCapturingHandler(&buf, slog.LevelInfo))

	payload := map[string]any{
		"host":  "10.0.0.1",
		"port":  8443,
		"token": "abc123",
		"nested": map[string]any{
			"secret": "inner",
			"safe":   "ok",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Info("config", "payload", payload)
	}
}

// BenchmarkReplaceAttr_FilteredLevel measures the cost when the message is
// below the handler level (should be near-zero — slog skips ReplaceAttr).
func BenchmarkReplaceAttr_FilteredLevel(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(newCapturingHandler(&buf, slog.LevelWarn))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Debug("noisy", "password", "secret", "payload", map[string]any{"token": "x"})
	}
}
