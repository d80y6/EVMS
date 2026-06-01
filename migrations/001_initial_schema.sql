-- DAM VMS Initial Schema
-- This migration sets up the core database schema.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "timescaledb";
CREATE EXTENSION IF NOT EXISTS "vector";

-- Tenants
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Users
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID REFERENCES tenants(id),
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Sites
CREATE TABLE IF NOT EXISTS sites (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID REFERENCES tenants(id),
    name TEXT NOT NULL,
    location TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Cameras
CREATE TABLE IF NOT EXISTS cameras (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    site_id UUID REFERENCES sites(id),
    name TEXT NOT NULL,
    description TEXT,
    connection_url TEXT NOT NULL,
    substream_url TEXT,
    onvif_data JSONB,
    status TEXT DEFAULT 'offline',
    config JSONB DEFAULT '{}',
    ptz_protocol TEXT DEFAULT 'NONE',
    retention_days INT DEFAULT 30,
    prerecord_seconds INT DEFAULT 5,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Recordings (TimescaleDB Hypertable)
CREATE TABLE IF NOT EXISTS recordings (
    id UUID DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    file_path TEXT NOT NULL,
    file_size BIGINT,
    segment_type TEXT,
    metadata JSONB
);

SELECT create_hypertable('recordings', 'start_time', if_not_exists => true);

-- AI Events (TimescaleDB Hypertable)
CREATE TABLE IF NOT EXISTS ai_events (
    id UUID DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL,
    event_time TIMESTAMPTZ NOT NULL,
    object_type TEXT,
    confidence FLOAT,
    bounding_box JSONB,
    track_id TEXT,
    thumbnail TEXT,
    embedding vector(512),
    metadata JSONB
);

SELECT create_hypertable('ai_events', 'event_time', if_not_exists => true);

-- Audit Logs
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id),
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id UUID,
    details JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sites_tenant ON sites(tenant_id);
CREATE INDEX IF NOT EXISTS idx_cameras_site ON cameras(site_id);
CREATE INDEX IF NOT EXISTS idx_cameras_status ON cameras(status);
CREATE INDEX IF NOT EXISTS idx_recordings_camera ON recordings(camera_id);
CREATE INDEX IF NOT EXISTS idx_recordings_time ON recordings(start_time DESC);
CREATE INDEX IF NOT EXISTS idx_ai_events_camera ON ai_events(camera_id);
CREATE INDEX IF NOT EXISTS idx_ai_events_time ON ai_events(event_time DESC);
CREATE INDEX IF NOT EXISTS idx_ai_events_object ON ai_events(object_type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_time ON audit_logs(created_at DESC);

-- Bookmarks
CREATE TABLE IF NOT EXISTS bookmarks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id),
    timestamp TIMESTAMPTZ NOT NULL,
    label TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    created_by TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_bookmarks_camera ON bookmarks(camera_id);
CREATE INDEX IF NOT EXISTS idx_bookmarks_time ON bookmarks(timestamp DESC);

-- Legal Holds
CREATE TABLE IF NOT EXISTS legal_holds (
    id UUID PRIMARY KEY,
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_legal_holds_camera ON legal_holds(camera_id);

-- Crowd Heatmaps (TimescaleDB Hypertable)
CREATE TABLE IF NOT EXISTS crowd_heatmaps (
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    cell_x INT NOT NULL,
    cell_y INT NOT NULL,
    bucket TIMESTAMPTZ NOT NULL,
    count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (camera_id, cell_x, cell_y, bucket)
);

SELECT create_hypertable('crowd_heatmaps', 'bucket', if_not_exists => true);
