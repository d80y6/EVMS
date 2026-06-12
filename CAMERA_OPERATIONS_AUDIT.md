# Camera Operations Audit Report
**Date:** 2026-06-11  
**Scope:** Camera CRUD, Discovery, PTZ Control, Snapshots, Health Monitoring, Offline Detection  
**Source Files:** `services/api-gateway/main.go`, `services/camera-mgmt/main.go`, `services/camera-control/main.go`, `services/discovery/main.go`, `web/src/components/cameras/`

---

## 1. Camera CRUD Lifecycle

### 1.1 Architecture Overview
```
UI (React) -> API Gateway (HTTP) -> Camera Management (gRPC) -> PostgreSQL
```

The API Gateway exposes REST endpoints (e.g. `POST /api/cameras`) and proxies to the camera-mgmt gRPC service. The camera-mgmt service handles all CRUD operations against the `cameras` table.

### 1.2 Data Model (`Camera` struct)
| Field | Type | Default | Notes |
|-------|------|---------|-------|
| ID | UUID | auto | Primary key |
| SiteID | UUID | required | FK to sites |
| Name | string | required | |
| Description | *string | nullable | |
| ConnectionURL | string | required | RTSP URL |
| SubstreamURL | *string | nullable | |
| Status | string | "offline" | "online"/"offline" |
| PtzProtocol | string | "NONE" | onvif, vapix, hikvision, pelco_d, pelco_p |
| RetentionDays | int | 30 | Per-camera retention |
| PrerecordSeconds | int | 5 | Pre-recording buffer size |
| OnvifData | JSONB | "{}" | Cached ONVIF capabilities |
| OnvifUsername | *string | nullable | |
| OnvifPassword | *string | nullable | AES encrypted |
| Config | JSONB | "{}" | Free-form camera config |
| CreatedAt | timestamp | now | |

### 1.3 CRUD Flow
- **Create (`POST /api/cameras`)** -> admin role required -> `CreateCamera` gRPC -> validates tenant ownership of site -> inserts with defaults -> returns full camera object (including decrypted password)
- **Read (`GET /api/cameras`)** -> auth required -> `ListCameras` gRPC -> tenant-scoped query -> returns array
- **Read single (`GET /api/cameras/{id}`)** -> auth required -> `GetCamera` gRPC -> tenant-scoped
- **Update (`PUT /api/cameras/{id}`)** -> operator role required -> `UpdateCamera` gRPC -> tenant-scoped -> `COALESCE(NULLIF(...))` pattern for partial updates
- **Delete (`DELETE /api/cameras/{id}`)** -> admin role required -> `DeleteCamera` gRPC -> hard DELETE from DB

### 1.4 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **No soft-delete** | MEDIUM | Camera deletion is a hard `DELETE`. No `deleted_at` column. Recordings referencing the camera ID become orphaned. |
| **No delete cascade validation** | MEDIUM | Deleting a camera does not check for associated recordings, events, or retention policies. Foreign key relationships exist at the application level only. |
| **ONVIF password exposed in API response** | HIGH | `mapCameraToProto` decrypts the ONVIF password (`common.MustDecrypt`) before returning it. Every API response includes the plaintext password. The UI hides it with a placeholder, but the API leaks it. |
| **No input validation on connection URL** | MEDIUM | The `ConnectionURL` field is accepted as-is without validation that it is a valid RTSP/HTTP URL. Malformed URLs are stored and will fail at health-check time. |
| **Partial update via COALESCE(NULLIF)** | LOW | `COALESCE(NULLIF($1, ''), site_id)` means empty strings are treated as "keep existing". Explicitly clearing a field to empty string is impossible. |
| **No audit logging on CRUD** | LOW | Camera create/update/delete operations do not emit audit events. The audit service is configured in the gateway but not wired for camera lifecycle. |

---

## 2. Discovery Workflow

### 2.1 Architecture
```
UI Wizard -> API Gateway -> Discovery Service (HTTP) -> Scanners -> PostgreSQL
                                                          |-> NATS (optional)
```

