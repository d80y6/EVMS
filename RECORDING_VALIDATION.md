# Recording System Validation Report
**Date:** 2026-06-11  
**Scope:** Recording Creation, Storage, Retention Enforcement, Timeline/Playback, Export, Storage Accounting, Legal Hold Integration  
**Source Files:** `services/recorder/main.go`, `services/recorder/retention.go`, `services/recorder/legal_hold.go`, `services/playback/main.go`, `services/export/main.go`

---

## 1. Recording Creation and Storage

### 1.1 Architecture Overview
```
Ingest Service (FFmpeg) --> NATS --> Recorder Service --> PostgreSQL
                                  |-> Frame Analysis
                                  |-> Audio Service
                                  |-> Timeline Indexing
```

The recorder subscribes to two NATS subjects:
- `camera.*.recordings.new` - Recording completion events from the ingest pipeline
- `camera.*.h264` - Raw H.264 frame data for pre-recording buffer

### 1.2 Recording Segment Model
| Field | Type | Source |
|-------|------|--------|
| CameraID | UUID | From NATS event |
| StartTime | timestamp | Parsed from filename (`20060102_150405`), fallback to file mtime |
| EndTime | timestamp | Start + 60s, capped at file mtime |
| FilePath | string | Absolute path to .mp4 file |
| FileSize | int64 | From os.Stat |

### 1.3 Ingestion Flow
1. NATS message arrives on `camera.{id}.recordings.new`
2. Shard check: `r.shard.OwnsCamera(cameraID)` - skips if not owned by this shard
3. File stabilization: polls `os.Stat` up to 35 times (70s total), waiting for file size to stabilize across two consecutive reads 2s apart
4. FFmpeg moov atom relocation (`fixupMoovAtom`) for fast streaming start
5. Timestamp extraction from filename
6. Pre-recording buffer flush to `.preroll` companion file
7. Segment indexed into PostgreSQL `recordings` table

### 1.4 Pre-Recording Buffer
- Implementation: Circular byte buffer (ring buffer)
- Capacity: `seconds * bitrate * 1024 / 8` (e.g., 5s x 4096kbps = ~2.5MB)
- Thread-safe: `sync.Mutex` on all operations
- Flushed to `.preroll` file during segment indexing

### 1.5 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **File stabilization polling is blocking** | MEDIUM | `processRecordingEvent` uses sequential polling with `time.After(2s)` inside the NATS callback. This blocks the NATS subscription goroutine for up to 70 seconds per segment. With many cameras, this can backlog messages. |
| **No recording file integrity verification** | MEDIUM | After moov atom fixup, there is no `ffmpeg -v error` validation that the MP4 is playable. The `fixupMoovAtom` function uses `ffmpeg -c copy` which can silently produce corrupt output if the input is malformed. |
| **Pre-record buffer in memory only** | LOW | The ring buffer exists entirely in process memory. If the recorder crashes, pre-record data is lost. No persistence or recovery mechanism. |
| **Timestamp extraction fragile** | LOW | Timestamp parsing from filename uses `time.Parse("20060102_150405", ...)`. If the filename format changes (e.g., different FFmpeg strftime config), timestamps silently fall back to file mtime with no warning to operators. |
| **End time derivation may be inaccurate** | LOW | End time is calculated as `startTime + 60s`, capped at file mtime. For segments shorter than 60s (e.g., last segment before stop), mtime may reflect completion time, but for normal segments, the 60-second assumption may not match actual segment duration. |
| **No gap detection** | MEDIUM | There is no mechanism to detect or alert on recording gaps (e.g., a camera that stops producing segments). Missing consecutive segments are not flagged. |

---

## 2. Retention Enforcement

### 2.1 Retention Architecture
```
RetentionPolicyManager (in-memory cache)
        |
        v
Recorder.runRetentionCleanup()  <-- Ticker every 1 hour
Recorder.runRetentionCleanupWithPolicies()  <-- Per-camera policies
Recorder.StartRetentionPolicyWorker()  <-- Refreshes cache every 1 hour
```

