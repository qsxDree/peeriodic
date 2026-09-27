package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/models"
	"github.com/logoes0/peeriodic.git/ot"
)

var clientColors = []string{
	"#FF5722", "#E91E63", "#9C27B0", "#673AB7",
	"#3F51B5", "#2196F3", "#00BCD4", "#009688",
	"#4CAF50", "#8BC34A", "#FF9800", "#795548",
}

// ClientSession represents a connected user in a room
type ClientSession struct {
	Conn      *websocket.Conn
	ClientID  string
	UID       string
	Name      string
	Color     string
	Role      string
	Cursor    *models.CursorPosition
	writeMu   sync.Mutex
	Connected time.Time
}

// SendMessage safely writes a message to this client's WebSocket connection
func (cs *ClientSession) SendMessage(msg models.Message) error {
	cs.writeMu.Lock()
	defer cs.writeMu.Unlock()
	if cs.Conn == nil {
		return nil
	}
	return cs.Conn.WriteJSON(msg)
}

// RoomManager manages clients and collaborative document state for a specific room
type RoomManager struct {
	ID        string
	Clients   map[*websocket.Conn]*ClientSession
	DocState  *ot.DocumentState
	mu        sync.RWMutex
	lastSaved time.Time
}

// WebSocketService handles WebSocket connections and real-time collaboration
type WebSocketService struct {
	config      *config.Config
	authService *AuthService
	rooms       map[string]*RoomManager
	mu          sync.RWMutex
}

// NewWebSocketService creates a new WebSocket service instance
func NewWebSocketService(cfg *config.Config, authService *AuthService) *WebSocketService {
	if authService == nil {
		authService = NewAuthService(cfg)
	}
	return &WebSocketService{
		config:      cfg,
		authService: authService,
		rooms:       make(map[string]*RoomManager),
	}
}

// GetUpgrader returns a configured WebSocket upgrader
func (ws *WebSocketService) GetUpgrader() websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  ws.config.WebSocket.ReadBufferSize,
		WriteBufferSize: ws.config.WebSocket.WriteBufferSize,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		EnableCompression: true,
	}
}

