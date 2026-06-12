# Camera Details API + Camera Discovery API Design

## Overview

Update the DAM VMS API gateway and frontend TypeScript client to support
professional VMS frontend components: CameraDetailsDrawer and CameraDiscoveryWizard.

## Phase 1 — Camera Details API Updates

Update 7 existing gateway handlers to return the spec-required fields combined
with existing extras.

### /details — handleCameraDetails

Current: 11 fields (id, name, description, site_id, site_name, ip_address, status,
connection_url, ptz_protocol, retention_days, created_at)

Add: manufacturer, model, firmware, serial_number, hardware_id (query from
`cameras.onvif_data` JSONB, same pattern used in the removed duplicate handler)

New response: all current fields + manufacturer, model, firmware, serial_number,
hardware_id.

### /streams — handleCameraStreams

Current: `{profiles: [{id, name, url, encoding, width, height, fps, bitrate}]}`

Restructure to match spec:
```json
{
  "main_stream": "",
  "sub_stream": "",
  "profiles": [
    {"token": "", "name": "", "resolution": "", "fps": 0, "codec": ""}
  ]
}
```

Keep internal profile data (encoding, width, height, bitrate, url) available
but map to spec shape. `resolution` = `"{width}x{height}"`, `codec` = `encoding`,
`token` = `id`.

### /ptz — handleCameraPTZ

Current: `{protocol, absolute_move, relative_move, continuous_move, presets_count, presets}`

Restructure to:
```json
{
  "protocol": "",
  "supported": true,
  "presets": [{"token": "", "name": ""}]
}
```

`supported` = `protocol != "NONE"`. Keep `absolute_move`, `relative_move`,
`continuous_move` as extras.

### /network — handleCameraNetwork

Current: `{ip_address, rtsp_port, onvif_port, http_port, dhcp}`

Restructure to:
```json
{
  "hostname": "",
  "dns": [],
  "ntp": [],
  "interfaces": [{"name": "", "ipv4": "", "mac": ""}]
}
```

Maintain `ip_address`, `rtsp_port`, `onvif_port`, `http_port` as extras.
`interfaces` populated from parsed connection_url when available; hostname, dns,
ntp left empty (would need real ONVIF queries which belong in the discovery
service, not the gateway).

### /diagnostics — handleCameraDiagnostics

Current: `{reachable, status, uptime_pct, response_time_ms}`

Restructure to:
```json
{
  "reachable": true,
  "onvif": true,
  "rtsp": true,
  "latency_ms": 0,
  "last_error": ""
}
```

`reachable` = `camera.Status == "online"`. `onvif`/`rtsp` derived from status.
Keep `status`, `uptime_pct`, `response_time_ms` as extras.

### /recording — handleCameraRecording

Current: `{retention_days, prerecord_seconds, recording_enabled, total_recordings, storage_used_bytes, storage_available_bytes}`

Restructure to:
```json
{
  "retention_days": 30,
  "recordings_count": 0,
  "oldest_recording": "",
  "latest_recording": ""
}
```

Query `MIN(start_time)` and `MAX(end_time)` from recordings table for
oldest/latest. Keep prerecord_seconds, recording_enabled, storage_used_bytes,
storage_available_bytes as extras.

### /onvif — handleCameraOnvif

Current: `{device_uri, username, analytics, events, ptz, imaging, firmware, serial_number, hardware, manufacturer, model}`

Restructure to:
```json
{
  "username": "",
  "capabilities": {},
  "events_supported": true,
  "analytics_supported": true
}
```

Parse `onvif_data` JSONB to populate `capabilities` map. Keep `analytics`,
`events`, `ptz`, `imaging`, `device_uri` as extras.

---

## Phase 2 — Camera Discovery API

Add 6 new handler functions in the gateway. These handlers directly access `g.db`
using the existing `discovery_scans` and `discovery_results` tables
(migration 010).

The existing `handleDiscovery` proxy remains as fallback for any discovery paths
the gateway doesn't explicitly handle. The new handlers are matched BEFORE the
generic `"/api/discovery/"` prefix case in the switch statement.

### POST /api/discovery/scan

Request body:
```json
{"subnet": "192.168.1.0/24"}
```

Handler creates a new `discovery_scans` record with status=`"pending"` and the
provided subnet. The actual scanning is performed asynchronously by the existing
discovery service's orchestrator.

Response:
```json
{"scan_id": ""}
```

### GET /api/discovery/scans

Queries `discovery_scans` ordered by `created_at DESC`. Returns simplified
records.

Response:
```json
{"scans": [{"id": "", "status": "", "started_at": "", "completed_at": ""}]}
```

### GET /api/discovery/scans/{id}

Queries a single scan by ID.

Response:
```json
{"id": "", "status": "", "started_at": "", "completed_at": ""}
```

### GET /api/discovery/scans/{id}/results

Queries `discovery_results` for the given scan ID.

Response:
```json
{
  "devices": [
    {"ip": "", "manufacturer": "", "model": "", "serial_number": "", "onvif": true, "rtsp": true}
  ]
}
```

