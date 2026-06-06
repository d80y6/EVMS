-- Migration: 010_discovery.up.sql

CREATE TABLE IF NOT EXISTS discovery_scans (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    site_id     UUID REFERENCES sites(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'pending',
    methods     TEXT[] NOT NULL,
    subnets     TEXT[],
    ports       INT[] DEFAULT '{80,554,8080}',
    total_found INT DEFAULT 0,
    error       TEXT,
    started_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_by  UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS discovery_results (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scan_id       UUID REFERENCES discovery_scans(id) ON DELETE CASCADE,
    site_id       UUID REFERENCES sites(id) ON DELETE CASCADE,
    ip_address    TEXT NOT NULL,
    port          INT,
    xaddr         TEXT,
    manufacturer  TEXT,
    model         TEXT,
    firmware      TEXT,
    serial_number TEXT,
    hostname      TEXT,
    capabilities  JSONB DEFAULT '{}',
    onvif_data    JSONB,
    is_new        BOOLEAN DEFAULT TRUE,
    already_in_db BOOLEAN DEFAULT FALSE,
    imported      BOOLEAN DEFAULT FALSE,
    imported_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_discovery_results_scan ON discovery_results(scan_id);
CREATE INDEX IF NOT EXISTS idx_discovery_results_site ON discovery_results(site_id);
CREATE INDEX IF NOT EXISTS idx_discovery_scans_site ON discovery_scans(site_id);
CREATE INDEX IF NOT EXISTS idx_discovery_scans_status ON discovery_scans(status);

ALTER TABLE sites ADD COLUMN IF NOT EXISTS discovery_config JSONB DEFAULT '{}';