// HandleConnection handles a new WebSocket connection with auth & role checking
func (ws *WebSocketService) HandleConnection(w http.ResponseWriter, r *http.Request, dbService *DatabaseService) {
	log.Printf("🌐 WebSocket request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("❌ PANIC in WebSocket handler: %v", rec)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Max-Age", "86400")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		log.Printf("❌ WebSocket connection attempt without room ID")
		http.Error(w, "Missing room ID", http.StatusBadRequest)
		return
	}

	// 1. Authenticate user from query parameter or header
	token := r.URL.Query().Get("token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token = strings.TrimSpace(parts[1])
			}
		}
	}

	userUID := r.URL.Query().Get("uid")
	userName := r.URL.Query().Get("name")

	if token != "" && ws.authService != nil {
		if claims, err := ws.authService.ValidateToken(token); err == nil {
			userUID = claims.UID
			userName = claims.Name
		}
	}

	// 2. Determine effective role and verify read permission
	role := RoleEditor
	if dbService != nil {
		effectiveRole, err := dbService.GetEffectiveRole(roomID, userUID)
		if err == nil {
			role = effectiveRole
		}
	}

	if role == RoleNone {
		log.Printf("❌ Access denied for user %s on room %s", userUID, roomID)
		http.Error(w, "Access denied to this document", http.StatusForbidden)
		return
	}

	// 3. Ensure room exists in database
	var initialContent string
	var currentRev int
	if dbService != nil {
		room, err := dbService.EnsureRoomExists(roomID)
		if err != nil {
			log.Printf("❌ Failed to ensure room exists: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		initialContent = room.Content
		currentRev = room.CurrentRev
	}

	upgrader := ws.GetUpgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket upgrade failed for room %s: %v", roomID, err)
		http.Error(w, "WebSocket upgrade failed", http.StatusInternalServerError)
		return
	}

	// Get or create room manager
	roomManager := ws.getOrCreateRoom(roomID, initialContent, currentRev)

	// Create client session
	clientID := generateRandomID(8)
	if userName == "" {
		userName = fmt.Sprintf("User-%s", clientID[:4])
	}
	colorIndex := time.Now().UnixNano() % int64(len(clientColors))
	session := &ClientSession{
		Conn:      conn,
		ClientID:  clientID,
		UID:       userUID,
		Name:      userName,
		Color:     clientColors[colorIndex],
		Role:      role,
		Connected: time.Now(),
	}

	// Register client in room
	roomManager.mu.Lock()
	roomManager.Clients[conn] = session
	clientCount := len(roomManager.Clients)
	presenceList := roomManager.getPresenceListUnsafe()
	content, rev := roomManager.DocState.GetSnapshot()
	roomManager.mu.Unlock()

	log.Printf("✅ Client %s (%s, role: %s) connected to room %s (total clients: %d)", clientID, userName, role, roomID, clientCount)
	defer ws.closeConnection(conn, roomID, dbService)

	// Send initial state to the newly connected client
	initMsg := models.Message{
		Type:     models.MsgTypeInit,
		Rev:      rev,
		Data:     content,
		ClientID: clientID,
		Name:     userName,
		Color:    session.Color,
		Role:     session.Role,
		Presence: presenceList,
	}
	if err := session.SendMessage(initMsg); err != nil {
		log.Printf("❌ Failed to send initial state to %s: %v", clientID, err)
		return
	}

	// Broadcast presence update to others in room
	roomManager.Broadcast(models.Message{
		Type:     models.MsgTypePresence,
		Presence: presenceList,
	}, conn)

	// Handle incoming messages
	ws.handleMessages(session, roomManager, dbService)
}

// handleMessages processes incoming WebSocket messages from a client
func (ws *WebSocketService) handleMessages(session *ClientSession, roomManager *RoomManager, dbService *DatabaseService) {
	conn := session.Conn
	roomID := roomManager.ID

	for {
		var msg models.Message
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("❌ Client %s disconnected unexpectedly from room %s: %v", session.ClientID, roomID, err)
			} else {
				log.Printf("📖 Client %s disconnected normally from room %s", session.ClientID, roomID)
			}
			break
		}

		switch msg.Type {
		case models.MsgTypeOp:
			ws.handleOpMessage(session, roomManager, msg, dbService)

		case models.MsgTypeCursor:
			if msg.Cursor != nil {
				session.Cursor = msg.Cursor
				// Broadcast cursor position to all other peers
				roomManager.Broadcast(models.Message{
					Type:     models.MsgTypeCursor,
					ClientID: session.ClientID,
					Name:     session.Name,
					Color:    session.Color,
					Role:     session.Role,
					Cursor:   session.Cursor,
				}, conn)
			}

		case models.MsgTypePresence:
			if msg.Name != "" {
				session.Name = msg.Name
			}
			if msg.Color != "" {
				session.Color = msg.Color
			}
			presenceList := roomManager.GetPresenceList()
			roomManager.Broadcast(models.Message{
				Type:     models.MsgTypePresence,
				Presence: presenceList,
			}, nil)

		case models.MsgTypeSync:
			ws.handleSyncRequest(session, roomManager, msg.Rev)

		case models.MsgTypePing:
			_ = session.SendMessage(models.Message{Type: models.MsgTypePong})

		// Backward compatibility for raw document replace
		case models.MsgTypeUpdate:
			ws.handleLegacyDocumentUpdate(session, roomManager, msg.Data, dbService)

		default:
			log.Printf("⚠️ Unknown message type '%s' from client %s in room %s", msg.Type, session.ClientID, roomID)
		}
	}
}

