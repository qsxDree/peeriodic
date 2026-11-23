-- Migration: Add users table and update rooms table
-- This migration adds user management functionality to the application

-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    uid VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Add indexes for users table
CREATE INDEX IF NOT EXISTS idx_users_uid ON users(uid);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- Create rooms table if it does not exist yet
CREATE TABLE IF NOT EXISTS rooms (
    id VARCHAR(255) PRIMARY KEY,
    title VARCHAR(255) NOT NULL DEFAULT 'Untitled Room',
    content TEXT DEFAULT '',
    user_uid VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Update rooms table to reference users with a foreign key constraint safely
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_rooms_user_uid'
    ) THEN
        ALTER TABLE rooms 
        ADD CONSTRAINT fk_rooms_user_uid 
        FOREIGN KEY (user_uid) REFERENCES users(uid) ON DELETE SET NULL;
    END IF;
END $$;

-- Update existing rooms to handle NULL user_uid values
UPDATE rooms SET user_uid = NULL WHERE user_uid = '';

-- Create trigger for users table
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create trigger for users table
DROP TRIGGER IF EXISTS update_users_updated_at ON users;
CREATE TRIGGER update_users_updated_at 
    BEFORE UPDATE ON users 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();


