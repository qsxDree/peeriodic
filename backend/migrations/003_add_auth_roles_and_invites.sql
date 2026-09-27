-- Migration: Add authentication, per-document roles, and invite links

-- 1. Add password_hash to users table
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS password_hash VARCHAR(255) DEFAULT '';

-- 2. Add public_role to rooms (default access level for anyone with the room link)
ALTER TABLE rooms 
ADD COLUMN IF NOT EXISTS public_role VARCHAR(32) NOT NULL DEFAULT 'editor';

-- 3. Create document_members table for per-document roles (owner, editor, viewer)
CREATE TABLE IF NOT EXISTS document_members (
    id SERIAL PRIMARY KEY,
    room_id VARCHAR(255) NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_uid VARCHAR(255) NOT NULL REFERENCES users(uid) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL DEFAULT 'editor',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    CONSTRAINT unique_room_user UNIQUE(room_id, user_uid)
);

CREATE INDEX IF NOT EXISTS idx_doc_members_room ON document_members(room_id);
CREATE INDEX IF NOT EXISTS idx_doc_members_user ON document_members(user_uid);

-- 4. Create room_invites table for invite tokens
CREATE TABLE IF NOT EXISTS room_invites (
    id SERIAL PRIMARY KEY,
    room_id VARCHAR(255) NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    token VARCHAR(255) UNIQUE NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'editor',
    created_by VARCHAR(255) REFERENCES users(uid) ON DELETE SET NULL,
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_room_invites_token ON room_invites(token);
CREATE INDEX IF NOT EXISTS idx_room_invites_room ON room_invites(room_id);
