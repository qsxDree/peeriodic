-- Database setup script for Peeriodic
-- Run this script as a PostgreSQL superuser (like 'postgres')

-- Step 1: Create the database (run this as superuser)
-- Uncomment the line below if the database doesn't exist
-- CREATE DATABASE peeriodic;

-- Step 2: Create a user for the application (run this as superuser)
-- Uncomment and modify the lines below if you want to create a new user
-- CREATE USER logoes WITH PASSWORD 'your_password_here';
-- GRANT ALL PRIVILEGES ON DATABASE peeriodic TO logoes;

-- Step 3: Connect to the peeriodic database and run the following:
-- \c peeriodic;

-- Drop existing tables if they exist
DROP TABLE IF EXISTS rooms CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- Create users table
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    uid VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255),
    password_hash VARCHAR(255) DEFAULT '',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create rooms table with proper schema
CREATE TABLE rooms (
    id VARCHAR(255) PRIMARY KEY,
    title VARCHAR(255) NOT NULL DEFAULT 'Untitled Room',
    content TEXT DEFAULT '',
    current_rev INT NOT NULL DEFAULT 0,
    public_role VARCHAR(32) NOT NULL DEFAULT 'editor',
    user_uid VARCHAR(255) REFERENCES users(uid) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create room_operations table for event logging and OT history
CREATE TABLE room_operations (
    id BIGSERIAL PRIMARY KEY,
    room_id VARCHAR(255) NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    rev INT NOT NULL,
    client_id VARCHAR(255) NOT NULL,
    op_type VARCHAR(16) NOT NULL,
    pos INT NOT NULL,
    text TEXT,
    length INT,
    created_at TIMESTAMP DEFAULT NOW(),
    CONSTRAINT unique_room_rev UNIQUE(room_id, rev)
);

-- Create document_members table for per-document roles (owner, editor, viewer)
CREATE TABLE document_members (
    id SERIAL PRIMARY KEY,
    room_id VARCHAR(255) NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_uid VARCHAR(255) NOT NULL REFERENCES users(uid) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL DEFAULT 'editor',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    CONSTRAINT unique_room_user UNIQUE(room_id, user_uid)
);

-- Create room_invites table for invite tokens
CREATE TABLE room_invites (
    id SERIAL PRIMARY KEY,
    room_id VARCHAR(255) NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    token VARCHAR(255) UNIQUE NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'editor',
    created_by VARCHAR(255) REFERENCES users(uid) ON DELETE SET NULL,
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Create indexes for better performance
CREATE INDEX idx_rooms_user_uid ON rooms(user_uid);
CREATE INDEX idx_rooms_updated_at ON rooms(updated_at);
CREATE INDEX idx_room_operations_room_rev ON room_operations(room_id, rev);
CREATE INDEX idx_doc_members_room ON document_members(room_id);
CREATE INDEX idx_doc_members_user ON document_members(user_uid);
CREATE INDEX idx_room_invites_token ON room_invites(token);
CREATE INDEX idx_room_invites_room ON room_invites(room_id);
CREATE INDEX idx_users_uid ON users(uid);
CREATE INDEX idx_users_email ON users(email);

-- Optional: Create a function to automatically update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers to automatically update updated_at
DROP TRIGGER IF EXISTS update_rooms_updated_at ON rooms;
CREATE TRIGGER update_rooms_updated_at 
    BEFORE UPDATE ON rooms 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_users_updated_at ON users;
CREATE TRIGGER update_users_updated_at 
    BEFORE UPDATE ON users 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Insert test data
INSERT INTO users (uid, email, name) 
VALUES ('test-user', 'test@example.com', 'Test User')
ON CONFLICT (uid) DO NOTHING;

INSERT INTO rooms (id, title, content, user_uid) 
VALUES ('test-room-123', 'Test Room', 'Hello World!', 'test-user')
ON CONFLICT (id) DO NOTHING;

