ALTER TABLE cameras ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_cameras_deleted_at ON cameras(deleted_at) WHERE deleted_at IS NOT NULL;
