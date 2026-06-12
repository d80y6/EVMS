CREATE TABLE IF NOT EXISTS report_configs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('audit', 'events', 'storage', 'health')),
    schedule TEXT NOT NULL DEFAULT 'daily',
    format TEXT NOT NULL DEFAULT 'pdf',
    recipients JSONB NOT NULL DEFAULT '[]',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS report_archive (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    config_id UUID NOT NULL REFERENCES report_configs(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    format TEXT NOT NULL,
    file_path TEXT NOT NULL DEFAULT '',
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
