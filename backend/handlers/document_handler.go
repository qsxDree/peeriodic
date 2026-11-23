package handlers

import (
	"errors"
	"net/http"

	"github.com/logoes0/peeriodic.git/logger"
	"github.com/logoes0/peeriodic.git/middleware"
	"github.com/logoes0/peeriodic.git/services"
	"github.com/logoes0/peeriodic.git/utils"
)

// DocumentHandler handles document-related HTTP requests
type DocumentHandler struct {
	dbService *services.DatabaseService
	wsService *services.WebSocketService
}

// NewDocumentHandler creates a new document handler instance
func NewDocumentHandler(dbService *services.DatabaseService, wsService *services.WebSocketService) *DocumentHandler {
	return &DocumentHandler{
		dbService: dbService,
		wsService: wsService,
	}
}

// SaveDocumentRequest represents the request body for saving a document
type SaveDocumentRequest struct {
	Content *string `json:"content"`
}

// SaveDocumentResponse represents the response for saving a document
type SaveDocumentResponse struct {
	Status        string `json:"status"`
	RoomID        string `json:"roomId"`
	ContentLength int    `json:"contentLength"`
}

// HandleSave handles document saving
func (dh *DocumentHandler) HandleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.MethodNotAllowed(w)
		return
	}

	// Get room ID from query parameters
	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		utils.BadRequest(w, "Missing room ID")
		return
	}

	// Parse request body
	var req SaveDocumentRequest
	if err := utils.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Content == nil {
		utils.BadRequest(w, "Content is required")
		return
	}

	// Check write permissions
	userUID := middleware.GetUserUID(r.Context())
	role, err := dh.dbService.GetEffectiveRole(r.Context(), roomID, userUID)
	if err != nil || !services.CanWrite(role) {
		utils.Forbidden(w, "You have read-only (viewer) access to this document")
		return
	}

	// Save document to database
	err = dh.wsService.SaveDocument(r.Context(), dh.dbService, roomID, *req.Content)
	if errors.Is(err, services.ErrConflict) {
		utils.ErrorResponse(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		logger.Error("Failed to save document", "room_id", roomID, "error", err)
		utils.InternalServerError(w, "Failed to save document")
		return
	}

	response := SaveDocumentResponse{
		Status:        "success",
		RoomID:        roomID,
		ContentLength: len(*req.Content),
	}

	utils.SuccessResponse(w, response)
}
