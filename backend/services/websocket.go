package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/logger"
	"github.com/logoes0/peeriodic.git/models"
	"github.com/logoes0/peeriodic.git/ot"
	"github.com/logoes0/peeriodic.git/utils"
)

var clientColors = []string{
	"#FF5722", "#E91E63", "#9C27B0", "#673AB7",
	"#3F51B5", "#2196F3", "#00BCD4", "#009688",
	"#4CAF50", "#8BC34A", "#FF9800", "#795548",
}

type ClientSession struct {
	Conn        *websocket.Conn
	ClientID    string
	UID         string
	Name        string
	Color       string
	Role        string
	Cursor      *models.CursorPosition
	Connected   time.Time
	writeMu     sync.Mutex
	expiresAt   time.Time
	initialized bool
}

func (cs *ClientSession) SendMessage(msg models.Message) error {
	cs.writeMu.Lock()
	defer cs.writeMu.Unlock()
	if cs.Conn == nil {
		return nil
	}
	if !cs.initialized && msg.Type != models.MsgTypeInit {
		return nil
	}
	cs.initialized = true
	if err := cs.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if err := cs.Conn.WriteJSON(msg); err != nil {
		_ = cs.Conn.Close()
		return err
	}
	return nil
}

type RoomManager struct {
	ID       string
	Clients  map[*websocket.Conn]*ClientSession
	DocState *ot.DocumentState
	mu       sync.RWMutex
	// Keep revision commits, acknowledgements and broadcasts in the same order.
	opMu sync.Mutex
}

// shortcut: rooms are coordinated in one process; add shared coordination before running multiple replicas.
type WebSocketService struct {
	config      *config.Config
	authService *AuthService
	rooms       map[string]*RoomManager
	mu          sync.RWMutex
	stopping    bool
	clients     sync.WaitGroup
}

func NewWebSocketService(cfg *config.Config, authService *AuthService) *WebSocketService {
	if authService == nil {
		authService = NewAuthService(cfg)
	}
	return &WebSocketService{config: cfg, authService: authService, rooms: make(map[string]*RoomManager)}
}

func (ws *WebSocketService) GetUpgrader() websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:   ws.config.WebSocket.ReadBufferSize,
		WriteBufferSize:  ws.config.WebSocket.WriteBufferSize,
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			parsed, err := url.Parse(origin)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
				return false
			}
			if strings.EqualFold(parsed.Host, r.Host) {
				return true
			}
			for _, allowed := range ws.config.WebSocket.AllowedOrigins {
				if strings.EqualFold(origin, allowed) {
					return true
				}
			}
			return false
		},
	}
}