### POST /api/discovery/test-credentials

Tests ONVIF credentials against a camera at the given IP.

Request:
```json
{"ip": "", "username": "", "password": ""}
```

Handler constructs an ONVIF device URL (`http://{ip}:80/onvif/device_service`),
calls the ONVIF `GetDeviceInformation` via `pkg/onvif`. Uses the same pattern
as the discovery service's `handleTestCredentials`.

Response:
```json
{"success": true, "manufacturer": "", "model": ""}
```

### POST /api/discovery/import

Imports discovery results as camera records.

Request:
```json
{"scan_id": "", "devices": [], "site_id": "", "username": "", "password": ""}
```

Handler iterates devices, creates a `CreateCameraRequest` for each via gRPC
`cameraSvc`, marks results as imported in `discovery_results`.

Response:
```json
{"created": 0, "failed": 0}
```

### Route Registration

New cases are inserted in the `ServeHTTP` switch statement BEFORE the
`"/api/discovery/"` proxy catch-all:

```go
case path == "/api/discovery/scan" && r.Method == http.MethodPost:
    g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleDiscoveryScan))(w, r)
case path == "/api/discovery/scans" && r.Method == http.MethodGet:
    g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscoveryListScans))(w, r)
case strings.HasPrefix(path, "/api/discovery/scans/") && strings.HasSuffix(path, "/results") && r.Method == http.MethodGet:
    g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscoveryGetResults))(w, r)
case strings.HasPrefix(path, "/api/discovery/scans/") && r.Method == http.MethodGet:
    g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscoveryGetScan))(w, r)
case path == "/api/discovery/test-credentials" && r.Method == http.MethodPost:
    g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleDiscoveryTestCredentials))(w, r)
case path == "/api/discovery/import" && r.Method == http.MethodPost:
    g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleDiscoveryImport))(w, r)
```

---

## Phase 3 — Camera Update Verification

The `PUT /api/cameras/{id}` flow is already complete for the specified fields:

| Layer | File | Status |
|-------|------|--------|
| HTTP handler | `api-gateway/main.go:990` | Parses all 9 fields |
| gRPC client | `api-gateway/main.go:1012` | Passes all fields to UpdateCamera |
| Proto message | `api/proto/v1/camera.proto:66` | All 9 fields + config, prerecord_seconds |
| gRPC server | `camera-mgmt/main.go:296` | All fields in UPDATE SQL |
| SQL | `camera-mgmt/main.go:315` | All columns updated |
| TypeScript client | `web/src/api/client.ts:550` | All 9 fields in Partial type |

No code changes needed for Phase 3 — the flow is end-to-end correct. The
TypeScript `Camera` return type already covers the response via `request<Camera>`.

---

## Phase 4 — TypeScript API Client Types

Update `web/src/api/client.ts`:

### New Camera Detail Response Types

Replace inline return types for each camera detail method with named interfaces:

- `CameraDetailsResponse` — all /details fields
- `CameraStreamsResponse` — {main_stream, sub_stream, profiles}
- `CameraPTZResponse` — {protocol, supported, presets}
- `CameraNetworkResponse` — {hostname, dns, ntp, interfaces}
- `CameraDiagnosticsResponse` — {reachable, onvif, rtsp, latency_ms, last_error}
- `CameraRecordingResponse` — {retention_days, recordings_count, oldest, latest}
- `CameraOnvifResponse` — {username, capabilities, events_supported, analytics_supported}

### New Discovery Response Types

- `DiscoveryScanResponse` — {id, status, started_at, completed_at}
- `DiscoveryListScansResponse` — {scans: DiscoveryScanResponse[]}
- `DiscoveryResultsResponse` — {devices: DiscoveryDevice[]}
- `DiscoveryDevice` — {ip, manufacturer, model, serial_number, onvif, rtsp}
- `DiscoveryScanResult` — {scan_id}
- `DiscoveryTestCredentialsResponse` — {success, manufacturer, model}
- `DiscoveryImportResponse` — {created, failed}

### Update Discovery API Methods

Add new API methods matching the spec paths:

- `startScan({subnet})` → `POST /api/discovery/scan`
- `listScans()` → `GET /api/discovery/scans`
- `getScan(id)` → `GET /api/discovery/scans/{id}`
- `getScanResults(id)` → `GET /api/discovery/scans/{id}/results`
- `testCredentials({ip, username, password})` → `POST /api/discovery/test-credentials`
- `importDevices({scan_id, devices, site_id, username, password})` → `POST /api/discovery/import`

---

## Implementation Order

1. Phase 1: Update 7 camera detail handlers (gateway)
2. Phase 2: Add 6 discovery handlers + route registration (gateway)
3. Phase 3: Verify update path (no code change expected)
4. Phase 4: Update TypeScript client types

---

## Files Changed

| File | Change |
|------|--------|
| `services/api-gateway/main.go` | Update 7 handlers, add 6 handlers, add route cases |
| `web/src/api/client.ts` | New interfaces, new methods, update existing method return types |
