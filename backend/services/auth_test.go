package services

import (
	"github.com/golang-jwt/jwt/v5"
	"testing"
	"time"

	"github.com/logoes0/peeriodic.git/config"
)

func TestAuthServicePasswordHashing(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:        "test-secret",
			TokenExpiryHours: 24,
		},
	}
	auth := NewAuthService(cfg)

	password := "my-secure-password-123"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash == password {
		t.Fatal("hash should not equal raw password")
	}

	if !auth.CheckPasswordHash(password, hash) {
		t.Fatal("password verification failed for correct password")
	}

	if auth.CheckPasswordHash("wrong-password", hash) {
		t.Fatal("password verification succeeded for incorrect password")
	}
}

func TestAuthServiceJWTTokenLifecycle(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:        "super-secret-key-123",
			TokenExpiryHours: 1,
		},
	}
	auth := NewAuthService(cfg)

	uid := "user-uuid-1"
	email := "alice@example.com"
	name := "Alice"

	tokenString, err := auth.GenerateToken(uid, email, name)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if tokenString == "" {
		t.Fatal("token string is empty")
	}

	claims, err := auth.ValidateToken(tokenString)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	if claims.UID != uid {
		t.Errorf("expected UID %s, got %s", uid, claims.UID)
	}
	if claims.Email != email {
		t.Errorf("expected Email %s, got %s", email, claims.Email)
	}
	if claims.Name != name {
		t.Errorf("expected Name %s, got %s", name, claims.Name)
	}

	// Validate with wrong secret fails
	wrongCfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:        "different-secret",
			TokenExpiryHours: 1,
		},
	}
	wrongAuth := NewAuthService(wrongCfg)
	_, err = wrongAuth.ValidateToken(tokenString)
	if err == nil {
		t.Fatal("expected validation to fail with mismatched secret")
	}
}

func TestRoleHelpers(t *testing.T) {
	if !CanWrite(RoleOwner) || !CanWrite(RoleEditor) {
		t.Error("owner and editor should have write access")
	}
	if CanWrite(RoleViewer) || CanWrite(RoleNone) {
		t.Error("viewer and none should not have write access")
	}

	if !CanAdmin(RoleOwner) {
		t.Error("owner should have admin access")
	}
	if CanAdmin(RoleEditor) || CanAdmin(RoleViewer) {
		t.Error("editor and viewer should not have admin access")
	}

	if !CanRead(RoleOwner) || !CanRead(RoleEditor) || !CanRead(RoleViewer) {
		t.Error("owner, editor, and viewer should have read access")
	}
	if CanRead(RoleNone) {
		t.Error("none should not have read access")
	}
}

func TestJWTRejectsInvalidClaimsAndAlgorithms(t *testing.T) {
	cfg := &config.Config{Auth: config.AuthConfig{JWTSecret: "test-secret", TokenExpiryHours: 1}}
	auth := NewAuthService(cfg)
	for _, tc := range []struct {
		name   string
		change func(*Claims)
		method jwt.SigningMethod
	}{
		{"missing expiry", func(c *Claims) { c.ExpiresAt = nil }, jwt.SigningMethodHS256},
		{"expired", func(c *Claims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) }, jwt.SigningMethodHS256},
		{"wrong issuer", func(c *Claims) { c.Issuer = "other" }, jwt.SigningMethodHS256},
		{"missing UID", func(c *Claims) { c.UID = "" }, jwt.SigningMethodHS256},
		{"mismatched subject", func(c *Claims) { c.Subject = "another-user" }, jwt.SigningMethodHS256},
		{"wrong algorithm", func(c *Claims) {}, jwt.SigningMethodHS384},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := &Claims{UID: "user", RegisteredClaims: jwt.RegisteredClaims{Subject: "user", Issuer: "peeriodic", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
			tc.change(claims)
			token, err := jwt.NewWithClaims(tc.method, claims).SignedString([]byte(cfg.Auth.JWTSecret))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = auth.ValidateToken(token); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
}
