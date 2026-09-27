package routers

import (
	"log"
	"net/http"
	"strings"

	"github.com/logoes0/peeriodic.git/handlers"
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
	wsService       *services.WebSocketService
	authService     *services.AuthService
}

// NewRouter creates a new router instance
func NewRouter(dbService *services.DatabaseService, wsService *services.WebSocketService, authService *services.AuthService) *Router {
	return &Router{
		roomHandler:     handlers.NewRoomHandler(dbService),
		documentHandler: handlers.NewDocumentHandler(dbService),
		authHandler:     handlers.NewAuthHandler(dbService, authService),
		inviteHandler:   handlers.NewInviteHandler(dbService, authService),
		wsService:       wsService,
		authService:     authService,
	}
}

// SetupRoutes configures all application routes with middleware
func (r *Router) SetupRoutes() {
	// WebSocket endpoint - NO middleware (WebSocket needs direct access to response writer)
	http.HandleFunc("/ws", r.handleWebSocket)

	// Auth API endpoints
	http.HandleFunc("/api/auth/register", middleware.Logging(middleware.CORS(r.authHandler.HandleRegister)))
	http.HandleFunc("/api/auth/login", middleware.Logging(middleware.CORS(r.authHandler.HandleLogin)))
	http.HandleFunc("/api/auth/me", middleware.Logging(middleware.CORS(middleware.RequireAuth(r.authService)(r.authHandler.HandleMe))))

	// Room & Document API endpoints
	http.HandleFunc("/api/rooms", middleware.Logging(middleware.CORS(middleware.OptionalAuth(r.authService)(r.handleRooms))))
	http.HandleFunc("/api/save", middleware.Logging(middleware.CORS(middleware.OptionalAuth(r.authService)(r.handleSave))))

	// Handle room-specific operations and sub-routes (/api/rooms/:id/...)
	http.HandleFunc("/api/rooms/", middleware.Logging(middleware.CORS(middleware.OptionalAuth(r.authService)(r.handleRoomOperations))))

	// Handle invite token acceptance (/api/invites/:token/accept)
	http.HandleFunc("/api/invites/", middleware.Logging(middleware.CORS(middleware.RequireAuth(r.authService)(r.inviteHandler.HandleAcceptInvite))))
}

// handleWebSocket handles WebSocket connections
func (r *Router) handleWebSocket(w http.ResponseWriter, req *http.Request) {
	r.wsService.HandleConnection(w, req, r.roomHandler.DBService)
}

// handleRooms handles room listing and creation
func (r *Router) handleRooms(w http.ResponseWriter, req *http.Request) {
	r.roomHandler.HandleRooms(w, req)
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
	if subResource == "invites" {
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

	log.Printf("⚠️ Unknown sub-resource '%s' for room %s", subResource, roomID)
	utils.NotFound(w, "Resource not found")
}

// handleSave handles document saving
func (r *Router) handleSave(w http.ResponseWriter, req *http.Request) {
	r.documentHandler.HandleSave(w, req)
}
