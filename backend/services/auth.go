package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/logoes0/peeriodic.git/config"
	"golang.org/x/crypto/bcrypt"
)

// Document roles
const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
	RoleNone   = "none"
)

// CanWrite returns true if the role permits editing document content
func CanWrite(role string) bool {
	return role == RoleOwner || role == RoleEditor
}

// CanAdmin returns true if the role permits managing members and deleting the document
func CanAdmin(role string) bool {
	return role == RoleOwner
}

// CanRead returns true if the role permits viewing the document
func CanRead(role string) bool {
	return role == RoleOwner || role == RoleEditor || role == RoleViewer
}

// Claims represents JWT claims for an authenticated user
type Claims struct {
	UID   string `json:"uid"`
	Email string `json:"email"`
	Name  string `json:"name"`
	jwt.RegisteredClaims
}

// AuthService handles authentication, JWT token lifecycle, and password hashing
type AuthService struct {
	config *config.Config
}

// NewAuthService creates a new AuthService instance
func NewAuthService(cfg *config.Config) *AuthService {
	return &AuthService{
		config: cfg,
	}
}

// HashPassword hashes a raw password using bcrypt
func (as *AuthService) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// CheckPasswordHash compares a raw password against a bcrypt hash
func (as *AuthService) CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateToken generates a signed JWT token for a user
func (as *AuthService) GenerateToken(userUID, email, name string) (string, error) {
	expiryDuration := time.Duration(as.config.Auth.TokenExpiryHours) * time.Hour
	expirationTime := time.Now().Add(expiryDuration)

	claims := &Claims{
		UID:   userUID,
		Email: email,
		Name:  name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userUID,
			Issuer:    "peeriodic",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(as.config.Auth.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken parses and verifies a signed JWT token
func (as *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(as.config.Auth.JWTSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if !token.Valid {
		return nil, errors.New("token is not valid")
	}

	return claims, nil
}
