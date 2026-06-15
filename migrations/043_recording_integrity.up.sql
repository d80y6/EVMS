ALTER TABLE recordings ADD COLUMN sha256 TEXT;
ALTER TABLE recordings ADD COLUMN last_verified TIMESTAMPTZ;

CREATE TABLE recording_gaps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  camera_id UUID NOT NULL REFERENCES cameras(id),
  expected_start TIMESTAMPTZ NOT NULL,
  actual_start TIMESTAMPTZ NOT NULL,
  gap_seconds INTEGER NOT NULL,
  detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_recording_gaps_camera ON recording_gaps(camera_id);
