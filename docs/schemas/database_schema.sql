-- DAM VMS Database Schema

-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "timescaledb";
CREATE EXTENSION IF NOT EXISTS "vector";

-- Tenants
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Users
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID REFERENCES tenants(id),
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL, -- 'admin', 'operator', 'viewer'
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Sites
CREATE TABLE sites (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID REFERENCES tenants(id),
    name TEXT NOT NULL,
    location TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Cameras
CREATE TABLE cameras (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    site_id UUID REFERENCES sites(id),
    name TEXT NOT NULL,
    description TEXT,
    connection_url TEXT NOT NULL, -- RTSP URL
    substream_url TEXT,
    onvif_data JSONB,
    status TEXT DEFAULT 'offline', -- 'online', 'offline', 'error'
    config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Recordings (TimescaleDB Hypertable)
CREATE TABLE recordings (
    id UUID DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    file_path TEXT NOT NULL,
    file_size BIGINT,
    segment_type TEXT, -- 'continuous', 'motion', 'event'
    metadata JSONB
);

SELECT create_hypertable('recordings', 'start_time');

-- AI Events (TimescaleDB Hypertable)
CREATE TABLE ai_events (
    id UUID DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL,
    event_time TIMESTAMPTZ NOT NULL,
    object_type TEXT, -- 'person', 'car', 'license_plate', etc.
    confidence FLOAT,
    bounding_box JSONB, -- [x1, y1, x2, y2]
    embedding vector(512), -- For facial/object recognition
    metadata JSONB
);

SELECT create_hypertable('ai_events', 'event_time');

-- Audit Logs
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id),
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id UUID,
    details JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
