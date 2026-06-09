CREATE TABLE IF NOT EXISTS tours (
    id TEXT PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    interval_sec INT NOT NULL DEFAULT 30,
    steps JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tours_tenant ON tours(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tours_enabled ON tours(enabled);
