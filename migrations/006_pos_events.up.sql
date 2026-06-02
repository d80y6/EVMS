CREATE TABLE IF NOT EXISTS pos_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id UUID NOT NULL REFERENCES cameras(id),
    transaction_id TEXT NOT NULL,
    total NUMERIC(10,2) NOT NULL,
    items JSONB,
    metadata JSONB,
    event_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pos_camera_time ON pos_transactions(camera_id, event_time DESC);
