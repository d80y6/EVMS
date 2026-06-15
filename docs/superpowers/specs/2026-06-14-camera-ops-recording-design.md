# Camera Operations & Recording System — Production Hardening

**Date:** 2026-06-14
**Status:** Design Approved

## Overview

Six-phase sequential hardening of EVMS camera operations and recording subsystems, following the Data-first approach: establish data integrity at camera boundaries, then harden operations, then enhance recording subsystem.

---

## Phase 1: Camera Input Validation

**Location:** `services/camera-mgmt/main.go` — CreateCamera, UpdateCamera handlers

### Validation Rules

| Field | Rule | Error |
|-------|------|-------|
| `name` | Non-empty, max 255 chars | `INVALID_NAME` |
| `site_id` | Must be valid UUID, site must exist in tenant | `INVALID_SITE` |
| `connection_url` | Must be valid URL (rtsp://, http://, https://) | `INVALID_URL` |
| `substream_url` | If set, must be valid URL | `INVALID_SUBSTREAM_URL` |
| `ptz_protocol` | Must be one of: `NONE`, `onvif`, `vapix`, `hikvision` | `INVALID_PTZ_PROTOCOL` |
| `retention_days` | Must be 1–3650 | `INVALID_RETENTION` |
| `prerecord_seconds` | Must be 0–30 | `INVALID_PRERECORD` |
| Duplicate URL | No other camera with same `connection_url` in same tenant | `DUPLICATE_URL` |
| ONVIF credentials | Both username + password required if either set | `INCOMPLETE_ONVIF_CREDS` |

### Implementation

- Single `validateCamera(req *CreateCameraRequest, existing ...*Camera) *status.Error` function
- Returns gRPC `InvalidArgument` with field-level details
- Called before DB insert/update — fail-fast
- Duplicate URL check queries `SELECT COUNT(*) FROM cameras WHERE connection_url = $1 AND site_id IN (SELECT id FROM sites WHERE tenant_id = $2) AND deleted_at IS NULL`

### Testing

- Unit test for each validation rule
- Test edge cases: empty fields, malformed URLs, boundary values, missing ONVIF partner field

---

## Phase 2: Soft-delete & Cascade Validation

**Location:** `services/camera-mgmt/main.go` + migration 041

### Database Changes (Migration 041)

```sql
ALTER TABLE cameras ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_cameras_deleted_at ON cameras(deleted_at) WHERE deleted_at IS NOT NULL;
```

### Soft-delete Behavior

- `DeleteCamera`: `UPDATE cameras SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
- `ListCameras`: `WHERE deleted_at IS NULL`
- `GetCamera`: `WHERE deleted_at IS NULL`
- `UpdateCamera`: `WHERE deleted_at IS NULL`
- `StreamStatus`: Return `"deleted"` for soft-deleted cameras
- Optional hard-delete: `?hard=true` query parameter triggers `DELETE FROM cameras WHERE id = $1 AND deleted_at IS NOT NULL`

### Background Cleanup

- New goroutine in `main.go`: every 24h, `DELETE FROM recordings WHERE camera_id IN (SELECT id FROM cameras WHERE deleted_at < NOW() - INTERVAL '30 days')`, then hard-delete those cameras

### Cascade Validation

- `DeleteCamera` (hard mode): count recordings `SELECT COUNT(*) FROM recordings WHERE camera_id = $1`; if > 1000, require `?force=true` parameter
- `DeleteSite`: `SELECT COUNT(*) FROM cameras WHERE site_id = $1 AND deleted_at IS NULL`; if > 0, return `FAILED_PRECONDITION` with count

---

## Phase 3: PTZ Rate Limiting

**Location:** `services/camera-control/main.go` — new rate limiter module

### Rate Limiter

Per-camera in-memory token bucket using `golang.org/x/time/rate`:

| Setting | Env Var | Default |
|---------|---------|---------|
| Max commands/sec per camera | `PTZ_RATE_LIMIT` | 5 |
| Min interval (ms) | `PTZ_COOLDOWN_MS` | 200 |
| Max concurrent commands per camera | `PTZ_CONCURRENCY` | 2 |

### Implementation

```go
type ptzRateLimiter struct {
    mu          sync.Mutex
    limiters    map[string]*rate.Limiter   // camera_id -> limiter
    lastCmd     map[string]time.Time       // camera_id -> last command time
    semaphores  map[string]chan struct{}   // camera_id -> concurrency semaphore
}
```

- `acquire(cameraID string) error`: checks rate limit, cooldown, and acquires semaphore slot
- `release(cameraID string)`: releases semaphore slot
- Returns HTTP 429 with `Retry-After` header on rate limit
- Configurable via env vars; hot-reload not required

### PTZ Handler Integration

In `handlePTZRouter` router, wrap PTZ actions:
```go
func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
    camID := chi.URLParam(r, "id")
    if err := s.ptzLimiter.acquire(camID); err != nil {
        http.Error(w, `{"error":"rate_limit","message":"too many PTZ requests"}`, 429)
        return
    }
    defer s.ptzLimiter.release(camID)
    // ... existing PTZ logic
}
```

### Config Validation

Validate env vars at startup: rate > 0, cooldown >= 50ms, concurrency >= 1.

---

## Phase 4: Multi-probe Health Checks

**Location:** `services/camera-mgmt/health.go` + `services/camera-mgmt/main.go`

### Probe Functions

| Probe | Function | Timeout | On Failure |
|-------|----------|---------|------------|
| TCP dial | `net.DialTimeout("tcp", host:port, 3s)` | 3s | `offline` |
| RTSP DESCRIBE | `fmt.Fprintf(conn, "DESCRIBE %s RTSP/1.0\r\n...")` | 5s | `degraded` |
| ONVIF DeviceInfo | `onvif.GetDeviceInformation()` via SOAP | 5s | `degraded` |

### Health States

| State | Condition |
|-------|-----------|
| `online` | All three probes pass |
| `degraded` | TCP passes, RTSP or ONVIF fails |
| `offline` | TCP fails |

### Database Changes (Migration 042)

```sql
ALTER TABLE cameras ADD COLUMN last_seen_online TIMESTAMPTZ;
ALTER TABLE cameras ADD COLUMN last_status_change TIMESTAMPTZ;
```

### Health Loop Changes

- Current loop at `main.go:644` modified to call `checkCameraHealth(ctx, db, camera)`
- Per-tenant scoping: query `SELECT c.* FROM cameras c JOIN sites s ON c.site_id = s.id WHERE c.deleted_at IS NULL` (tenant check already in tenant-scoped queries)
- Shard by tenant: group cameras by `s.tenant_id`, process one tenant at a time to avoid thundering herd
- Only probe cameras with `connection_url` not null and no `deleted_at`

### Config

| Env Var | Default | Description |
|---------|---------|-------------|
| `HEALTH_CHECK_INTERVAL` | 30s | How often to run health loop |
| `TCP_PROBE_TIMEOUT` | 3s | TCP dial timeout |
| `RTSP_PROBE_TIMEOUT` | 5s | RTSP DESCRIBE timeout |
| `ONVIF_PROBE_TIMEOUT` | 5s | ONVIF GetDeviceInformation timeout |

---

## Phase 5: Recording Integrity

**Location:** `services/recorder/integrity.go`

### Gap Detection

- New background worker `GapDetector` running every 15 min
- Per camera (only cameras with recordings in last 24h):
  ```sql
  SELECT start_time, end_time FROM recordings
  WHERE camera_id = $1 AND start_time > NOW() - INTERVAL '24 hours'
  ORDER BY start_time
  ```
- Walk results; if `next.start_time - prev.end_time > 65s` → gap
- Log: `level=ERROR msg="recording gap detected" camera_id=... expected_start=... actual_start=... gap_seconds=...`
- Prometheus counter: `recording_gaps_total{camera_id="..."}`

### Database Changes (Migration 043)

```sql
ALTER TABLE recordings ADD COLUMN sha256 TEXT;
CREATE TABLE recording_gaps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  camera_id UUID NOT NULL REFERENCES cameras(id),
  expected_start TIMESTAMPTZ NOT NULL,
  actual_start TIMESTAMPTZ NOT NULL,
  gap_seconds INTEGER NOT NULL,
  detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### File Checksums at Ingest