// handleOpMessage processes an OT operation from a client
func (ws *WebSocketService) handleOpMessage(session *ClientSession, roomManager *RoomManager, msg models.Message, dbService *DatabaseService) {
	// Enforce role-based editing permissions: Viewers cannot edit!
	if !CanWrite(session.Role) {
		log.Printf("⚠️ Rejecting edit from viewer %s in room %s", session.ClientID, roomManager.ID)
		_ = session.SendMessage(models.Message{
			Type: models.MsgTypeError,
			Data: "You have read-only access (viewer) to this document",
		})
		return
	}

	if len(msg.Ops) == 0 {
		return
	}

	newRev, transformedOps, err := roomManager.DocState.ApplyClientOps(msg.Rev, session.ClientID, msg.Ops, true)
	if err != nil {
		log.Printf("⚠️ OT error for client %s in room %s: %v", session.ClientID, roomManager.ID, err)
		// Request full resync
		content, rev := roomManager.DocState.GetSnapshot()
		_ = session.SendMessage(models.Message{
			Type:  models.MsgTypeSync,
			Rev:   rev,
			Data:  content,
			Color: "resync_required",
		})
		return
	}

	// 1. Send Ack to the submitting client
	_ = session.SendMessage(models.Message{
		Type: models.MsgTypeAck,
		Rev:  newRev,
	})

	// 2. Broadcast transformed operation to all peers
	roomManager.Broadcast(models.Message{
		Type:     models.MsgTypeOp,
		Rev:      newRev,
		ClientID: session.ClientID,
		Ops:      transformedOps,
	}, session.Conn)

	// 3. Persist operation asynchronously
	if dbService != nil {
		go func() {
			for _, op := range transformedOps {
				if err := dbService.SaveOperation(roomManager.ID, newRev, session.ClientID, string(op.Type), op.Pos, op.Text, op.Len); err != nil {
					log.Printf("⚠️ Failed to persist operation in room %s: %v", roomManager.ID, err)
				}
			}

			// Save snapshot periodically or when rev is a multiple of 10
			if newRev%10 == 0 || time.Since(roomManager.lastSaved) > 30*time.Second {
				content, rev := roomManager.DocState.GetSnapshot()
				if err := dbService.UpdateRoomContentAndRev(roomManager.ID, content, rev); err != nil {
					log.Printf("⚠️ Failed to update room snapshot %s: %v", roomManager.ID, err)
				} else {
					roomManager.lastSaved = time.Now()
				}
			}
		}()
	}
}

// handleSyncRequest handles a client resync request
func (ws *WebSocketService) handleSyncRequest(session *ClientSession, roomManager *RoomManager, clientRev int) {
	records, err := roomManager.DocState.GetOpsSince(clientRev)
	if err == nil && len(records) > 0 {
		// Client is within recent history buffer, send missing ops
		var opsToSend []ot.Operation
		for _, rec := range records {
			opsToSend = append(opsToSend, rec.Ops...)
		}
		_, currentRev := roomManager.DocState.GetSnapshot()
		_ = session.SendMessage(models.Message{
			Type: models.MsgTypeSync,
			Rev:  currentRev,
			Ops:  opsToSend,
		})
		return
	}

	// Client is too far behind or on mismatch: send full snapshot
	content, currentRev := roomManager.DocState.GetSnapshot()
	_ = session.SendMessage(models.Message{
		Type: models.MsgTypeSync,
		Rev:  currentRev,
		Data: content,
	})
}

