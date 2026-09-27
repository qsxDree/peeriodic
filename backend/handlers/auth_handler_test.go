package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/middleware"
	"github.com/logoes0/peeriodic.git/models"
	"github.com/logoes0/peeriodic.git/services"
)

func TestAuthHandler_Validation(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:        "test-secret",
			TokenExpiryHours: 24,
		},
	}
	authService := services.NewAuthService(cfg)
	// dbService is nil for pure validation tests
	handler := NewAuthHandler(nil, authService)

	t.Run("HandleRegister - Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/register", nil)
		rec := httptest.NewRecorder()
		handler.HandleRegister(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 Method Not Allowed, got %d", rec.Code)
		}
	})

	t.Run("HandleRegister - Invalid Email", func(t *testing.T) {
		body, _ := json.Marshal(models.RegisterRequest{
			Email:    "notanemail",
			Password: "password123",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handler.HandleRegister(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("HandleRegister - Password Too Short", func(t *testing.T) {
		body, _ := json.Marshal(models.RegisterRequest{
			Email:    "valid@example.com",
			Password: "123",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handler.HandleRegister(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("HandleLogin - Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
		rec := httptest.NewRecorder()
		handler.HandleLogin(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 Method Not Allowed, got %d", rec.Code)
		}
	})

	t.Run("HandleLogin - Missing Credentials", func(t *testing.T) {
		body, _ := json.Marshal(models.LoginRequest{
			Email:    "",
			Password: "",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handler.HandleLogin(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("HandleMe - Unauthenticated returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		rec := httptest.NewRecorder()
		handler.HandleMe(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("HandleMe - Authenticated returns user claims", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		ctx := middleware.WithUserClaims(req.Context(), "user-abc", "user@test.com", "Tester")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.HandleMe(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
	})
}
