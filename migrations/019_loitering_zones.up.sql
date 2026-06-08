CREATE TABLE IF NOT EXISTS loitering_zones (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    polygon JSONB NOT NULL,
    dwell_threshold_seconds INT NOT NULL DEFAULT 30,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS loitering_events (
    id UUID DEFAULT uuid_generate_v4(),
    zone_id UUID NOT NULL REFERENCES loitering_zones(id) ON DELETE CASCADE,
    camera_id UUID NOT NULL,
    track_id TEXT,
    dwell_seconds FLOAT NOT NULL,
    confidence FLOAT NOT NULL DEFAULT 1.0,
    event_time TIMESTAMPTZ NOT NULL
);

SELECT create_hypertable('loitering_events', 'event_time', if_not_exists => true);

CREATE INDEX IF NOT EXISTS idx_loitering_events_zone ON loitering_events(zone_id);
CREATE INDEX IF NOT EXISTS idx_loitering_events_camera ON loitering_events(camera_id);
CREATE INDEX IF NOT EXISTS idx_loitering_events_time ON loitering_events(event_time DESC);