Four scanner implementations:
- **WS-Discovery** - Web Services Dynamic Discovery (SOAP multicast)
- **IP Range** - Direct IP scan on common ports (80, 554, 8080)
- **mDNS** - Multicast DNS (Bonjour/Avahi)
- **Manual IP** - Single IP probe

### 2.2 Scan Lifecycle
1. User submits scan with subnet, site ID -> creates `discovery_scans` row with status "pending"
2. `ScanOrchestrator` runs configured scanners asynchronously
3. Results stored in `discovery_results` table
4. UI polls every 2s for up to 60s to check completion (max 30 attempts)
5. User selects devices to import, provides ONVIF credentials
6. Credential test via ONVIF `GetDeviceInformation` probe
7. Import creates cameras via `camera-mgmt` gRPC

### 2.3 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **Synchronous polling** | LOW | UI polls scan status via `setTimeout` loop. No WebSocket/NATS push for scan completion. |
| **No scan timeout enforcement** | MEDIUM | The scanner runs without a hard deadline. `ScanTimeout` config exists (5s) but is per-probe, not per-scan. Long-running scans could accumulate. |
| **Import creates cameras without ONVIF data probe** | MEDIUM | During import, cameras are created with the RTSP URL but ONVIF capabilities are not fetched/attached. The wizard tests credentials separately but does not store ONVIF data at import time. |
| **No duplicate IP detection** | MEDIUM | Import does not check if a camera with the same IP already exists. Duplicate entries can be created. |
| **Results not paginated** | LOW | `discovery_results` can contain hundreds of devices; the GET endpoint returns all results without pagination. |
| **Default credentials hardcoded in UI** | LOW | The UI wizard pre-fills username as "admin". This is a common default and may encourage weak credentials. |
| **No scan cleanup** | LOW | No mechanism to purge old discovery scans/results. They accumulate indefinitely in the DB. |

---

## 3. PTZ Control Flow

### 3.1 Architecture
```
UI -> API Gateway (HTTP) -> Camera Control Service (HTTP) -> Camera (ONVIF/VAPIX/Hikvision)
```

The `camera-control` service handles all PTZ, imaging, device, and network operations. It retrieves camera credentials from `camera-mgmt` gRPC, then communicates directly with the camera using the appropriate protocol driver.

### 3.2 Supported Protocols
| Protocol | Network Type | Implemented Commands |
|----------|-------------|---------------------|
| ONVIF | SOAP/XML | ContinuousMove, AbsoluteMove, RelativeMove, Stop, GotoPreset, SetPreset, RemovePreset, GotoHome, SetHome, GetStatus |
| VAPIX (Axis) | HTTP GET | move, zoom, stop, goto_preset, set_preset |
| Hikvision | HTTP PUT XML | move, zoom, stop, goto_preset, set_preset (ISAPI) |

### 3.3 PTZ Routing
```
/cameras/{id}/ptz/move               POST (direction, speed)
/cameras/{id}/ptz/zoom               POST (zoom)
/cameras/{id}/ptz/presets            GET (list) / POST (set)
/cameras/{id}/ptz/presets/{id}/goto  POST
/cameras/{id}/ptz/presets/{id}       DELETE
/cameras/{id}/ptz/stop               POST
/cameras/{id}/ptz/home               POST
/cameras/{id}/ptz/set-home           POST
/cameras/{id}/ptz/absolute-move      POST (pan, tilt, zoom)
/cameras/{id}/ptz/relative-move      POST (pan, tilt, zoom)
/cameras/{id}/ptz/status             GET
```

