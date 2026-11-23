package logger

import (
	"context"
	"testing"
)

func TestStructuredLogger(t *testing.T) {
	l := Init("text", "DEBUG")
	if l == nil {
		t.Fatal("expected logger to initialize")
	}

	ctx := WithRequestID(context.Background(), "req-test-123")
	reqID := GetRequestID(ctx)
	if reqID != "req-test-123" {
		t.Errorf("expected req-test-123, got %s", reqID)
	}

	ctxLogger := WithContext(ctx)
	if ctxLogger == nil {
		t.Fatal("expected contextual logger to not be nil")
	}

	// Test package-level helpers
	Debug("test debug message", "key", "val")
	Info("test info message", "count", 42)
	Warn("test warn message")
	Error("test error message", "code", 500)
}
