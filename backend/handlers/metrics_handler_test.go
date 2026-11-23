package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsHandler(t *testing.T) {
	handler := NewMetricsHandler(nil)

	t.Run("HandleMetrics - Returns 200 OK with runtime stats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		handler.HandleMetrics(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var resp MetricsResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.ActiveGoroutines <= 0 {
			t.Errorf("expected positive active goroutines, got %d", resp.ActiveGoroutines)
		}
		if resp.UptimeSeconds < 0 {
			t.Errorf("expected non-negative uptime, got %f", resp.UptimeSeconds)
		}
	})

	t.Run("HandleMetrics - Method Not Allowed for POST", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
		rec := httptest.NewRecorder()
		handler.HandleMetrics(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 Method Not Allowed, got %d", rec.Code)
		}
	})
}