func (ws *WebSocketService) HandleConnection(w http.ResponseWriter, r *http.Request, db *DatabaseService) {
	ws.mu.RLock()
	stopping := ws.stopping
	ws.mu.RUnlock()
	if stopping {
		utils.ErrorResponse(w, http.StatusServiceUnavailable, "Server is shutting down")
		return
	}
	if r.Method != http.MethodGet {
		utils.MethodNotAllowed(w)
		return
	}
	roomID := r.URL.Query().Get("room")
	if roomID == "" || len(roomID) > 255 {
		utils.BadRequest(w, "Invalid room ID")
		return
	}
	userUID, userName := "", r.URL.Query().Get("name")
	token := r.URL.Query().Get("token")
	if header := r.Header.Get("Authorization"); header != "" {
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			utils.Unauthorized(w, "Invalid authorization header")
			return
		}
		token = parts[1]
	}
	var expiresAt time.Time
	if token != "" {
		claims, err := ws.authService.ValidateToken(token)
		if err != nil {
			utils.Unauthorized(w, "Invalid or expired token")
			return
		}
		userUID, userName = claims.UID, claims.Name
		expiresAt = claims.ExpiresAt.Time
	}
	upgrader := ws.GetUpgrader()
	if !websocket.IsWebSocketUpgrade(r) {
		utils.BadRequest(w, "WebSocket upgrade required")
		return
	}
	if !upgrader.CheckOrigin(r) {
		utils.Forbidden(w, "Origin is not allowed")
		return
	}
	role := RoleEditor
	if db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		effectiveRole, err := db.GetEffectiveRole(ctx, roomID, userUID)
		// Guest documents are created locally by the existing frontend.
		if errors.Is(err, ErrNotFound) && userUID == "" {
			if err = db.EnsureGuestRoom(ctx, roomID); err == nil {
				effectiveRole, err = db.GetEffectiveRole(ctx, roomID, userUID)
			}
		}
		cancel()
		if errors.Is(err, ErrNotFound) {
			utils.NotFound(w, "Room not found")
			return
		}
		if err != nil {
			utils.InternalServerError(w, "Failed to check document access")
			return
		}
		role = effectiveRole
	}
	if !CanRead(role) {
		utils.Forbidden(w, "Access denied to this document")
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1024 * 1024)

	clientID := uuid.NewString()
	if userName == "" {
		userName = fmt.Sprintf("User-%s", clientID[:4])
	}
	if len([]rune(userName)) > 255 {
		userName = string([]rune(userName)[:255])
	}
	session := &ClientSession{Conn: conn, ClientID: clientID, UID: userUID, Name: userName,
		Color: clientColors[time.Now().UnixNano()%int64(len(clientColors))], Role: role,
		Connected: time.Now(), expiresAt: expiresAt}

	// Registration and removal share one lock so reconnects cannot join an evicted room.
	ws.mu.Lock()
	if ws.stopping {
		ws.mu.Unlock()
		return
	}
	room := ws.rooms[roomID]
	if room == nil {
		content, rev := "", 0
		if db != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			stored, loadErr := db.GetRoom(ctx, roomID)
			cancel()
			if loadErr != nil {
				ws.mu.Unlock()
				return
			}
			content, rev = stored.Content, stored.CurrentRev
		}
		room = newRoomManager(roomID, content, rev)
		ws.rooms[roomID] = room
	}
	room.mu.Lock()
	room.Clients[conn] = session
	room.mu.Unlock()
	ws.clients.Add(1)
	ws.mu.Unlock()
	defer ws.clients.Done()
	defer ws.closeConnection(conn, roomID)

	room.opMu.Lock()
	content, rev := room.DocState.GetSnapshot()
	presence := room.GetPresenceList()
	err = session.SendMessage(models.Message{Type: models.MsgTypeInit, Rev: rev, Data: content,
		ClientID: clientID, Name: userName, Color: session.Color, Role: role, Presence: presence})
	if err == nil {
		room.Broadcast(models.Message{Type: models.MsgTypePresence, Presence: presence}, conn)
	}
	room.opMu.Unlock()
	if err != nil {
		return
	}
	ws.handleMessages(r.Context(), session, room, db)
}

func (ws *WebSocketService) handleMessages(ctx context.Context, session *ClientSession, room *RoomManager, db *DatabaseService) {
	conn := session.Conn
	for {
		deadline := time.Now().Add(90 * time.Second)
		if !session.expiresAt.IsZero() && session.expiresAt.Before(deadline) {
			deadline = session.expiresAt
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return
		}
		var msg models.Message
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		if !session.expiresAt.IsZero() && !time.Now().Before(session.expiresAt) {
			return
		}
		if db != nil && msg.Type != models.MsgTypeOp && msg.Type != models.MsgTypeUpdate {
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			role, err := db.GetEffectiveRole(checkCtx, room.ID, session.UID)
			cancel()
			if err != nil || !CanRead(role) {
				return
			}
		}
		switch msg.Type {
		case models.MsgTypeOp:
			ws.handleOpMessage(ctx, session, room, msg, db)
		case models.MsgTypeCursor:
			if msg.Cursor != nil && msg.Cursor.Pos >= 0 && msg.Cursor.SelStart >= 0 && msg.Cursor.SelEnd >= 0 {
				room.mu.Lock()
				session.Cursor = msg.Cursor
				reply := models.Message{Type: models.MsgTypeCursor, ClientID: session.ClientID,
					Name: session.Name, Color: session.Color, Role: session.Role, Cursor: session.Cursor}
				room.mu.Unlock()
				room.Broadcast(reply, conn)
			}
		case models.MsgTypePresence:
			room.mu.Lock()
			if msg.Name != "" && len([]rune(msg.Name)) <= 255 {
				session.Name = msg.Name
			}
			if len(msg.Color) == 7 && msg.Color[0] == '#' && strings.Trim(msg.Color[1:], "0123456789abcdefABCDEF") == "" {
				session.Color = msg.Color
			}
			presence := room.getPresenceListUnsafe()
			room.mu.Unlock()
			room.Broadcast(models.Message{Type: models.MsgTypePresence, Presence: presence}, nil)
		case models.MsgTypeSync:
			ws.handleSyncRequest(session, room)
		case models.MsgTypePing:
			_ = session.SendMessage(models.Message{Type: models.MsgTypePong})
		case models.MsgTypeUpdate:
			ws.handleLegacyDocumentUpdate(ctx, session, room, msg.Data, db)
		default:
			_ = session.SendMessage(models.Message{Type: models.MsgTypeError, Data: "Unknown message type"})
		}
	}
}

