DROP INDEX IF EXISTS idx_cameras_deleted_at;
ALTER TABLE cameras DROP COLUMN IF EXISTS deleted_at;
