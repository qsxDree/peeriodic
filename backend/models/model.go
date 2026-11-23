package models

import (
	"time"

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
	Rev      int             `json:"rev"`
	Data     string          `json:"data"`
	ClientID string          `json:"client_id,omitempty"`
	Role     string          `json:"role,omitempty"`
	Ops      []ot.Operation  `json:"ops,omitempty"`
	Presence []UserPresence  `json:"presence,omitempty"`
	Cursor   *CursorPosition `json:"cursor,omitempty"`
	Name     string          `json:"name,omitempty"`
	Color    string          `json:"color,omitempty"`
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
	Role       string    `json:"role,omitempty"`
	UserUID    *string   `json:"user_uid,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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