### 2.2 Retention Layers
| Layer | Configuration | Scope |
|-------|---------------|-------|
| Global default | `RecorderConfig.RetentionDays` (default 7) | All cameras without a policy |
| Per-camera policy | `PerCameraRetentionPolicy.RetentionDays` | Specific camera override |
| Motion retention | `PerCameraRetentionPolicy.MotionRetentionDays` | Motion-triggered recordings |
| Archive tiering | `TierConfig` (Hot=7d, Warm=30d) | Storage tier transitions |

### 2.3 Retention Cleanup Logic
1. Query segments older than global cutoff: `SELECT ... WHERE start_time < $1`
2. For each segment:
   a. Skip if camera is on legal hold
   b. Check per-camera policy effective retention (from cached policy or global default)
   c. Skip if within per-camera policy cutoff
   d. Delete file from disk
   e. Delete `.preroll` companion file
   f. Delete DB record

### 2.4 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **Basic cleanup query uses global cutoff only** | MEDIUM | The initial query selects all segments older than the **global** default retention (7 days). Per-camera policies with longer retention are applied as a secondary filter, but the base query always uses the global cutoff. For cameras with retention > global default, expired segments are still loaded into memory and filtered programmatically. |
| **Retention policy cache may be stale** | MEDIUM | The policy cache is refreshed on a fixed 1-hour interval. If a policy is updated, the old retention window applies for up to 1 hour. This means segments could be deleted prematurely or retained too long during the window. |
| **Legal hold cache similarly stale** | MEDIUM | Legal hold cache refreshes only on `ImportLegacyHolds` (startup) and `ReleaseHold`. There is no periodic refresh. If a legal hold is created externally, it won't be recognized until the next cache rebuild. |
| **No storage quota enforcement** | MEDIUM | Retention is based solely on time. There is no storage capacity quota, no percentage-used enforcement, and no mechanism to prioritize retention when storage is full. |
| **No retention for deleted cameras** | MEDIUM | When a camera is deleted (from `camera-mgmt`), there is no cascade to update retention policies or cleanup its recordings. The recording service continues to retain and attempt cleanup of orphaned recordings. |
| **Archive tiering lacks verification** | LOW | The tiering manager moves files between hot/warm/cold paths but there is no verification that files were successfully copied before deletion from the source tier. |
| **No retention audit log** | LOW | Deletion events are logged but not persisted. There is no record of what was deleted, when, or by which retention policy. |

---

## 3. Timeline and Playback

### 3.1 Playback Service
```
Client -> API Gateway -> Playback Service (HTTP) -> Filesystem
```

The playback service serves recorded video files directly from the filesystem with path traversal protection.

### 3.2 Path Traversal Protection
- `filepath.Clean()` on request path
- `filepath.EvalSymlinks()` to resolve symlinks
- `filepath.Rel()` check: must not start with ".." or be absolute
- Directory access blocked
- Files must live under `RECORDINGS_ROOT` (default `/recordings`)

### 3.3 MP4 Integrity Check
- Validates MP4 `ftyp` box presence in first 16 bytes
- Minimum file size check (1024 bytes)
- Runs on every playback request
- Failure logs a warning but still serves the file

### 3.4 Timeline Service
- Indexes recording segments by camera and time
- Provides `GET /timeline` and `GET /recording-timeline` endpoints
- Frame-level indexing via `FrameAnalysisService`
- Motion frame detection: `GET /motion-frames`
- Scene change detection: `GET /scene-changes`
- Audio metadata and level endpoints

### 3.5 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **MP4 integrity check is minimal** | MEDIUM | Only checks for `ftyp` box and minimum size. Does not validate moov box presence, does not attempt `ffmpeg -v error`, and does not check for truncation. A file that starts with `ftyp` but is truncated mid-stream will pass the check. |
| **No access control on playback URLs** | HIGH | The playback service relies on the API Gateway for authentication (`JWTAuthMiddleware`), but there is no per-camera or per-tenant authorization check. Any authenticated user can play back any recording. |
| **No recording-level authorization** | MEDIUM | The timeline service and recording queries in the API Gateway do join through `cameras -> sites` for tenant isolation, but the playback service itself serves files without tenant context. A tenant could access another tenant's recordings if they know the file path. |
| **Audio playback follows separate code path** | LOW | Audio playback has its own handler with its own path traversal checks, duplicating the same logic as the video handler. Could be refactored. |
| **No transcoding or adaptive bitrate** | LOW | `http.ServeFile` serves the original recorded file. No transcoding, no HLS/DASH packaging, no adaptive bitrate. Playback quality depends entirely on the original recording settings. |
| **No seek validation** | LOW | HTTP range requests are handled transparently by `http.ServeFile`. No validation that the requested byte range corresponds to valid video frames. |

