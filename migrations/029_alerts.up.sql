CREATE TABLE IF NOT EXISTS alerts (
    id TEXT PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    rule_id TEXT NOT NULL DEFAULT '',
    camera_id TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'triggered' CHECK (status IN ('triggered', 'acknowledged', 'escalated', 'resolved')),
    acked_by TEXT NOT NULL DEFAULT '',
    acked_at TIMESTAMPTZ,
    escalated BOOLEAN NOT NULL DEFAULT false,
    escalation_webhook TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alerts_tenant ON alerts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_camera ON alerts(camera_id);
