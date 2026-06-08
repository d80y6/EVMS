-- Add composite index for the most common recording query pattern
CREATE INDEX IF NOT EXISTS idx_recordings_camera_time
    ON recordings(camera_id, start_time DESC);

-- Also index end_time for timeline span queries
CREATE INDEX IF NOT EXISTS idx_recordings_camera_end_time
    ON recordings(camera_id, end_time DESC);
