CREATE TABLE export_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  camera_id UUID NOT NULL REFERENCES cameras(id),
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ NOT NULL,
  watermark BOOLEAN NOT NULL DEFAULT false,
  status TEXT NOT NULL DEFAULT 'queued',
  file_path TEXT,
  sha256 TEXT,
  size_bytes BIGINT,
  error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_export_jobs_camera ON export_jobs(camera_id);
CREATE INDEX idx_export_jobs_status ON export_jobs(status);