func (ws *WebSocketService) handleOpMessage(ctx context.Context, session *ClientSession, room *RoomManager, msg models.Message, db *DatabaseService) {
	room.opMu.Lock()
	defer room.opMu.Unlock()
	role := session.Role
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if db != nil {
		var err error
		role, err = db.GetEffectiveRole(ctx, room.ID, session.UID)
		if err != nil {
			_ = session.SendMessage(models.Message{Type: models.MsgTypeError, Data: "Failed to check editing permission"})
			return
		}
	}
	if !CanWrite(role) {
		_ = session.SendMessage(models.Message{Type: models.MsgTypeError, Data: "You have read-only access to this document"})
		return
	}
	var persist func(string, int, []ot.Operation) error
	if db != nil {
		persist = func(content string, rev int, ops []ot.Operation) error {
			return db.SaveRoomRevision(ctx, room.ID, content, rev, session.ClientID, ops)
		}
	}
	newRev, ops, err := room.DocState.ApplyClientOpsAndPersist(msg.Rev, session.ClientID, msg.Ops, true, persist)
	if err != nil {
		logger.Warn("Document operation rejected", "room_id", room.ID, "error", err)
		content, rev := room.DocState.GetSnapshot()
		_ = session.SendMessage(models.Message{Type: models.MsgTypeSync, Rev: rev, Data: content, Color: "resync_required"})
		return
	}
	_ = session.SendMessage(models.Message{Type: models.MsgTypeAck, Rev: newRev})
	room.Broadcast(models.Message{Type: models.MsgTypeOp, Rev: newRev, ClientID: session.ClientID, Ops: ops}, session.Conn)
}

func (ws *WebSocketService) handleSyncRequest(session *ClientSession, room *RoomManager) {
	room.opMu.Lock()
	defer room.opMu.Unlock()
	// A snapshot keeps revision and content consistent, including empty documents.
	content, rev := room.DocState.GetSnapshot()
	_ = session.SendMessage(models.Message{Type: models.MsgTypeSync, Rev: rev, Data: content})
}

func (ws *WebSocketService) handleLegacyDocumentUpdate(ctx context.Context, session *ClientSession, room *RoomManager, content string, db *DatabaseService) {
	old, rev := room.DocState.GetSnapshot()
	if old == content {
		return
	}
	ws.handleOpMessage(ctx, session, room, models.Message{Rev: rev, Ops: replacementOps(old, content)}, db)
}

func replacementOps(old, content string) []ot.Operation {
	var ops []ot.Operation
	if old != "" {
		ops = append(ops, ot.Operation{Type: ot.OpDelete, Pos: 0, Len: len([]rune(old))})
	}
	if content != "" {
		ops = append(ops, ot.Operation{Type: ot.OpInsert, Pos: 0, Text: content})
	}
	return ops
}

