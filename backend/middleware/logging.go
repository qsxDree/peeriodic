package middleware

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/logoes0/peeriodic.git/logger"
)

var (
	TotalRequests uint64
	TotalErrors   uint64
)

// GetRequestMetrics returns total requests and error counts
func GetRequestMetrics() (uint64, uint64) {
	return atomic.LoadUint64(&TotalRequests), atomic.LoadUint64(&TotalErrors)
}

// ResponseWriterWrapper wraps http.ResponseWriter to capture status code and bytes written
type ResponseWriterWrapper struct {
	http.ResponseWriter
	StatusCode   int
	BytesWritten int64
}

// WriteHeader captures the status code
func (rw *ResponseWriterWrapper) WriteHeader(code int) {
	if rw.StatusCode != 0 {
		return
	}
	rw.StatusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures bytes written
func (rw *ResponseWriterWrapper) Write(b []byte) (int, error) {
	if rw.StatusCode == 0 {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.BytesWritten += int64(n)
	return n, err
}

// RequestID adds or propagates an X-Request-ID header and stores it in context
func RequestID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" || len(reqID) > 128 {
			reqID = uuid.New().String()
		}

		w.Header().Set("X-Request-ID", reqID)
		ctx := logger.WithRequestID(r.Context(), reqID)
		next(w, r.WithContext(ctx))
	}
}

// Logging middleware logs HTTP requests with structured attributes
func Logging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		reqID := logger.GetRequestID(r.Context())
		if reqID == "" {
			reqID = r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = uuid.New().String()
			}
			w.Header().Set("X-Request-ID", reqID)
			r = r.WithContext(logger.WithRequestID(r.Context(), reqID))
		}

		rw := &ResponseWriterWrapper{
			ResponseWriter: w,
		}

		next(rw, r)

		duration := time.Since(start)
		status := rw.StatusCode
		if status == 0 {
			status = http.StatusOK
		}

		atomic.AddUint64(&TotalRequests, 1)
		if status >= 400 {
			atomic.AddUint64(&TotalErrors, 1)
		}

		logAttrs := []any{
			slog.String("request_id", reqID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.Float64("duration_sec", duration.Seconds()),
			slog.Int64("bytes", rw.BytesWritten),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		}

		// Also attach user UID if available in context
		if uid := GetUserUID(r.Context()); uid != "" {
			logAttrs = append(logAttrs, slog.String("user_uid", uid))
		}

		// Choose log level based on HTTP status code
		if status >= 500 {
			logger.Get().Error("HTTP Request Failed", logAttrs...)
		} else if status >= 400 {
			logger.Get().Warn("HTTP Client Error", logAttrs...)
		} else {
			logger.Get().Info("HTTP Request Completed", logAttrs...)
		}
	}
}
