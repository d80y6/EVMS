CREATE TABLE IF NOT EXISTS system_config (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL DEFAULT '{}',
    category TEXT NOT NULL DEFAULT 'general' CHECK (category IN ('general', 'retention', 'security', 'notifications', 'storage', 'ai')),
    description TEXT NOT NULL DEFAULT '',
    schema_json JSONB NOT NULL DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS system_config_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key TEXT NOT NULL REFERENCES system_config(key) ON DELETE CASCADE,
    old_value JSONB,
    new_value JSONB,
    changed_by TEXT NOT NULL DEFAULT '',
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_config_category ON system_config(category);
CREATE INDEX IF NOT EXISTS idx_system_config_history_key ON system_config_history(config_key);
CREATE INDEX IF NOT EXISTS idx_system_config_history_at ON system_config_history(changed_at);
