package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	healthHandler := NewHealthHandler(nil)

	t.Run("HandleHealthz - Returns 200 OK with uptime", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		healthHandler.HandleHealthz(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		var resp HealthResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Status != "ok" {
			t.Errorf("expected status 'ok', got %s", resp.Status)
		}
		if resp.UptimeSeconds < 0 {
			t.Errorf("uptime should be positive, got %f", resp.UptimeSeconds)
		}
	})

	t.Run("HandleHealthz - Method Not Allowed for POST", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
		rec := httptest.NewRecorder()
		healthHandler.HandleHealthz(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 Method Not Allowed, got %d", rec.Code)
		}
	})

	t.Run("HandleReadyz - Uninitialized DB returns 503", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		healthHandler.HandleReadyz(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503 Service Unavailable, got %d", rec.Code)
		}

		var resp ReadyResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != "unavailable" {
			t.Errorf("expected status 'unavailable', got %s", resp.Status)
		}
	})
}
