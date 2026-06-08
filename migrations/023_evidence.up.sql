CREATE TABLE IF NOT EXISTS evidence_cases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    case_number TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed', 'archived')),
    assigned_to TEXT NOT NULL DEFAULT '',
    tags TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS evidence_lockers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id UUID NOT NULL REFERENCES evidence_cases(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS evidence_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    locker_id UUID NOT NULL REFERENCES evidence_lockers(id) ON DELETE CASCADE,
    recording_id TEXT NOT NULL DEFAULT '',
    camera_id TEXT NOT NULL DEFAULT '',
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    file_path TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    mime_type TEXT NOT NULL DEFAULT 'video/mp4',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS evidence_shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id UUID NOT NULL REFERENCES evidence_items(id) ON DELETE CASCADE,
    share_token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS evidence_access_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id UUID NOT NULL REFERENCES evidence_items(id) ON DELETE CASCADE,
    actor TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL DEFAULT 'view',
    ip_address TEXT NOT NULL DEFAULT '',
    accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_evidence_cases_tenant ON evidence_cases(tenant_id);
CREATE INDEX IF NOT EXISTS idx_evidence_cases_status ON evidence_cases(status);
CREATE INDEX IF NOT EXISTS idx_evidence_lockers_case ON evidence_lockers(case_id);
CREATE INDEX IF NOT EXISTS idx_evidence_items_locker ON evidence_items(locker_id);
CREATE INDEX IF NOT EXISTS idx_evidence_shares_token ON evidence_shares(share_token);
CREATE INDEX IF NOT EXISTS idx_evidence_access_log_item ON evidence_access_log(item_id);
CREATE INDEX IF NOT EXISTS idx_evidence_access_log_at ON evidence_access_log(accessed_at);
