package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/logoes0/peeriodic.git/middleware"
	"github.com/logoes0/peeriodic.git/models"
	"github.com/logoes0/peeriodic.git/services"
	"github.com/logoes0/peeriodic.git/utils"
)

// InviteHandler handles room invitations, sharing flows, and membership roles
type InviteHandler struct {
	dbService *services.DatabaseService
	wsService *services.WebSocketService
}

// NewInviteHandler creates a new InviteHandler instance
func NewInviteHandler(dbService *services.DatabaseService, wsService *services.WebSocketService) *InviteHandler {
	return &InviteHandler{
		dbService: dbService,
		wsService: wsService,
	}
}

// HandleCreateInvite generates a shareable invite token for a room
func (ih *InviteHandler) HandleCreateInvite(w http.ResponseWriter, r *http.Request, roomID string) {
	if r.Method != http.MethodPost {
		utils.MethodNotAllowed(w)
		return
	}

	userUID := middleware.GetUserUID(r.Context())
	role, err := ih.dbService.GetEffectiveRole(r.Context(), roomID, userUID)
	if err != nil {
		utils.NotFound(w, "Room not found")
		return
	}

	if !services.CanWrite(role) {
		utils.Forbidden(w, "Only room owners and editors can create invite links")
		return
	}

	var req models.CreateInviteRequest
	if err := utils.DecodeJSON(w, r, &req); err != nil {
		return
	}

	targetRole := strings.ToLower(strings.TrimSpace(req.Role))
	if targetRole == "" {
		targetRole = services.RoleEditor
	}
	if targetRole != services.RoleEditor && targetRole != services.RoleViewer {
		utils.BadRequest(w, "Role must be 'editor' or 'viewer'")
		return
	}
	if req.ExpiresInHours < 0 || req.ExpiresInHours > 8760 {
		utils.BadRequest(w, "Invite expiry must be between 0 and 8760 hours")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresInHours > 0 {
		exp := time.Now().Add(time.Duration(req.ExpiresInHours) * time.Hour)
		expiresAt = &exp
	}

	token := uuid.New().String()
	var createdBy *string
	if userUID != "" {
		createdBy = &userUID
	}

	invite, err := ih.dbService.CreateRoomInvite(r.Context(), roomID, token, targetRole, createdBy, expiresAt)
	if err != nil {
		utils.InternalServerError(w, "Failed to create room invite")
		return
	}

	utils.SuccessResponse(w, invite)
}

// HandleAcceptInvite handles accepting an invite link and adding the user as a room member
func (ih *InviteHandler) HandleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.MethodNotAllowed(w)
		return
	}

	userUID := middleware.GetUserUID(r.Context())
	if userUID == "" {
		utils.Unauthorized(w, "Authentication required to accept an invite")
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// Expected: /api/invites/:token/accept -> len >= 4
	if len(pathParts) != 4 || pathParts[3] != "accept" {
		utils.BadRequest(w, "Invalid invite acceptance URL")
		return
	}
	token := pathParts[2]

	room, grantedRole, err := ih.dbService.AcceptRoomInvite(r.Context(), token, userUID)
	if err != nil {
		utils.BadRequest(w, err.Error())
		return
	}

	utils.SuccessResponse(w, map[string]interface{}{
		"room": RoomResponse{
			ID:         room.ID,
			Title:      room.Title,
			CurrentRev: room.CurrentRev,
			PublicRole: room.PublicRole,
		},
		"role": grantedRole,
	})
}

// HandleMembers handles listing members of a room
func (ih *InviteHandler) HandleMembers(w http.ResponseWriter, r *http.Request, roomID string) {
	switch r.Method {
	case http.MethodGet:
		userUID := middleware.GetUserUID(r.Context())
		role, err := ih.dbService.GetEffectiveRole(r.Context(), roomID, userUID)
		if err != nil {
			utils.NotFound(w, "Room not found")
			return
		}

		if !services.CanRead(role) {
			utils.Forbidden(w, "You do not have access to view members of this room")
			return
		}

		members, err := ih.dbService.ListRoomMembers(r.Context(), roomID)
		if err != nil {
			utils.InternalServerError(w, "Failed to list room members")
			return
		}

		utils.SuccessResponse(w, members)

	default:
		utils.MethodNotAllowed(w)
	}
}

// HandleMemberOperations handles updating or removing a member in a room
func (ih *InviteHandler) HandleMemberOperations(w http.ResponseWriter, r *http.Request, roomID, targetUID string) {
	userUID := middleware.GetUserUID(r.Context())
	currentRole, err := ih.dbService.GetEffectiveRole(r.Context(), roomID, userUID)
	if err != nil {
		utils.NotFound(w, "Room not found")
		return
	}
	if services.CanAdmin(currentRole) && userUID == targetUID {
		utils.Forbidden(w, "The owner cannot be removed or demoted")
		return
	}

	switch r.Method {
	case http.MethodPut:
		// Only owners can change other members' roles
		if !services.CanAdmin(currentRole) {
			utils.Forbidden(w, "Only room owners can modify member roles")
			return
		}

		var req models.UpdateMemberRoleRequest
		if err := utils.DecodeJSON(w, r, &req); err != nil {
			return
		}

		newRole := strings.ToLower(strings.TrimSpace(req.Role))
		if newRole != services.RoleEditor && newRole != services.RoleViewer {
			utils.BadRequest(w, "Role must be 'editor' or 'viewer'")
			return
		}

		if err := ih.dbService.UpdateRoomMemberRole(r.Context(), roomID, targetUID, newRole); err != nil {
			utils.InternalServerError(w, "Failed to update member role")
			return
		}
		ih.wsService.CloseRoomConnections(roomID, targetUID)

		utils.SuccessResponse(w, map[string]string{
			"status":   "updated",
			"room_id":  roomID,
			"user_uid": targetUID,
			"role":     newRole,
		})

	case http.MethodDelete:
		// Owner can remove anyone; user can remove themselves (leave room)
		if !services.CanAdmin(currentRole) && userUID != targetUID {
			utils.Forbidden(w, "You do not have permission to remove this member")
			return
		}

		if err := ih.dbService.RemoveRoomMember(r.Context(), roomID, targetUID); err != nil {
			utils.InternalServerError(w, "Failed to remove member")
			return
		}
		ih.wsService.CloseRoomConnections(roomID, targetUID)

		utils.SuccessResponse(w, map[string]string{
			"status":   "removed",
			"room_id":  roomID,
			"user_uid": targetUID,
		})

	default:
		utils.MethodNotAllowed(w)
	}
}