// handleLegacyDocumentUpdate maintains backwards compatibility with raw string updates
func (ws *WebSocketService) handleLegacyDocumentUpdate(session *ClientSession, roomManager *RoomManager, content string, dbService *DatabaseService) {
	if !CanWrite(session.Role) {
		_ = session.SendMessage(models.Message{
			Type: models.MsgTypeError,
			Data: "You have read-only access (viewer) to this document",
		})
		return
	}

	oldContent, currentRev := roomManager.DocState.GetSnapshot()
	if content == oldContent {
		return
	}

	// Synthesize a delete and insert op
	var ops []ot.Operation
	oldLen := len([]rune(oldContent))
	if oldLen > 0 {
		ops = append(ops, ot.Operation{
			Type: ot.OpDelete,
			Pos:  0,
			Len:  oldLen,
		})
	}
	if len(content) > 0 {
		ops = append(ops, ot.Operation{
			Type: ot.OpInsert,
			Pos:  0,
			Text: content,
		})
	}

	ws.handleOpMessage(session, roomManager, models.Message{
		Rev: currentRev,
		Ops: ops,
	}, dbService)
}

// getOrCreateRoom returns an existing room manager or creates a new one
func (ws *WebSocketService) getOrCreateRoom(roomID, initialContent string, initialRev int) *RoomManager {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if room, exists := ws.rooms[roomID]; exists {
		return room
	}

	room := &RoomManager{
		ID:        roomID,
		Clients:   make(map[*websocket.Conn]*ClientSession),
		DocState:  ot.NewDocumentState(initialContent, initialRev),
		lastSaved: time.Now(),
	}
	ws.rooms[roomID] = room
	return room
}

// closeConnection removes a client from the room and cleans up
func (ws *WebSocketService) closeConnection(conn *websocket.Conn, roomID string, dbService *DatabaseService) {
	conn.Close()

	ws.mu.RLock()
	room, exists := ws.rooms[roomID]
	ws.mu.RUnlock()

	if !exists {
		return
	}

	room.mu.Lock()
	session, hadSession := room.Clients[conn]
	delete(room.Clients, conn)
	remainingClients := len(room.Clients)
	presenceList := room.getPresenceListUnsafe()
	room.mu.Unlock()

	if hadSession {
		log.Printf("Client %s disconnected from room %s (%d clients remaining)", session.ClientID, roomID, remainingClients)
	}

	// Broadcast updated presence to remaining clients
	room.Broadcast(models.Message{
		Type:     models.MsgTypePresence,
		Presence: presenceList,
	}, nil)

	// Persist snapshot if room is now empty
	if remainingClients == 0 {
		content, rev := room.DocState.GetSnapshot()
		if dbService != nil {
			_ = dbService.UpdateRoomContentAndRev(roomID, content, rev)
		}

		ws.mu.Lock()
		delete(ws.rooms, roomID)
		ws.mu.Unlock()
		log.Printf("Room %s cleaned up and persisted (no clients remaining)", roomID)
	}
}

// Broadcast sends a message to all clients in the room except optional excluded connection
func (rm *RoomManager) Broadcast(message models.Message, excludeConn *websocket.Conn) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for conn, session := range rm.Clients {
		if conn != excludeConn {
			if err := session.SendMessage(message); err != nil {
				log.Printf("⚠️ Failed to broadcast to client %s: %v", session.ClientID, err)
			}
		}
	}
}

// GetPresenceList returns list of connected users in room
func (rm *RoomManager) GetPresenceList() []models.UserPresence {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.getPresenceListUnsafe()
}

func (rm *RoomManager) getPresenceListUnsafe() []models.UserPresence {
	list := make([]models.UserPresence, 0, len(rm.Clients))
	for _, session := range rm.Clients {
		list = append(list, models.UserPresence{
			ClientID:    session.ClientID,
			UID:         session.UID,
			Name:        session.Name,
			Color:       session.Color,
			Role:        session.Role,
			Cursor:      session.Cursor,
			ConnectedAt: session.Connected,
		})
	}
	return list
}

// GetRoomStats returns active rooms and their client counts
func (ws *WebSocketService) GetRoomStats() map[string]int {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	stats := make(map[string]int)
	for roomID, room := range ws.rooms {
		room.mu.RLock()
		stats[roomID] = len(room.Clients)
		room.mu.RUnlock()
	}
	return stats
}

func generateRandomID(length int) string {
	b := make([]byte, length/2+1)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:length]
}
