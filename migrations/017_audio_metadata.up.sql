CREATE TABLE IF NOT EXISTS audio_metadata (
    id UUID DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    recording_id UUID,
    stream_index INT NOT NULL DEFAULT 0,
    codec TEXT NOT NULL DEFAULT 'aac',
    sample_rate INT NOT NULL DEFAULT 48000,
    channels INT NOT NULL DEFAULT 1,
    has_audio BOOLEAN NOT NULL DEFAULT true,
    rms_level FLOAT NOT NULL DEFAULT 0,
    peak_level FLOAT NOT NULL DEFAULT 0,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB
);

SELECT create_hypertable('audio_metadata', 'timestamp', if_not_exists => true);

CREATE INDEX IF NOT EXISTS idx_audio_metadata_camera ON audio_metadata(camera_id, timestamp DESC);