---

## 4. Export Workflow

### 4.1 Export Flow
```
Client -> API Gateway -> Export Service (HTTP) -> FFmpeg -> /exports/ output
```

1. Export request received: `POST /export` with camera_id, start_time, end_time, watermark flag
2. Camera ID sanitized (alphanumeric, dash, underscore only)
3. Recording segments discovered via filesystem listing of `/recordings/{camera_id}/`
4. Path validation: `ValidateRecordingPath` + `ValidateFilePath` (must be under `/recordings`)
5. FFmpeg concat demuxer merges segments:
   ```
   ffmpeg -y -i seg1.mp4 -i seg2.mp4 ... -filter_complex concat=N -c:v libx264 -preset fast output.mp4
   ```
6. Optional watermark via `drawtext` filter
7. SHA-256 checksum computed on output
8. Result returned: file path, checksum, size

### 4.2 Evidence Management System
| Endpoint | Purpose |
|----------|---------|
| `/api/evidence/cases` | CRUD for evidence cases |
| `/api/evidence/lockers` | Secure digital evidence lockers |
| `/api/evidence/items` | Individual evidence items |
| `/api/evidence/share/` | Share access management |

### 4.3 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **Watermark text injection risk** | HIGH | The watermark text is constructed using string concatenation: `"drawtext=text='...Camera: " + req.CameraID + "'..."`. If the camera name contains FFmpeg filter special characters (e.g., `'`, `:`, `]`), this can break the filter syntax or potentially inject arbitrary filter commands. Camera ID is sanitized, but other fields are not. |
| **No export job queue** | MEDIUM | Export is synchronous. The HTTP request blocks until FFmpeg completes. For long exports (hours of video), this can timeout and the export is lost. No job ID, no progress tracking, no async completion notification. |
| **No export size limit** | MEDIUM | There is no limit on the time range or number of segments that can be exported. A multi-day export could consume all available disk space in `/exports/` or tie up FFmpeg for hours. |
| **Segment discovery via filesystem** | MEDIUM | Segments are discovered by listing the filesystem directory rather than querying the recordings database. This means exports won't include recordings that were indexed but whose files have been moved (e.g., to archive storage). |
| **No export cleanup** | LOW | Exported files accumulate in `/exports/` indefinitely. No retention or cleanup mechanism for completed exports. |
| **`/api/evidence` endpoints not behind admin role** | MEDIUM | Evidence case/locker/item endpoints use `JWTAuthMiddleware` only (any authenticated user). The API Gateway proxies them without `requireRole("admin")` or `requireRole("operator")`. |
| **FFmpeg process not killed on request cancellation** | MEDIUM | `exec.CommandContext(r.Context(), ...)` is used, which should kill FFmpeg if the request context is cancelled. However, the FFmpeg process group may not be properly terminated on all platforms. |

---

## 5. Storage Accounting

### 5.1 Implemented Metrics
| Metric | Source | Details |
|--------|--------|---------|
| `RecordingsIndexed` | Prometheus counter | Per-camera indexed segment count |
| `SegmentWriteDuration` | Prometheus histogram | Per-camera DB write duration |
| `IngestionRateTracker` | In-memory | Rolling 5-minute ingestion rate per camera |
| `/storage/estimates` | HTTP endpoint | Estimated storage needs based on current rate |
| `/storage/forecast` | HTTP endpoint | Forecast storage requirements |
| Per-camera stats | API Gateway | `COUNT(*)`, `SUM(file_size)`, `MIN(start_time)`, `MAX(end_time)` |

