package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/logoes0/peeriodic.git/middleware"
	"github.com/logoes0/peeriodic.git/models"
	"github.com/logoes0/peeriodic.git/services"
	"github.com/logoes0/peeriodic.git/utils"
)

// AuthHandler handles user registration, login, and profile requests
type AuthHandler struct {
	dbService   *services.DatabaseService
	authService *services.AuthService
}

// NewAuthHandler creates a new AuthHandler instance
func NewAuthHandler(dbService *services.DatabaseService, authService *services.AuthService) *AuthHandler {
	return &AuthHandler{
		dbService:   dbService,
		authService: authService,
	}
}

// HandleRegister handles user registration
func (ah *AuthHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.MethodNotAllowed(w)
		return
	}

	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	if req.Email == "" || !strings.Contains(req.Email, "@") {
		utils.BadRequest(w, "Valid email is required")
		return
	}

	if len(req.Password) < 6 {
		utils.BadRequest(w, "Password must be at least 6 characters")
		return
	}

	if req.Name == "" {
		req.Name = strings.Split(req.Email, "@")[0]
	}

	// Check if user already exists
	existingUser, _ := ah.dbService.GetUserByEmail(req.Email)
	if existingUser != nil {
		utils.BadRequest(w, "A user with this email already exists")
		return
	}

	// Hash password
	passwordHash, err := ah.authService.HashPassword(req.Password)
	if err != nil {
		utils.InternalServerError(w, "Failed to secure password")
		return
	}

	userUID := uuid.New().String()
	user, err := ah.dbService.CreateUserWithPassword(userUID, req.Email, req.Name, passwordHash)
	if err != nil {
		utils.InternalServerError(w, "Failed to create user account")
		return
	}

	token, err := ah.authService.GenerateToken(user.UID, user.Email, user.Name)
	if err != nil {
		utils.InternalServerError(w, "Failed to generate authentication token")
		return
	}

	utils.SuccessResponse(w, models.AuthResponse{
		Token: token,
		User: models.User{
			ID:        user.ID,
			UID:       user.UID,
			Email:     user.Email,
			Name:      user.Name,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	})
}

// HandleLogin handles user authentication
func (ah *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.MethodNotAllowed(w)
		return
	}

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		utils.BadRequest(w, "Email and password are required")
		return
	}

	user, err := ah.dbService.GetUserByEmail(req.Email)
	if err != nil || user == nil {
		utils.Unauthorized(w, "Invalid email or password")
		return
	}

	if !ah.authService.CheckPasswordHash(req.Password, user.PasswordHash) {
		utils.Unauthorized(w, "Invalid email or password")
		return
	}

	token, err := ah.authService.GenerateToken(user.UID, user.Email, user.Name)
	if err != nil {
		utils.InternalServerError(w, "Failed to generate token")
		return
	}

	utils.SuccessResponse(w, models.AuthResponse{
		Token: token,
		User: models.User{
			ID:        user.ID,
			UID:       user.UID,
			Email:     user.Email,
			Name:      user.Name,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
	})
}

// HandleMe returns the currently authenticated user's profile
func (ah *AuthHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.MethodNotAllowed(w)
		return
	}

	uid := middleware.GetUserUID(r.Context())
	if uid == "" {
		utils.Unauthorized(w, "Authentication required")
		return
	}

	if ah.dbService != nil {
		user, err := ah.dbService.GetUserByUID(uid)
		if err != nil {
			utils.NotFound(w, "User not found")
			return
		}

		utils.SuccessResponse(w, models.User{
			ID:        user.ID,
			UID:       user.UID,
			Email:     user.Email,
			Name:      user.Name,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		})
		return
	}

	// Fallback to token claims if DB service is unavailable
	utils.SuccessResponse(w, models.User{
		UID:   uid,
		Email: middleware.GetUserEmail(r.Context()),
		Name:  middleware.GetUserName(r.Context()),
	})
}
