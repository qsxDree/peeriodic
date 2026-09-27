package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/services"
)

func TestAuthMiddleware(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:        "test-jwt-secret-xyz",
			TokenExpiryHours: 24,
		},
	}
	authService := services.NewAuthService(cfg)

	// Generate a valid token
	token, err := authService.GenerateToken("user-123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	handlerCalled := false
	var capturedUID, capturedEmail, capturedName string

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		capturedUID = GetUserUID(r.Context())
		capturedEmail = GetUserEmail(r.Context())
		capturedName = GetUserName(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// 1. Test RequireAuth with valid Bearer token header
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	RequireAuth(authService)(testHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if !handlerCalled {
		t.Error("handler was not called")
	}
	if capturedUID != "user-123" || capturedEmail != "alice@example.com" || capturedName != "Alice" {
		t.Errorf("captured claims mismatch: %s, %s, %s", capturedUID, capturedEmail, capturedName)
	}

	// 2. Test RequireAuth with token query param
	handlerCalled = false
	req = httptest.NewRequest("GET", "/api/protected?token="+token, nil)
	rec = httptest.NewRecorder()

	RequireAuth(authService)(testHandler).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK via query param, got %d", rec.Code)
	}

	// 3. Test RequireAuth with no token -> 401 Unauthorized
	req = httptest.NewRequest("GET", "/api/protected", nil)
	rec = httptest.NewRecorder()

	RequireAuth(authService)(testHandler).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized without token, got %d", rec.Code)
	}

	// 4. Test OptionalAuth with no token -> succeeds as anonymous
	handlerCalled = false
	req = httptest.NewRequest("GET", "/api/optional", nil)
	rec = httptest.NewRecorder()

	OptionalAuth(authService)(testHandler).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for optional auth without token, got %d", rec.Code)
	}
	if capturedUID != "" {
		t.Errorf("expected empty UID for anonymous user, got %s", capturedUID)
	}
}