// SaveDocument prevents an HTTP snapshot from overwriting active collaborators.
func (ws *WebSocketService) SaveDocument(ctx context.Context, db *DatabaseService, roomID, content string) error {
	ws.mu.Lock()
	if room := ws.rooms[roomID]; room != nil {
		ws.mu.Unlock()
		room.opMu.Lock()
		defer room.opMu.Unlock()
		current, _ := room.DocState.GetSnapshot()
		if current != content {
			return ErrConflict
		}
		return nil
	}
	defer ws.mu.Unlock()
	stored, err := db.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}
	if stored.Content == content {
		return nil
	}
	return db.SaveRoomRevision(ctx, roomID, content, stored.CurrentRev+1, "http", replacementOps(stored.Content, content))
}

func newRoomManager(id, content string, rev int) *RoomManager {
	return &RoomManager{ID: id, Clients: make(map[*websocket.Conn]*ClientSession), DocState: ot.NewDocumentState(content, rev)}
}

func (ws *WebSocketService) closeConnection(conn *websocket.Conn, roomID string) {
	_ = conn.Close()
	ws.mu.Lock()
	room := ws.rooms[roomID]
	if room == nil {
		ws.mu.Unlock()
		return
	}
	room.mu.Lock()
	delete(room.Clients, conn)
	presence := room.getPresenceListUnsafe()
	if len(room.Clients) == 0 {
		delete(ws.rooms, roomID)
	}
	room.mu.Unlock()
	ws.mu.Unlock()
	room.Broadcast(models.Message{Type: models.MsgTypePresence, Presence: presence}, nil)
}

func (rm *RoomManager) Broadcast(message models.Message, exclude *websocket.Conn) {
	rm.mu.RLock()
	sessions := make([]*ClientSession, 0, len(rm.Clients))
	for conn, session := range rm.Clients {
		if conn != exclude {
			sessions = append(sessions, session)
		}
	}
	rm.mu.RUnlock()
	for _, session := range sessions {
		_ = session.SendMessage(message)
	}
}

func (rm *RoomManager) GetPresenceList() []models.UserPresence {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.getPresenceListUnsafe()
}

func (rm *RoomManager) getPresenceListUnsafe() []models.UserPresence {
	list := make([]models.UserPresence, 0, len(rm.Clients))
	for _, session := range rm.Clients {
		list = append(list, models.UserPresence{
			ClientID: session.ClientID, UID: session.UID, Name: session.Name, Color: session.Color,
			Role: session.Role, Cursor: session.Cursor, ConnectedAt: session.Connected})
	}
	return list
}

func (ws *WebSocketService) GetRoomStats() map[string]int {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	stats := make(map[string]int, len(ws.rooms))
	for id, room := range ws.rooms {
		room.mu.RLock()
		stats[id] = len(room.Clients)
		room.mu.RUnlock()
	}
	return stats
}

func (ws *WebSocketService) Shutdown(ctx context.Context) error {
	ws.mu.Lock()
	ws.stopping = true
	var connections []*websocket.Conn
	for _, room := range ws.rooms {
		room.mu.RLock()
		for conn := range room.Clients {
			connections = append(connections, conn)
		}
		room.mu.RUnlock()
	}
	ws.mu.Unlock()
	closeMessage := websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server is shutting down")
	for _, conn := range connections {
		deadline := time.Now().Add(time.Second)
		if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
			deadline = limit
		}
		_ = conn.WriteControl(websocket.CloseMessage, closeMessage, deadline)
		_ = conn.Close()
	}
	done := make(chan struct{})
	go func() { ws.clients.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CloseRoomConnections applies membership changes immediately; an empty UID closes everyone.
func (ws *WebSocketService) CloseRoomConnections(roomID, userUID string) {
	ws.mu.RLock()
	var connections []*websocket.Conn
	if room := ws.rooms[roomID]; room != nil {
		room.mu.RLock()
		for conn, session := range room.Clients {
			if userUID == "" || session.UID == userUID {
				connections = append(connections, conn)
			}
		}
		room.mu.RUnlock()
	}
	ws.mu.RUnlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}
