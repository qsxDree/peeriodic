package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/logoes0/peeriodic.git/logger"
	"time"

	"github.com/lib/pq"
	"github.com/logoes0/peeriodic.git/config"
	"github.com/logoes0/peeriodic.git/models"
	"github.com/logoes0/peeriodic.git/ot"
)

// DatabaseService handles all database operations
type DatabaseService struct {
	db *sql.DB
}

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("document changed; sync before saving")
)

func IsUniqueViolation(err error) bool {
	var pgErr *pq.Error
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("Database connection established")
	return &DatabaseService{db: db}, nil
}

// Close closes the database connection
func (ds *DatabaseService) Close() error {
	if ds == nil || ds.db == nil {
		return nil
	}
	return ds.db.Close()
}

// GetDB returns the underlying *sql.DB instance
func (ds *DatabaseService) GetDB() *sql.DB {
	if ds == nil {
		return nil
	}
	return ds.db
}

// Ping checks if the database is reachable
func (ds *DatabaseService) Ping(ctx context.Context) error {
	if ds == nil || ds.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return ds.db.PingContext(ctx)
}

type User = models.User

type Room = models.Room

func (ds *DatabaseService) EnsureGuestRoom(ctx context.Context, id string) error {
	_, err := ds.db.ExecContext(ctx, `INSERT INTO rooms (id, title, content, current_rev, public_role) VALUES ($1, 'Untitled Room', '', 0, 'editor') ON CONFLICT (id) DO NOTHING`, id)
	return err
}

// CreateUserWithPassword creates a new user with a hashed password
func (ds *DatabaseService) CreateUserWithPassword(ctx context.Context, uid, email, name, passwordHash string) (*User, error) {
	query := `
		INSERT INTO users (uid, email, name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id, uid, email, name, COALESCE(password_hash, ''), created_at, updated_at
	`

	user := &User{}
	err := ds.db.QueryRowContext(ctx, query, uid, email, name, passwordHash).Scan(
		&user.ID, &user.UID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetUserByUID retrieves a user by UID
func (ds *DatabaseService) GetUserByUID(ctx context.Context, uid string) (*User, error) {
	query := `
		SELECT id, uid, email, COALESCE(name, ''), COALESCE(password_hash, ''), created_at, updated_at
		FROM users
		WHERE uid = $1
	`

	user := &User{}
	err := ds.db.QueryRowContext(ctx, query, uid).Scan(
		&user.ID, &user.UID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email (including password_hash for auth)
func (ds *DatabaseService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, uid, email, COALESCE(name, ''), COALESCE(password_hash, ''), created_at, updated_at
		FROM users
		WHERE email = $1
	`

	user := &User{}
	err := ds.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.UID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// CreateRoom creates a new room in the database
func (ds *DatabaseService) CreateRoom(ctx context.Context, id, title string, userUID *string) (*Room, error) {
	tx, err := ds.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	query := `
		INSERT INTO rooms (id, title, user_uid, content, current_rev, public_role, created_at, updated_at)
		VALUES ($1, $2, $3, '', 0, 'editor', NOW(), NOW())
		RETURNING id, title, content, current_rev, COALESCE(public_role, 'editor'), user_uid, created_at, updated_at
	`

	room := &Room{}
	err = tx.QueryRowContext(ctx, query, id, title, userUID).Scan(
		&room.ID, &room.Title, &room.Content, &room.CurrentRev, &room.PublicRole, &room.UserUID, &room.CreatedAt, &room.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create room: %w", err)
	}

	// If a user created the room, grant them the 'owner' role in document_members
	if userUID != nil && *userUID != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO document_members (room_id, user_uid, role) VALUES ($1, $2, 'owner')`, id, *userUID); err != nil {
			return nil, fmt.Errorf("failed to create owner membership: %w", err)
		}
	}
	return room, tx.Commit()
}

// SaveRoomRevision atomically saves the snapshot and every operation in a batch.
func (ds *DatabaseService) SaveRoomRevision(ctx context.Context, id, content string, rev int, clientID string, ops []ot.Operation) error {
	tx, err := ds.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE rooms SET content = $1, current_rev = $2, updated_at = NOW() WHERE id = $3 AND current_rev = $2 - 1`, content, rev, id)
	if err != nil {
		return fmt.Errorf("failed to save document: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	for i, op := range ops {
		if _, err := tx.ExecContext(ctx, `INSERT INTO room_operations (room_id, rev, op_index, client_id, op_type, pos, text, length) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, rev, i, clientID, op.Type, op.Pos, op.Text, op.Len); err != nil {
			return fmt.Errorf("failed to save operation: %w", err)
		}
	}
	return tx.Commit()
}

