package routers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/logoes0/peeriodic.git/handlers"
	"github.com/logoes0/peeriodic.git/logger"
	"github.com/logoes0/peeriodic.git/middleware"
	"github.com/logoes0/peeriodic.git/services"
	"github.com/logoes0/peeriodic.git/utils"
)

// Router handles all HTTP routing
type Router struct {
	roomHandler     *handlers.RoomHandler
	documentHandler *handlers.DocumentHandler
	authHandler     *handlers.AuthHandler
	inviteHandler   *handlers.InviteHandler
	healthHandler   *handlers.HealthHandler
	metricsHandler  *handlers.MetricsHandler
	wsService       *services.WebSocketService
	authService     *services.AuthService
}

// NewRouter creates a new router instance
func NewRouter(dbService *services.DatabaseService, wsService *services.WebSocketService, authService *services.AuthService) *Router {
	return &Router{
		roomHandler:     handlers.NewRoomHandler(dbService, wsService),
		documentHandler: handlers.NewDocumentHandler(dbService, wsService),
		authHandler:     handlers.NewAuthHandler(dbService, authService),
		inviteHandler:   handlers.NewInviteHandler(dbService, wsService),
		healthHandler:   handlers.NewHealthHandler(dbService),
		metricsHandler:  handlers.NewMetricsHandler(wsService),
		wsService:       wsService,
		authService:     authService,
	}
}

// wrap keeps recovery inside logging so panics are counted as failed requests.
func (r *Router) wrap(h http.HandlerFunc, authMW func(http.HandlerFunc) http.HandlerFunc, limiter *middleware.IPRateLimiter) http.HandlerFunc {
	handler := h
	if authMW != nil {
		handler = authMW(handler)
	}
	if limiter != nil {
		handler = limiter.Middleware(handler)
	}
	bounded := func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		handler(w, req.WithContext(ctx))
	}
	return middleware.RequestID(middleware.Logging(middleware.Recoverer(middleware.CORS(middleware.MaxBodyBytes(middleware.DefaultMaxBodyBytes)(bounded)))))
}

// SetupRoutes configures all application routes with middleware
func (r *Router) SetupRoutes() http.Handler {
	mux := http.NewServeMux()
	// 1. Health & readiness probes + metrics (no rate limiting or auth)
	mux.HandleFunc("/healthz", r.healthHandler.HandleHealthz)
	mux.HandleFunc("/readyz", r.healthHandler.HandleReadyz)
	mux.HandleFunc("/metrics", r.metricsHandler.HandleMetrics)

	// 2. WebSocket endpoint (handles connection upgrade directly)
	mux.HandleFunc("/ws", middleware.RequestID(middleware.CORS(r.handleWebSocket)))

	// 3. Auth API endpoints (uses strict AuthAPILimiter to protect against brute force)
	mux.HandleFunc("/api/auth/register", r.wrap(r.authHandler.HandleRegister, nil, middleware.AuthAPILimiter))
	mux.HandleFunc("/api/auth/login", r.wrap(r.authHandler.HandleLogin, nil, middleware.AuthAPILimiter))
	mux.HandleFunc("/api/auth/me", r.wrap(r.authHandler.HandleMe, middleware.RequireAuth(r.authService), middleware.GeneralAPILimiter))

	// 4. Room & Document API endpoints (General rate limiter)
	mux.HandleFunc("/api/rooms", r.wrap(r.roomHandler.HandleRooms, middleware.OptionalAuth(r.authService), middleware.GeneralAPILimiter))
	mux.HandleFunc("/api/save", r.wrap(r.documentHandler.HandleSave, middleware.OptionalAuth(r.authService), middleware.GeneralAPILimiter))

	// 5. Room-specific sub-routes (/api/rooms/:id/...)
	mux.HandleFunc("/api/rooms/", r.wrap(r.handleRoomOperations, middleware.OptionalAuth(r.authService), middleware.GeneralAPILimiter))

	// 6. Invite token acceptance (/api/invites/:token/accept)
	mux.HandleFunc("/api/invites/", r.wrap(r.inviteHandler.HandleAcceptInvite, middleware.RequireAuth(r.authService), middleware.GeneralAPILimiter))
	return mux
}

// handleWebSocket handles WebSocket connections
func (r *Router) handleWebSocket(w http.ResponseWriter, req *http.Request) {
	r.wsService.HandleConnection(w, req, r.roomHandler.DBService)
}

// handleRoomOperations handles room-specific operations and sub-resources
func (r *Router) handleRoomOperations(w http.ResponseWriter, req *http.Request) {
	cleanPath := strings.Trim(req.URL.Path, "/")
	pathParts := strings.Split(cleanPath, "/")

	// Expected formats:
	// api/rooms/:id
	// api/rooms/:id/invites
	// api/rooms/:id/members
	// api/rooms/:id/members/:uid
	if len(pathParts) < 3 {
		utils.BadRequest(w, "Invalid room URL")
		return
	}

	roomID := pathParts[2]
	if roomID == "" {
		utils.BadRequest(w, "Missing room ID")
		return
	}

	// 1. Direct operations on /api/rooms/:id
	if len(pathParts) == 3 {
		switch req.Method {
		case http.MethodGet:
			r.roomHandler.HandleRoomByID(w, req)
		case http.MethodDelete:
			r.roomHandler.HandleDeleteRoom(w, req)
		default:
			utils.MethodNotAllowed(w)
		}
		return
	}

	subResource := pathParts[3]

	// 2. Sub-resource: /api/rooms/:id/invites
	if subResource == "invites" && len(pathParts) == 4 {
		r.inviteHandler.HandleCreateInvite(w, req, roomID)
		return
	}

	// 3. Sub-resource: /api/rooms/:id/members
	if subResource == "members" {
		if len(pathParts) == 4 {
			r.inviteHandler.HandleMembers(w, req, roomID)
		} else if len(pathParts) == 5 {
			targetUID := pathParts[4]
			r.inviteHandler.HandleMemberOperations(w, req, roomID, targetUID)
		} else {
			utils.BadRequest(w, "Invalid members path")
		}
		return
	}

	logger.Warn("Unknown sub-resource requested for room", "sub_resource", subResource, "room_id", roomID)
	utils.NotFound(w, "Resource not found")
}
