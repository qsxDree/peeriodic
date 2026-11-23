package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
)

// Init initializes the global structured logger.
// format: "json" or "text" (default is "text" for local dev, "json" for production)
// levelStr: "DEBUG", "INFO", "WARN", "ERROR"
func Init(format string, levelStr string) *slog.Logger {
	var level slog.Level
	switch strings.ToUpper(strings.TrimSpace(levelStr)) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if strings.ToLower(strings.TrimSpace(format)) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	l := slog.New(handler)
	slog.SetDefault(l)
	return l
}

// Get returns the default structured logger
func Get() *slog.Logger {
	return slog.Default()
}

// WithContext returns a logger enriched with the request ID.
func WithContext(ctx context.Context) *slog.Logger {
	l := Get()
	if ctx == nil {
		return l
	}

	var attrs []any
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
		attrs = append(attrs, slog.String("request_id", reqID))
	}

	if len(attrs) > 0 {
		return l.With(attrs...)
	}
	return l
}

// Context helpers for attaching Request ID
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
		return reqID
	}
	return ""
}

// Package-level logging convenience wrappers
func Debug(msg string, args ...any) {
	Get().Debug(msg, args...)
}

func Info(msg string, args ...any) {
	Get().Info(msg, args...)
}

func Warn(msg string, args ...any) {
	Get().Warn(msg, args...)
}

func Error(msg string, args ...any) {
	Get().Error(msg, args...)
}
