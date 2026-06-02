CREATE TABLE IF NOT EXISTS face_watchlist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    face_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    notes TEXT,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS face_detections (
    id UUID DEFAULT gen_random_uuid(),
    camera_id UUID NOT NULL,
    event_time TIMESTAMPTZ NOT NULL,
    face_id TEXT,
    name TEXT,
    confidence FLOAT,
    bounding_box JSONB,
    watchlisted BOOLEAN DEFAULT false,
    metadata JSONB
);

SELECT create_hypertable('face_detections', 'event_time', if_not_exists => true);
