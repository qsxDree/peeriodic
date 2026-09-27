package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/logoes0/peeriodic.git/services"
	"github.com/logoes0/peeriodic.git/utils"
)

type contextKey string

const (
	UserUIDKey   contextKey = "user_uid"
	UserEmailKey contextKey = "user_email"
	UserNameKey  contextKey = "user_name"
)

// ExtractToken extracts a bearer JWT token from Authorization header or ?token= query param
func ExtractToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	// Fallback to query parameter (common for WebSocket connections)
	return r.URL.Query().Get("token")
}

// GetUserUID retrieves the authenticated user's UID from context
func GetUserUID(ctx context.Context) string {
	if val, ok := ctx.Value(UserUIDKey).(string); ok {
		return val
	}
	return ""
}

// GetUserEmail retrieves the authenticated user's email from context
func GetUserEmail(ctx context.Context) string {
	if val, ok := ctx.Value(UserEmailKey).(string); ok {
		return val
	}
	return ""
}

// GetUserName retrieves the authenticated user's display name from context
func GetUserName(ctx context.Context) string {
	if val, ok := ctx.Value(UserNameKey).(string); ok {
		return val
	}
	return ""
}

// WithUserClaims returns a new context with user claims attached
func WithUserClaims(ctx context.Context, uid, email, name string) context.Context {
	ctx = context.WithValue(ctx, UserUIDKey, uid)
	ctx = context.WithValue(ctx, UserEmailKey, email)
	return context.WithValue(ctx, UserNameKey, name)
}

// RequireAuth middleware strictly enforces authentication; returns 401 if missing/invalid
func RequireAuth(authService *services.AuthService) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			tokenString := ExtractToken(r)
			if tokenString == "" {
				utils.Unauthorized(w, "Authentication required")
				return
			}

			claims, err := authService.ValidateToken(tokenString)
			if err != nil {
				utils.Unauthorized(w, "Invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), UserUIDKey, claims.UID)
			ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
			ctx = context.WithValue(ctx, UserNameKey, claims.Name)

			next(w, r.WithContext(ctx))
		}
	}
}

// OptionalAuth middleware extracts credentials if present, but permits anonymous guests
func OptionalAuth(authService *services.AuthService) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			tokenString := ExtractToken(r)
			if tokenString != "" {
				if claims, err := authService.ValidateToken(tokenString); err == nil {
					ctx := context.WithValue(r.Context(), UserUIDKey, claims.UID)
					ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
					ctx = context.WithValue(ctx, UserNameKey, claims.Name)
					next(w, r.WithContext(ctx))
					return
				}
			}

			next(w, r)
		}
	}
}
