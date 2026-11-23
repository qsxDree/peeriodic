ALTER TABLE room_operations ADD COLUMN IF NOT EXISTS op_index INT NOT NULL DEFAULT 0;
ALTER TABLE room_operations DROP CONSTRAINT IF EXISTS unique_room_rev;
ALTER TABLE room_operations ADD CONSTRAINT unique_room_rev_op UNIQUE (room_id, rev, op_index);
