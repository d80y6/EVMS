CREATE TABLE IF NOT EXISTS abandoned_object_zones (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    polygon JSONB NOT NULL,
    stationary_threshold_seconds INT NOT NULL DEFAULT 60,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS abandoned_object_events (
    id UUID DEFAULT uuid_generate_v4(),
    zone_id UUID NOT NULL REFERENCES abandoned_object_zones(id) ON DELETE CASCADE,
    camera_id UUID NOT NULL,
    object_id TEXT,
    object_class TEXT NOT NULL,
    stationary_seconds FLOAT NOT NULL,
    confidence FLOAT NOT NULL DEFAULT 1.0,
    bounding_box JSONB,
    event_time TIMESTAMPTZ NOT NULL
);

SELECT create_hypertable('abandoned_object_events', 'event_time', if_not_exists => true);

CREATE INDEX IF NOT EXISTS idx_abandoned_object_events_zone ON abandoned_object_events(zone_id);
CREATE INDEX IF NOT EXISTS idx_abandoned_object_events_camera ON abandoned_object_events(camera_id);
CREATE INDEX IF NOT EXISTS idx_abandoned_object_events_time ON abandoned_object_events(event_time DESC);
