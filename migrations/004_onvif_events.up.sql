CREATE TABLE IF NOT EXISTS onvif_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id VARCHAR(255) NOT NULL,
    subscription_id VARCHAR(255),
    topic TEXT,
    source TEXT,
    event_type VARCHAR(255),
    severity VARCHAR(50),
    message TEXT,
    raw_xml TEXT,
    event_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_onvif_events_camera_id ON onvif_events(camera_id);
CREATE INDEX IF NOT EXISTS idx_onvif_events_event_type ON onvif_events(event_type);
CREATE INDEX IF NOT EXISTS idx_onvif_events_event_time ON onvif_events(event_time DESC);