### 3.4 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **No PTZ rate limiting** | MEDIUM | PTZ commands are accepted at any rate. No minimum interval enforcement. Could cause excessive wear on mechanical PTZ hardware. |
| **VAPIX/Hikvision responses discarded** | LOW | Both VAPIX and Hikvision command functions use `io.Copy(io.Discard, resp.Body)` - response body is read and discarded but never parsed for error messages. Only HTTP status code is checked. |
| **getONVIFProfileToken uses background context** | MEDIUM | `onvifCommand` calls `getONVIFProfileToken(context.Background(), ...)` instead of propagating the request context. Profile token requests are not subject to parent request timeout. |
| **PTZ status only for ONVIF** | MEDIUM | The `handlePTZStatus` endpoint explicitly rejects non-ONVIF protocols: "PTZ status only supported for ONVIF". VAPIX/Hikvision cameras with PTZ cannot report position. |
| **No PTZ preset caching** | LOW | Presets are fetched from the camera on every GET request. No in-memory caching. Can be slow for cameras with many presets. |
| **IO relay control limited to ONVIF** | LOW | Relay output control (door locks, lights) is only implemented via ONVIF SOAP. No VAPIX or Hikvision equivalent. |
| **No concurrent PTZ command guarding** | MEDIUM | Multiple simultaneous PTZ commands to the same camera are not queued. The last command overwrites the previous one at the camera level. |

---

## 4. Snapshot Retrieval

### 4.1 Flow
```
CameraSnapshot.tsx (UI) -> GET /api/cameras/{id}/snapshot (camera-control)
                         -> ONVIF GetSnapshotURI -> returns URL -> <img> loads URL
```

- Auto-refresh interval: 30 seconds (configurable via `refreshInterval` prop)
- Manual refresh button available
- Error state with retry button

