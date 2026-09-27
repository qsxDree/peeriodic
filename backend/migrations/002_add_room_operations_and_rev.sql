-- Migration: Add current_rev to rooms and create room_operations table for event logging

ALTER TABLE rooms 
ADD COLUMN IF NOT EXISTS current_rev INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS room_operations (
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

CREATE INDEX IF NOT EXISTS idx_room_operations_room_rev ON room_operations(room_id, rev);
