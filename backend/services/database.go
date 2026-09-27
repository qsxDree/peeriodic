package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/models"
)

// DatabaseService handles all database operations
type DatabaseService struct {
	db *sql.DB
}

// NewDatabaseService creates a new database service instance
func NewDatabaseService(cfg *config.Config) (*DatabaseService, error) {
	db, err := sql.Open("postgres", cfg.GetDatabaseConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("✅ Database connection established successfully")
	return &DatabaseService{db: db}, nil
}

// Close closes the database connection
func (ds *DatabaseService) Close() error {
	return ds.db.Close()
}

// User represents a user in the database
type User struct {
	ID           int       `json:"id"`
	UID          string    `json:"uid"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Room represents a room in the database
type Room struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	CurrentRev int       `json:"current_rev"`
	PublicRole string    `json:"public_role"`
	UserUID    *string   `json:"user_uid"` // Pointer to handle NULL values
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// OperationRecord represents an operation log entry
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

// CreateUser creates a new user in the database without a password (legacy/OAuth)
func (ds *DatabaseService) CreateUser(uid, email, name string) (*User, error) {
	return ds.CreateUserWithPassword(uid, email, name, "")
}

// CreateUserWithPassword creates a new user with a hashed password
func (ds *DatabaseService) CreateUserWithPassword(uid, email, name, passwordHash string) (*User, error) {
	query := `
		INSERT INTO users (uid, email, name, password_hash, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, NOW(), NOW()) 
		RETURNING id, uid, email, name, COALESCE(password_hash, ''), created_at, updated_at
	`

	user := &User{}
	err := ds.db.QueryRow(query, uid, email, name, passwordHash).Scan(
		&user.ID, &user.UID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetUserByUID retrieves a user by UID
func (ds *DatabaseService) GetUserByUID(uid string) (*User, error) {
	query := `
		SELECT id, uid, email, name, COALESCE(password_hash, ''), created_at, updated_at 
		FROM users 
		WHERE uid = $1
	`

	user := &User{}
	err := ds.db.QueryRow(query, uid).Scan(
		&user.ID, &user.UID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", uid)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email (including password_hash for auth)
func (ds *DatabaseService) GetUserByEmail(email string) (*User, error) {
	query := `
		SELECT id, uid, email, name, COALESCE(password_hash, ''), created_at, updated_at 
		FROM users 
		WHERE email = $1
	`

	user := &User{}
	err := ds.db.QueryRow(query, email).Scan(
		&user.ID, &user.UID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", email)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// EnsureUserExists creates a user if it doesn't exist, otherwise returns the existing user
func (ds *DatabaseService) EnsureUserExists(uid, email, name string) (*User, error) {
	query := `
		INSERT INTO users (uid, email, name, created_at, updated_at) 
		VALUES ($1, $2, $3, NOW(), NOW()) 
		ON CONFLICT (uid) DO UPDATE SET 
			email = EXCLUDED.email,
			name = EXCLUDED.name,
			updated_at = NOW()
		RETURNING id, uid, email, name, COALESCE(password_hash, ''), created_at, updated_at
	`

	user := &User{}
	err := ds.db.QueryRow(query, uid, email, name).Scan(
		&user.ID, &user.UID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure user exists: %w", err)
	}

	return user, nil
}

// CreateRoom creates a new room in the database
func (ds *DatabaseService) CreateRoom(id, title string, userUID *string) (*Room, error) {
	query := `
		INSERT INTO rooms (id, title, user_uid, content, current_rev, public_role, created_at, updated_at) 
		VALUES ($1, $2, $3, '', 0, 'editor', NOW(), NOW()) 
		RETURNING id, title, content, current_rev, COALESCE(public_role, 'editor'), user_uid, created_at, updated_at
	`

	room := &Room{}
	err := ds.db.QueryRow(query, id, title, userUID).Scan(
		&room.ID, &room.Title, &room.Content, &room.CurrentRev, &room.PublicRole, &room.UserUID, &room.CreatedAt, &room.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create room: %w", err)
	}

	// If a user created the room, grant them the 'owner' role in document_members
	if userUID != nil && *userUID != "" {
		_ = ds.AddRoomMember(room.ID, *userUID, RoleOwner)
	}

	return room, nil
}

// GetRoom retrieves a room by ID
func (ds *DatabaseService) GetRoom(id string) (*Room, error) {
	query := `
		SELECT id, title, content, COALESCE(current_rev, 0), COALESCE(public_role, 'editor'), user_uid, created_at, updated_at 
		FROM rooms 
		WHERE id = $1
	`

	room := &Room{}
	err := ds.db.QueryRow(query, id).Scan(
		&room.ID, &room.Title, &room.Content, &room.CurrentRev, &room.PublicRole, &room.UserUID, &room.CreatedAt, &room.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("room not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}

	return room, nil
}

// GetRoomsByUser retrieves all rooms owned by or shared with a specific user
func (ds *DatabaseService) GetRoomsByUser(userUID string) ([]*Room, error) {
	query := `
		SELECT DISTINCT r.id, r.title, r.content, COALESCE(r.current_rev, 0), COALESCE(r.public_role, 'editor'), r.user_uid, r.created_at, r.updated_at 
		FROM rooms r
		LEFT JOIN document_members m ON r.id = m.room_id AND m.user_uid = $1
		WHERE r.user_uid = $1 OR m.user_uid = $1
		ORDER BY r.updated_at DESC
	`

	rows, err := ds.db.Query(query, userUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rooms: %w", err)
	}
	defer rows.Close()

	var rooms []*Room
	for rows.Next() {
		room := &Room{}
		err := rows.Scan(
			&room.ID, &room.Title, &room.Content, &room.CurrentRev, &room.PublicRole, &room.UserUID, &room.CreatedAt, &room.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan room: %w", err)
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

// UpdateRoomContent updates the content of a room
func (ds *DatabaseService) UpdateRoomContent(id, content string) error {
	query := `
		UPDATE rooms 
		SET content = $1, updated_at = NOW() 
		WHERE id = $2
	`

	result, err := ds.db.Exec(query, content, id)
	if err != nil {
		return fmt.Errorf("failed to update room content: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("room not found: %s", id)
	}

	return nil
}

// UpdateRoomContentAndRev updates both content and current_rev of a room
func (ds *DatabaseService) UpdateRoomContentAndRev(id, content string, rev int) error {
	query := `
		UPDATE rooms 
		SET content = $1, current_rev = $2, updated_at = NOW() 
		WHERE id = $3
	`

	result, err := ds.db.Exec(query, content, rev, id)
	if err != nil {
		return fmt.Errorf("failed to update room content and rev: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("room not found: %s", id)
	}

	return nil
}

// UpdateRoomPublicRole updates the public sharing role of a room ('editor', 'viewer', or 'none')
func (ds *DatabaseService) UpdateRoomPublicRole(id, publicRole string) error {
	query := `UPDATE rooms SET public_role = $1, updated_at = NOW() WHERE id = $2`
	result, err := ds.db.Exec(query, publicRole, id)
	if err != nil {
		return fmt.Errorf("failed to update public role: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		return fmt.Errorf("room not found: %s", id)
	}
	return nil
}

// SaveOperation saves a single OT operation into the room_operations audit/resync log
func (ds *DatabaseService) SaveOperation(roomID string, rev int, clientID, opType string, pos int, text string, length int) error {
	query := `
		INSERT INTO room_operations (room_id, rev, client_id, op_type, pos, text, length, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (room_id, rev) DO NOTHING
	`

	_, err := ds.db.Exec(query, roomID, rev, clientID, opType, pos, text, length)
	if err != nil {
		return fmt.Errorf("failed to save operation: %w", err)
	}
	return nil
}

// GetOperationsSince retrieves operation log records after sinceRev
func (ds *DatabaseService) GetOperationsSince(roomID string, sinceRev int) ([]*OperationRecord, error) {
	query := `
		SELECT id, room_id, rev, client_id, op_type, pos, COALESCE(text, ''), COALESCE(length, 0), created_at
		FROM room_operations
		WHERE room_id = $1 AND rev > $2
		ORDER BY rev ASC
	`

	rows, err := ds.db.Query(query, roomID, sinceRev)
	if err != nil {
		return nil, fmt.Errorf("failed to get operations: %w", err)
	}
	defer rows.Close()

	var records []*OperationRecord
	for rows.Next() {
		rec := &OperationRecord{}
		if err := rows.Scan(&rec.ID, &rec.RoomID, &rec.Rev, &rec.ClientID, &rec.OpType, &rec.Pos, &rec.Text, &rec.Len, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan operation record: %w", err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// DeleteRoom deletes a room by ID
func (ds *DatabaseService) DeleteRoom(id string) error {
	query := `DELETE FROM rooms WHERE id = $1`

	result, err := ds.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete room: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("room not found: %s", id)
	}

	return nil
}

// EnsureRoomExists creates a room if it doesn't exist, otherwise returns the existing room
func (ds *DatabaseService) EnsureRoomExists(id string) (*Room, error) {
	query := `
		INSERT INTO rooms (id, title, content, current_rev, public_role, user_uid, created_at, updated_at) 
		VALUES ($1, 'Untitled Room', '', 0, 'editor', NULL, NOW(), NOW()) 
		ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id 
		RETURNING id, title, content, current_rev, COALESCE(public_role, 'editor'), user_uid, created_at, updated_at
	`

	room := &Room{}
	err := ds.db.QueryRow(query, id).Scan(
		&room.ID, &room.Title, &room.Content, &room.CurrentRev, &room.PublicRole, &room.UserUID, &room.CreatedAt, &room.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure room exists: %w", err)
	}

	return room, nil
}

// --- Access Control & Member Methods ---

// AddRoomMember adds or updates a user's role in a room
func (ds *DatabaseService) AddRoomMember(roomID, userUID, role string) error {
	query := `
		INSERT INTO document_members (room_id, user_uid, role, created_at, updated_at) 
		VALUES ($1, $2, $3, NOW(), NOW()) 
		ON CONFLICT (room_id, user_uid) DO UPDATE SET 
			role = EXCLUDED.role, 
			updated_at = NOW()
	`
	_, err := ds.db.Exec(query, roomID, userUID, role)
	if err != nil {
		return fmt.Errorf("failed to add room member: %w", err)
	}
	return nil
}

// GetRoomMember gets a member's role and user info
func (ds *DatabaseService) GetRoomMember(roomID, userUID string) (*models.DocumentMember, error) {
	query := `
		SELECT m.id, m.room_id, m.user_uid, m.role, COALESCE(u.name, ''), COALESCE(u.email, ''), m.created_at, m.updated_at
		FROM document_members m
		LEFT JOIN users u ON m.user_uid = u.uid
		WHERE m.room_id = $1 AND m.user_uid = $2
	`
	member := &models.DocumentMember{}
	err := ds.db.QueryRow(query, roomID, userUID).Scan(
		&member.ID, &member.RoomID, &member.UserUID, &member.Role, &member.UserName, &member.UserEmail, &member.CreatedAt, &member.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get room member: %w", err)
	}
	return member, nil
}

// GetEffectiveRole calculates the effective role for a user on a given room
func (ds *DatabaseService) GetEffectiveRole(roomID, userUID string) (string, error) {
	room, err := ds.GetRoom(roomID)
	if err != nil {
		return RoleNone, err
	}

	// 1. If user is document owner
	if userUID != "" && room.UserUID != nil && *room.UserUID == userUID {
		return RoleOwner, nil
	}

	// 2. Check explicit member role
	if userUID != "" {
		member, err := ds.GetRoomMember(roomID, userUID)
		if err == nil && member != nil {
			return member.Role, nil
		}
	}

	// 3. Fall back to room's public role
	if room.PublicRole != "" {
		return room.PublicRole, nil
	}

	return RoleEditor, nil
}

// ListRoomMembers returns all members and their roles for a room
func (ds *DatabaseService) ListRoomMembers(roomID string) ([]*models.DocumentMember, error) {
	query := `
		SELECT m.id, m.room_id, m.user_uid, m.role, COALESCE(u.name, ''), COALESCE(u.email, ''), m.created_at, m.updated_at
		FROM document_members m
		LEFT JOIN users u ON m.user_uid = u.uid
		WHERE m.room_id = $1
		ORDER BY m.created_at ASC
	`
	rows, err := ds.db.Query(query, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to list room members: %w", err)
	}
	defer rows.Close()

	var members []*models.DocumentMember
	for rows.Next() {
		m := &models.DocumentMember{}
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserUID, &m.Role, &m.UserName, &m.UserEmail, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan document member: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

// UpdateRoomMemberRole updates a member's role
func (ds *DatabaseService) UpdateRoomMemberRole(roomID, userUID, newRole string) error {
	query := `UPDATE document_members SET role = $1, updated_at = NOW() WHERE room_id = $2 AND user_uid = $3`
	result, err := ds.db.Exec(query, newRole, roomID, userUID)
	if err != nil {
		return fmt.Errorf("failed to update room member role: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		return fmt.Errorf("member not found in room")
	}
	return nil
}

// RemoveRoomMember removes a member from a room
func (ds *DatabaseService) RemoveRoomMember(roomID, userUID string) error {
	query := `DELETE FROM document_members WHERE room_id = $1 AND user_uid = $2`
	result, err := ds.db.Exec(query, roomID, userUID)
	if err != nil {
		return fmt.Errorf("failed to remove room member: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		return fmt.Errorf("member not found in room")
	}
	return nil
}

// --- Invite & Sharing Methods ---

// CreateRoomInvite creates a new invite token for a room
func (ds *DatabaseService) CreateRoomInvite(roomID, token, role string, createdBy *string, expiresAt *time.Time) (*models.RoomInvite, error) {
	query := `
		INSERT INTO room_invites (room_id, token, role, created_by, expires_at, created_at) 
		VALUES ($1, $2, $3, $4, $5, NOW()) 
		RETURNING id, room_id, token, role, created_by, expires_at, created_at
	`
	invite := &models.RoomInvite{}
	err := ds.db.QueryRow(query, roomID, token, role, createdBy, expiresAt).Scan(
		&invite.ID, &invite.RoomID, &invite.Token, &invite.Role, &invite.CreatedBy, &invite.ExpiresAt, &invite.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create room invite: %w", err)
	}
	return invite, nil
}

// GetRoomInvite retrieves an invite by token, checking for expiration
func (ds *DatabaseService) GetRoomInvite(token string) (*models.RoomInvite, error) {
	query := `
		SELECT id, room_id, token, role, created_by, expires_at, created_at 
		FROM room_invites 
		WHERE token = $1
	`
	invite := &models.RoomInvite{}
	err := ds.db.QueryRow(query, token).Scan(
		&invite.ID, &invite.RoomID, &invite.Token, &invite.Role, &invite.CreatedBy, &invite.ExpiresAt, &invite.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invite not found or invalid")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get room invite: %w", err)
	}

	if invite.ExpiresAt != nil && time.Now().After(*invite.ExpiresAt) {
		return nil, fmt.Errorf("invite token has expired")
	}

	return invite, nil
}

// AcceptRoomInvite accepts an invite token, assigns the user the invite role, and returns the room
func (ds *DatabaseService) AcceptRoomInvite(token, userUID string) (*Room, string, error) {
	invite, err := ds.GetRoomInvite(token)
	if err != nil {
		return nil, "", err
	}

	room, err := ds.GetRoom(invite.RoomID)
	if err != nil {
		return nil, "", err
	}

	// Add user to document_members with the role granted by invite
	if err := ds.AddRoomMember(room.ID, userUID, invite.Role); err != nil {
		return nil, "", fmt.Errorf("failed to add member: %w", err)
	}

	return room, invite.Role, nil
}