### 4.2 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **Snapshot URL exposed directly to client** | MEDIUM | The ONVIF snapshot URI is returned to the client and loaded directly in an `<img>` tag. This URL may include a non-expiring, unauthenticated access token depending on the camera model. |
| **No server-side caching** | MEDIUM | Each snapshot request goes directly to the camera ONVIF service. No image cache layer. Multiple UI sessions hitting the same camera simultaneously generate redundant ONVIF calls. |
| **No snapshot fallback** | LOW | If ONVIF `GetSnapshotURI` fails, there is no fallback to RTSP frame grabbing, FFmpeg thumbnail generation, or other methods. |
| **Cross-origin image loading** | LOW | The snapshot URL may be on a different origin (the camera's raw IP). If the camera does not set CORS headers, the image may fail to render in browser contexts. |

---

## 5. Offline Detection and Recovery

### 5.1 Health Check Implementation
- **Interval:** 30 seconds
- **Method:** TCP dial to camera IP on port 554, fallback to port 80
- **Scope:** ALL cameras in the database (no pagination)
- **Status transition:** "online" <-> "offline"
- **DB update:** `UPDATE cameras SET status = $1, updated_at = NOW() WHERE id = $2`

### 5.2 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **TCP-only health check** | MEDIUM | The check only tests TCP connectivity. It does not verify RTSP handshake, ONVIF availability, or video stream health. A camera could be TCP-reachable but not streaming. |
| **No exponential backoff** | MEDIUM | The health check runs at a fixed 30s interval regardless of camera count or failure rate. For large deployments (1000+ cameras), this generates 1000+ TCP connections every 30s. |
| **No pagination in health check query** | MEDIUM | `SELECT ... FROM cameras c` loads ALL cameras into memory. For large deployments this is a scalability concern. |
| **No shard awareness** | LOW | The health check runs on every camera-mgmt instance. In a sharded deployment, each instance would check all cameras regardless of shard ownership. |
| **No recovery actions** | MEDIUM | When a camera transitions from "offline" back to "online", no recovery action is triggered (e.g., restart recording session, refresh ONVIF data, re-subscribe to events). |
| **Status change not published via NATS** | LOW | Camera status changes are logged but not published via NATS. Other services (recorder, alert engine, event processor) cannot react to status transitions. |

---

## 6. Camera Health Monitoring

### 6.1 Diagnostics Endpoints
| Endpoint | Source | Data Provided |
|----------|--------|---------------|
| `/api/cameras/{id}/diagnostics` | API Gateway (stub) | reachable, onvif, rtsp, latency_ms, last_error, status, uptime_pct, response_time_ms |
| `/api/cameras/{id}/recording` | API Gateway (DB) | retention_days, recordings_count, oldest/latest recording, prerecord_seconds, storage_used |
| `/api/cameras/{id}/details` | API Gateway (DB+gRPC) | All camera fields + site_name, manufacturer, model, firmware, serial_number, hardware_id |
| `/api/health/system` | API Gateway | Upstream service health (all microservices) |

### 6.2 Findings and Risks
| Issue | Severity | Detail |
|-------|----------|--------|
| **Diagnostics data is largely synthetic** | MEDIUM | `handleCameraDiagnostics` returns hardcoded `uptime_pct: 99.5` and `response_time_ms: 45`. Latency is always 0. These values are not actually measured from the device. |
| **Individual DB queries for each detail field** | LOW | `handleCameraDetails` issues 5 separate `db.GetContext` calls (site name, manufacturer, model, firmware, serial number, hardware ID). These should be consolidated into a single JOIN query. |
| **No aggregated camera dashboard endpoint** | LOW | No single endpoint returns overall health status (count online/offline, total storage used, etc.). The UI must iterate all cameras individually. |
| **Upstream health lacks camera-mgmt gRPC check** | MEDIUM | The system health endpoint checks HTTP health of each service, but camera-mgmt's gRPC service health (port 50051) is not verified - only its HTTP health endpoint (port 8083) is checked. |

---

## 7. Rate Limiting and Authorization Summary

| Feature | Middleware | Role Required | Rate Limited |
|---------|-----------|---------------|--------------|
| List cameras | authMiddleware | any authenticated | yes |
| Get camera | authMiddleware | any authenticated | yes |
| Create camera | requireRole | admin | yes |
| Update camera | requireRole | operator | yes |
| Delete camera | requireRole | admin | yes |
| PTZ commands | requireRole | operator | yes |
| Discovery scan | requireRole | operator | yes |
| Discovery import | requireRole | operator | yes |
| License enforcement | licenseMiddleware | applies to POST /api/cameras | no |

### Rate Limiter Configuration
- Rate: 100 requests/second
- Burst: 200 requests
- Cleanup: every 10 minutes for stale IPs
- In-memory (not shared across gateway instances)

### Authorization Findings
- Rate limiter is per-instance (in-memory). In multi-replica deployments, each gateway has its own independent token bucket.
- CSRF protection enabled for all mutating endpoints (POST/PUT/DELETE) except login and webhooks.
- IP allowlist feature exists but requires database configuration to enable.
- License enforcement only checks max camera count; does not enforce feature tiers.

---

## 8. Summary of Critical and High Findings

1. **[HIGH] ONVIF password leak**: Decrypted credentials exposed in API responses. `mapCameraToProto` calls `common.MustDecrypt()` and returns the plaintext password in every camera response.

2. **[MEDIUM] No retention enforcement for deleted cameras**: Hard delete creates orphan recording rows with no referential integrity.

3. **[MEDIUM] Discovery import lacks duplicate detection**: Same camera IP can be imported multiple times, creating duplicate entries.

4. **[MEDIUM] PTZ command rate unconstrained**: No minimum interval enforcement between PTZ commands to the same camera.

5. **[MEDIUM] Health check is TCP-only**: Does not verify actual RTSP streaming or ONVIF service availability.

6. **[MEDIUM] No NATS publish on camera status change**: Other services cannot react to camera online/offline transitions.

7. **[MEDIUM] Scale concern**: All-cameras health check query loads everything into memory without pagination.

8. **[MEDIUM] Synthetic diagnostics data**: `uptime_pct`, `response_time_ms`, and latency values are hardcoded, not measured.

9. **[MEDIUM] getONVIFProfileToken uses background context**: Profile token discovery is not bounded by request timeout.

10. **[MEDIUM] Snapshot URLs exposed directly**: ONVIF snapshot URIs returned to client without server-side proxy or caching.

---

*End of Camera Operations Audit*
