package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseStatusIsRecordedOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := &ResponseWriterWrapper{ResponseWriter: rec}
	wrapped.WriteHeader(http.StatusCreated)
	wrapped.WriteHeader(http.StatusInternalServerError)
	_, _ = wrapped.Write([]byte("created"))
	if rec.Code != http.StatusCreated || wrapped.StatusCode != http.StatusCreated || wrapped.BytesWritten != 7 {
		t.Fatalf("incorrect response metrics: %+v", wrapped)
	}
}

func TestRecoveredPanicIsCountedAsFailure(t *testing.T) {
	beforeRequests, beforeErrors := GetRequestMetrics()
	handler := RequestID(Logging(Recoverer(func(http.ResponseWriter, *http.Request) { panic("test") })))
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest("GET", "/panic", nil))
	requests, failures := GetRequestMetrics()
	if rec.Code != 500 || requests != beforeRequests+1 || failures != beforeErrors+1 || rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("panic not recorded: status=%d requests=%d errors=%d", rec.Code, requests, failures)
	}
}