### 5.2 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **Ingestion rate tracker is local only** | MEDIUM | `IngestionRateTracker` is in-memory and per-instance. In a sharded recorder deployment, each shard has incomplete visibility into total ingestion rate across all cameras. |
| **No storage alerting** | LOW | There is no alert when storage utilization exceeds a configurable threshold (e.g., 80%, 90%, 95%). |
| **Per-camera storage stats query has no tenant isolation** | MEDIUM | `handleCameraRecording` in the API Gateway queries recordings directly by camera_id without joining through cameras -> sites for tenant filtering. A user could query recording stats for any camera. |

---

## 6. Legal Hold Integration

### 6.1 Legal Hold Model
| Field | Type | Notes |
|-------|------|-------|
| ID | UUID | Primary key |
| CameraID | string | Camera under hold |
| Reason | string | Legal reason for hold |
| CreatedBy | string | User who placed the hold |
| CreatedAt | timestamptz | |
| ReleasedAt | timestamptz | NULL if active |

### 6.2 Hold Enforcement
- Retention cleanup checks `legalHolds.IsOnHold(cameraID)` before deleting
- Hold is camera-level (all recordings for that camera are preserved)
- No individual recording-level holds
- API endpoints: create, list, release (admin-only via gateway)

### 6.3 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **Legal hold cache may be stale** | MEDIUM | Cache is refreshed on startup (`ImportLegacyHolds`) and on release (`ReleaseHold`). If a hold is created directly in the DB or by another instance, the cache won't reflect it until the next refresh. There is no periodic refresh mechanism for the legal hold cache. |
| **Camera-level granularity only** | MEDIUM | Legal holds apply to entire cameras. There is no mechanism to place a hold on a specific time range or specific recordings. If a hold is placed on a camera, ALL recordings for that camera are preserved indefinitely, even if only one small segment is relevant. |
| **No hold notification** | LOW | When recordings are skipped due to a legal hold, only a debug-level log is emitted. No alert, audit event, or notification is generated. |
| **No release notification** | LOW | Releasing a hold does not trigger any action. The recordings preserved under the hold remain until the next retention cleanup cycle (up to 1 hour later). |

---

## 7. General Infrastructure Findings

| Issue | Severity | Detail |
|-------|----------|--------|
| **Single bookmark server port (8087)** | LOW | The recorder service's HTTP API (bookmarks, legal holds, retention policies, timeline, etc.) runs on a single port. No separation of admin vs. user-facing endpoints at the service level. |
| **NATS connection health** | LOW | The recorder uses `ConnectNATSWithCircuitBreaker` but subscribes occur before the circuit breaker state is confirmed healthy. A flapping NATS connection could result in missed messages. |
| **No exactly-once semantics** | MEDIUM | NATS is at-least-once by default with queue groups, but there is no deduplication on the consumer side. If the same recording event is delivered twice, duplicate recording rows will be created. |
| **Shard ownership check race** | LOW | `OwnsCamera` is called in the NATS callback without holding the shard configuration lock. If shard configuration changes dynamically (unlikely but possible), a camera could be processed by the wrong shard. |

---

## 8. Summary of Critical and High Findings

1. **[HIGH] Watermark text injection risk**: FFmpeg filter text constructed via string concatenation. Malicious camera names could inject arbitrary filter commands.

2. **[HIGH] No access control on playback URLs**: Playback service has no per-camera or per-tenant authorization. Any authenticated user can access any recording.

3. **[MEDIUM] File stabilization polling blocks NATS goroutine**: Up to 70 seconds blocking in NATS callback per segment.

4. **[MEDIUM] No recording file integrity verification**: Moov atom fixup can silently produce corrupt output without validation.

5. **[MEDIUM] No gap detection**: Missing recording segments are not detected or alerted.

6. **[MEDIUM] Retention/legal hold cache staleness**: Policy and legal hold caches up to 1 hour stale, risking premature deletion or excess retention.

7. **[MEDIUM] No exactly-once semantics**: Duplicate NATS messages create duplicate recording rows.

8. **[MEDIUM] No export job queue**: Synchronous FFmpeg export blocks HTTP request, no progress tracking.

9. **[MEDIUM] No storage quota enforcement**: Retention is time-based only; no capacity-based enforcement.

10. **[MEDIUM] Legal hold camera-level only**: Cannot place holds on specific recordings or time ranges.

---

*End of Recording Validation Report*