In `processRecordingEvent` after `fixupMoovAtom`:
```go
sha256, err := computeSHA256(filePath)
if err != nil {
    slog.Warn("failed to compute sha256", "path", filePath, "error", err)
    // non-fatal — log and continue
}
```
Then pass `sha256` to `IndexSegment` which stores it in `recordings.sha256`.

### Periodic Integrity Verification

- New background worker `IntegrityVerifier` running every 24h
- Query recordings that haven't been verified in 7 days:
  ```sql
  SELECT id, file_path, sha256 FROM recordings
  WHERE sha256 IS NOT NULL
    AND (last_verified IS NULL OR last_verified < NOW() - INTERVAL '7 days')
  ORDER BY random() LIMIT (SELECT COUNT(*) * 0.05 FROM recordings WHERE sha256 IS NOT NULL)
  ```
- Sample 5% of such files per run
- Re-compute SHA256, compare to stored value
- Mismatch → log ERROR + Prometheus alert metric `recording_integrity_mismatch`
- Update `last_verified` timestamp

```sql
-- migration 043 (continued)
ALTER TABLE recordings ADD COLUMN last_verified TIMESTAMPTZ;
```

---

## Phase 6: Export Queue (NATS-based Async)

**Location:** `services/export/main.go` + `services/export/queue.go`

