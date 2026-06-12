CREATE TABLE IF NOT EXISTS license_activations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    license_key TEXT NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    max_cameras INT NOT NULL DEFAULT 0,
    features JSONB NOT NULL DEFAULT '[]',
    tier TEXT NOT NULL DEFAULT 'trial'
);