// GetRoom retrieves a room by ID
func (ds *DatabaseService) GetRoom(ctx context.Context, id string) (*Room, error) {
	query := `
		SELECT id, title, content, COALESCE(current_rev, 0), COALESCE(public_role, 'editor'), user_uid, created_at, updated_at
		FROM rooms
		WHERE id = $1
	`

	room := &Room{}
	err := ds.db.QueryRowContext(ctx, query, id).Scan(
		&room.ID, &room.Title, &room.Content, &room.CurrentRev, &room.PublicRole, &room.UserUID, &room.CreatedAt, &room.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("room not found: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}

	return room, nil
}

// GetRoomsByUser retrieves all rooms owned by or shared with a specific user
func (ds *DatabaseService) GetRoomsByUser(ctx context.Context, userUID string) ([]*Room, error) {
	query := `
		SELECT r.id, r.title, '', COALESCE(r.current_rev, 0), COALESCE(r.public_role, 'editor'), r.user_uid, r.created_at, r.updated_at,
		       CASE WHEN r.user_uid = $1 THEN 'owner' ELSE m.role END
		FROM rooms r
		LEFT JOIN document_members m ON r.id = m.room_id AND m.user_uid = $1
		WHERE r.user_uid = $1 OR m.user_uid = $1
		ORDER BY r.updated_at DESC
	`

	rows, err := ds.db.QueryContext(ctx, query, userUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rooms: %w", err)
	}
	defer rows.Close()

	rooms := make([]*Room, 0)
	for rows.Next() {
		room := &Room{}
		err := rows.Scan(
			&room.ID, &room.Title, &room.Content, &room.CurrentRev, &room.PublicRole, &room.UserUID, &room.CreatedAt, &room.UpdatedAt, &room.Role,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan room: %w", err)
		}
		rooms = append(rooms, room)
	}

	return rooms, rows.Err()
}

// DeleteRoom deletes a room by ID
func (ds *DatabaseService) DeleteRoom(ctx context.Context, id string) error {
	query := `DELETE FROM rooms WHERE id = $1`

	result, err := ds.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete room: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("room not found: %w", ErrNotFound)
	}

	return nil
}

// --- Access Control & Member Methods ---

// AddRoomMember adds or updates a user's role in a room
func (ds *DatabaseService) AddRoomMember(ctx context.Context, roomID, userUID, role string) error {
	query := `
		INSERT INTO document_members (room_id, user_uid, role, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (room_id, user_uid) DO UPDATE SET
			role = CASE WHEN document_members.role = 'owner' THEN 'owner'
			            WHEN document_members.role = 'editor' AND EXCLUDED.role = 'viewer' THEN 'editor'
			            ELSE EXCLUDED.role END,
			updated_at = NOW()
	`
	_, err := ds.db.ExecContext(ctx, query, roomID, userUID, role)
	if err != nil {
		return fmt.Errorf("failed to add room member: %w", err)
	}
	return nil
}

// GetEffectiveRole checks access without loading document contents.
func (ds *DatabaseService) GetEffectiveRole(ctx context.Context, roomID, userUID string) (string, error) {
	var role string
	err := ds.db.QueryRowContext(ctx, `
        SELECT CASE WHEN $2 <> '' AND r.user_uid = $2 THEN 'owner'
                    ELSE COALESCE(m.role, r.public_role, 'none') END
        FROM rooms r LEFT JOIN document_members m ON m.room_id = r.id AND m.user_uid = $2
        WHERE r.id = $1`, roomID, userUID).Scan(&role)
	if err == sql.ErrNoRows {
		return RoleNone, ErrNotFound
	}
	if err != nil {
		return RoleNone, fmt.Errorf("failed to check room access: %w", err)
	}
	return role, nil
}

// ListRoomMembers returns all members and their roles for a room
func (ds *DatabaseService) ListRoomMembers(ctx context.Context, roomID string) ([]*models.DocumentMember, error) {
	query := `
		SELECT m.id, m.room_id, m.user_uid, m.role, COALESCE(u.name, ''), COALESCE(u.email, ''), m.created_at, m.updated_at
		FROM document_members m
		LEFT JOIN users u ON m.user_uid = u.uid
		WHERE m.room_id = $1
		ORDER BY m.created_at ASC
	`
	rows, err := ds.db.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to list room members: %w", err)
	}
	defer rows.Close()

	members := make([]*models.DocumentMember, 0)
	for rows.Next() {
		m := &models.DocumentMember{}
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserUID, &m.Role, &m.UserName, &m.UserEmail, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan document member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// UpdateRoomMemberRole updates a member's role
func (ds *DatabaseService) UpdateRoomMemberRole(ctx context.Context, roomID, userUID, newRole string) error {
	query := `UPDATE document_members SET role = $1, updated_at = NOW() WHERE room_id = $2 AND user_uid = $3`
	result, err := ds.db.ExecContext(ctx, query, newRole, roomID, userUID)
	if err != nil {
		return fmt.Errorf("failed to update room member role: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("member not found in room: %w", ErrNotFound)
	}
	return nil
}

// RemoveRoomMember removes a member from a room
func (ds *DatabaseService) RemoveRoomMember(ctx context.Context, roomID, userUID string) error {
	query := `DELETE FROM document_members WHERE room_id = $1 AND user_uid = $2`
	result, err := ds.db.ExecContext(ctx, query, roomID, userUID)
	if err != nil {
		return fmt.Errorf("failed to remove room member: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("member not found in room: %w", ErrNotFound)
	}
	return nil
}

// --- Invite & Sharing Methods ---

// CreateRoomInvite creates a new invite token for a room
func (ds *DatabaseService) CreateRoomInvite(ctx context.Context, roomID, token, role string, createdBy *string, expiresAt *time.Time) (*models.RoomInvite, error) {
	query := `
		INSERT INTO room_invites (room_id, token, role, created_by, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id, room_id, token, role, created_by, expires_at, created_at
	`
	invite := &models.RoomInvite{}
	err := ds.db.QueryRowContext(ctx, query, roomID, token, role, createdBy, expiresAt).Scan(
		&invite.ID, &invite.RoomID, &invite.Token, &invite.Role, &invite.CreatedBy, &invite.ExpiresAt, &invite.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create room invite: %w", err)
	}
	return invite, nil
}

// GetRoomInvite retrieves an invite by token, checking for expiration
func (ds *DatabaseService) GetRoomInvite(ctx context.Context, token string) (*models.RoomInvite, error) {
	query := `
		SELECT id, room_id, token, role, created_by, expires_at, created_at
		FROM room_invites
		WHERE token = $1
	`
	invite := &models.RoomInvite{}
	err := ds.db.QueryRowContext(ctx, query, token).Scan(
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
func (ds *DatabaseService) AcceptRoomInvite(ctx context.Context, token, userUID string) (*Room, string, error) {
	invite, err := ds.GetRoomInvite(ctx, token)
	if err != nil {
		return nil, "", err
	}

	room, err := ds.GetRoom(ctx, invite.RoomID)
	if err != nil {
		return nil, "", err
	}

	// Add user to document_members with the role granted by invite
	if err := ds.AddRoomMember(ctx, room.ID, userUID, invite.Role); err != nil {
		return nil, "", fmt.Errorf("failed to add member: %w", err)
	}

	role, err := ds.GetEffectiveRole(ctx, room.ID, userUID)
	return room, role, err
}
