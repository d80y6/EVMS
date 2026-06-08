CREATE TABLE IF NOT EXISTS intrusion_zones (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    polygon JSONB NOT NULL,
    direction TEXT NOT NULL DEFAULT 'any',
    sensitivity FLOAT NOT NULL DEFAULT 0.8,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS intrusion_events (
    id UUID DEFAULT uuid_generate_v4(),
    zone_id UUID NOT NULL REFERENCES intrusion_zones(id) ON DELETE CASCADE,
    camera_id UUID NOT NULL,
    direction TEXT NOT NULL,
    confidence FLOAT NOT NULL DEFAULT 1.0,
    event_time TIMESTAMPTZ NOT NULL,
    track_id TEXT
);

SELECT create_hypertable('intrusion_events', 'event_time', if_not_exists => true);

CREATE INDEX IF NOT EXISTS idx_intrusion_events_zone ON intrusion_events(zone_id);
CREATE INDEX IF NOT EXISTS idx_intrusion_events_camera ON intrusion_events(camera_id);
CREATE INDEX IF NOT EXISTS idx_intrusion_events_time ON intrusion_events(event_time DESC);
