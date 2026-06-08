CREATE TABLE IF NOT EXISTS frame_index (
    id UUID DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    offset_bytes BIGINT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    motion_score FLOAT NOT NULL DEFAULT 0,
    scene_change BOOLEAN NOT NULL DEFAULT false,
    metadata JSONB
);

SELECT create_hypertable('frame_index', 'timestamp', if_not_exists => true);

CREATE INDEX IF NOT EXISTS idx_frame_index_camera ON frame_index(camera_id, timestamp DESC);
