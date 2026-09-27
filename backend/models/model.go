package models

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/logoes0/peeriodic.git/ot"
)

// Message types for WebSocket protocol
const (
	MsgTypeInit     = "init"
	MsgTypeOp       = "op"
	MsgTypeAck      = "ack"
	MsgTypeCursor   = "cursor"
	MsgTypePresence = "presence"
	MsgTypeSync     = "sync"
	MsgTypePing     = "ping"
	MsgTypePong     = "pong"
	MsgTypeError    = "error"
	// Backwards compatibility
	MsgTypeUpdate = "update"
)

// CursorPosition represents a user's cursor and text selection
type CursorPosition struct {
	Pos      int `json:"pos"`
	SelStart int `json:"sel_start"`
	SelEnd   int `json:"sel_end"`
}

// UserPresence represents a connected participant in a room
type UserPresence struct {
	ClientID    string          `json:"client_id"`
	UID         string          `json:"uid,omitempty"`
	Name        string          `json:"name"`
	Color       string          `json:"color"`
	Role        string          `json:"role,omitempty"`
	Cursor      *CursorPosition `json:"cursor,omitempty"`
	ConnectedAt time.Time       `json:"connected_at"`
}

// Message represents a WebSocket message in the collaboration protocol
type Message struct {
	Type     string          `json:"type"`
	Rev      int             `json:"rev,omitempty"`
	Data     string          `json:"data,omitempty"`
	ClientID string          `json:"client_id,omitempty"`
	Role     string          `json:"role,omitempty"`
	Ops      []ot.Operation  `json:"ops,omitempty"`
	Presence []UserPresence  `json:"presence,omitempty"`
	Cursor   *CursorPosition `json:"cursor,omitempty"`
	Name     string          `json:"name,omitempty"`
	Color    string          `json:"color,omitempty"`
}

// OperationRecord represents a persisted operation log entry in PostgreSQL
type OperationRecord struct {
	ID        int64     `json:"id"`
	RoomID    string    `json:"room_id"`
	Rev       int       `json:"rev"`
	ClientID  string    `json:"client_id"`
	OpType    string    `json:"op_type"`
	Pos       int       `json:"pos"`
	Text      string    `json:"text,omitempty"`
	Len       int       `json:"len,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// User represents a user in the system
type User struct {
	ID           int       `json:"id"`
	UID          string    `json:"uid"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DocumentMember represents a user's membership and role in a room
type DocumentMember struct {
	ID        int       `json:"id"`
	RoomID    string    `json:"room_id"`
	UserUID   string    `json:"user_uid"`
	Role      string    `json:"role"`
	UserName  string    `json:"user_name,omitempty"`
	UserEmail string    `json:"user_email,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RoomInvite represents an invite token to join a room with a specific role
type RoomInvite struct {
	ID        int        `json:"id"`
	RoomID    string     `json:"room_id"`
	Token     string     `json:"token"`
	Role      string     `json:"role"`
	CreatedBy *string    `json:"created_by,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Room represents a collaborative editing room
type Room struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	CurrentRev int       `json:"current_rev"`
	PublicRole string    `json:"public_role"`
	UserUID    *string   `json:"user_uid,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Clients    map[*websocket.Conn]bool
	Mu         sync.RWMutex
}

// NewRoom creates a new room instance
func NewRoom(id, title string, userUID *string) *Room {
	return &Room{
		ID:         id,
		Title:      title,
		UserUID:    userUID,
		CurrentRev: 0,
		PublicRole: "editor",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Clients:    make(map[*websocket.Conn]bool),
	}
}

// AddClient adds a client to the room
func (r *Room) AddClient(conn *websocket.Conn) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Clients[conn] = true
}

// RemoveClient removes a client from the room
func (r *Room) RemoveClient(conn *websocket.Conn) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	delete(r.Clients, conn)
}

// GetClientCount returns the number of clients in the room
func (r *Room) GetClientCount() int {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	return len(r.Clients)
}

// Broadcast sends a message to all clients in the room except the sender
func (r *Room) Broadcast(message Message, sender *websocket.Conn) {
	r.Mu.RLock()
	defer r.Mu.RUnlock()

	for client := range r.Clients {
		if client != sender {
			if err := client.WriteJSON(message); err != nil {
				client.Close()
				delete(r.Clients, client)
			}
		}
	}
}

// UpdateContent updates the room's content and timestamp
func (r *Room) UpdateContent(content string) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Content = content
	r.UpdatedAt = time.Now()
}

// Auth DTOs
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type CreateInviteRequest struct {
	Role           string `json:"role"`
	ExpiresInHours int    `json:"expires_in_hours,omitempty"`
}

type UpdateMemberRoleRequest struct {
	Role string `json:"role"`
}