### Architecture

```
Client → POST /export → 202 {job_id, status: "queued"}
                         → NATS JetStream "export.jobs"
                           → Export worker (queue "export")
                             → ffmpeg concat + watermark
                             → stores result
                             → GET /export/{id}/status → {status, result}
```

### Job States

`queued` → `processing` → `completed` / `failed`

### Database Changes (Migration 044)

```sql
CREATE TABLE export_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  camera_id UUID NOT NULL REFERENCES cameras(id),
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ NOT NULL,
  watermark TEXT,
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
```

### API Changes

**POST /export** (modified):
- Same auth and request validation
- Instead of blocking ffmpeg: insert `export_jobs` row, publish to NATS, return 202
- Response: `{"job_id": "...", "status": "queued"}`

**GET /export/{id}/status** (new):
- Query `export_jobs` by id
- Return: `{"job_id": "...", "status": "completed", "file_path": "...", "sha256": "...", "size_bytes": 1234}`
- Or: `{"job_id": "...", "status": "failed", "error": "ffmpeg exited with code 1"}`

**GET /export/{id}/download** (new):
- If status=completed, serve file
- If status≠completed, return 409 Conflict

### Worker

- Same process or separate consumer
- Subscribe to `export.jobs` with queue group `export`
- On message: set status=processing, run existing ffmpeg logic, set status=completed/failed
- Use JetStream for at-least-once delivery (ack on completion)

### Frontend Polling

- After POST /export returns 202, poll GET /export/{id}/status every 2s
- Show progress indicator in UI
- On completion: show download link
- On failure: show error message

---

## Migration Order

| Migration | Phase | Contents |
|-----------|-------|----------|
| 041 | P2 | `cameras.deleted_at`, index |
| 042 | P4 | `cameras.last_seen_online`, `cameras.last_status_change` |
| 043 | P5 | `recordings.sha256`, `recordings.last_verified`, `recording_gaps` table |
| 044 | P6 | `export_jobs` table |

## Rollback

Each migration has a down.sql. Feature flags not required — each phase is opt-in via env var defaults that enable the new behavior.
