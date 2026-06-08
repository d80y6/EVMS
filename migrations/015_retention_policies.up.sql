CREATE TABLE IF NOT EXISTS retention_policies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    retention_days INT NOT NULL DEFAULT 7,
    archive_enabled BOOLEAN NOT NULL DEFAULT false,
    archive_storage_class TEXT NOT NULL DEFAULT 'WARM',
    motion_retention_days INT NOT NULL DEFAULT 30,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(camera_id)
);

CREATE INDEX IF NOT EXISTS idx_retention_policies_camera ON retention_policies(camera_id);
