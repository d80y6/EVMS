CREATE TABLE IF NOT EXISTS ai_models (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    version INT NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'inactive' CHECK (status IN ('inactive', 'canary', 'active', 'archived')),
    model_path TEXT NOT NULL DEFAULT '',
    metrics JSONB NOT NULL DEFAULT '{}',
    canary_percent INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(name, version)
);

CREATE INDEX IF NOT EXISTS idx_ai_models_status ON ai_models(status);
