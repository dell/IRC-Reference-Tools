package logging

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	logger := New(slog.LevelInfo)
	if logger == nil {
		t.Fatal("New() returned nil")
	}

	// Test with different levels
	levels := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	for _, level := range levels {
		logger := New(level)
		if logger == nil {
			t.Errorf("New(%v) returned nil", level)
		}
	}
}

func TestLoggerMethods(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	logger := &logger{
		logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	tests := []struct {
		name     string
		logFunc  func(msg string, keyvals ...any)
		expected string
	}{
		{
			name:     "Info",
			logFunc:  logger.Info,
			expected: "INFO",
		},
		{
			name:     "Warn",
			logFunc:  logger.Warn,
			expected: "WARN",
		},
		{
			name:     "Error",
			logFunc:  logger.Error,
			expected: "ERROR",
		},
		{
			name:     "Debug",
			logFunc:  logger.Debug,
			expected: "DEBUG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc("test message", "key", "value")

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Expected level %s in output, got: %s", tt.expected, output)
			}

			if !strings.Contains(output, "test message") {
				t.Errorf("Expected message 'test message' in output, got: %s", output)
			}

			if !strings.Contains(output, "key=value") {
				t.Errorf("Expected 'key=value' in output, got: %s", output)
			}
		})
	}
}

func TestGetLogLevel(t *testing.T) {
	// Save original env var
	original := os.Getenv("LOG_LEVEL")
	defer os.Setenv("LOG_LEVEL", original)

	tests := []struct {
		envValue string
		expected slog.Level
	}{
		{"", defaultLogLevel},
		{"DEBUG", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"debug", slog.LevelDebug}, // Test case insensitive
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"INVALID", defaultLogLevel}, // Test invalid value
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("LOG_LEVEL", tt.envValue)
			result := getLogLevel()
			if result != tt.expected {
				t.Errorf("getLogLevel() with env %s = %v, expected %v", tt.envValue, result, tt.expected)
			}
		})
	}
}

func TestGlobalLogger(t *testing.T) {
	// Save original env var
	original := os.Getenv("LOG_LEVEL")
	defer os.Setenv("LOG_LEVEL", original)

	// Set to debug to capture all levels
	os.Setenv("LOG_LEVEL", "DEBUG")

	// Capture log output
	var buf bytes.Buffer

	// Create a custom logger that writes to our buffer
	originalLogger := Default
	defer func() { Default = originalLogger }()

	Default = New(slog.LevelDebug)

	// Redirect output by creating a new logger with custom handler
	logger := &logger{
		logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
	Default = logger

	// Test global functions
	Info("info message", "type", "global")
	Warn("warn message", "type", "global")
	Error("error message", "type", "global")
	Debug("debug message", "type", "global")

	// Count log entries
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	expectedCount := 4
	if len(lines) != expectedCount {
		t.Errorf("Expected %d log entries, got %d", expectedCount, len(lines))
	}
}
