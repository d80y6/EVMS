CREATE TABLE IF NOT EXISTS alert_rules (
    id TEXT PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    object_type TEXT NOT NULL DEFAULT '',
    zone TEXT NOT NULL DEFAULT '',
    min_confidence FLOAT NOT NULL DEFAULT 0.5,
    action TEXT NOT NULL DEFAULT 'alert',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_tenant ON alert_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_camera ON alert_rules(camera_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules(enabled);
