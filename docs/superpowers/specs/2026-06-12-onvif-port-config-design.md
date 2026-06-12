# ONVIF Per-Camera Port Configuration

## Problem

- Cameras may have ONVIF on non-standard ports (not 80)
- Legacy cameras may have RTSP only (no ONVIF at all)
- Current code hardcodes port 80 for ONVIF and attempts ONVIF connections even for RTSP-only cameras

## Approach (Shinobi-inspired)

Per-camera `config` JSONB controls ONVIF behavior via two fields:

```json
{
  "onvif_port": 8000,
  "is_onvif": true
}
```

- `is_onvif: false` → skip all ONVIF connection attempts for that camera
- `is_onvif: true` → use `onvif_port` for ONVIF HTTP URLs
- Default ONVIF port changed from 80 to 8000

## Backend Changes

### `pkg/onvif/soap.go` — `toHTTPURL()`
- When no explicit `onvifPort` is passed and RTSP port is 554: use `:8000` instead of `:80`

### `services/camera-control/main.go` — `getONVIFPort()`
- Check `config.is_onvif`; if false/absent, return 0 (skip ONVIF)
- Check `config.onvif_port`; if present, return it; default to 8000
- All ONVIF handlers return early with "ONVIF disabled" when port is 0

### `services/ingest/main.go` — `negotiateRTSPURL()` caller
- Check `config.is_onvif` before calling ONVIF negotiation
- If false, skip ONVIF entirely and use direct RTSP URL

## Frontend Changes

### `web/src/api/client.ts`
- Add `config` string field to `createCamera` / `updateCamera` payload types

### `web/src/components/cameras/CameraDialog.tsx`
- Add "ONVIF Enabled" toggle switch (default: on)
- Add "ONVIF Port" number input (default: 8000, 1-65535, disabled when toggle off)
- When toggle off and PTZ protocol is "onvif": show inline warning
- On submit: merge `{ onvif_port, is_onvif }` into existing camera `config` (preserve other keys), then `JSON.stringify()` the merge result into the payload

### `web/src/components/cameras/CameraDetailsDrawer.tsx`
- **Network tab**: show actual ONVIF port from `camera.config`, show ONVIF enabled/disabled
- **Diagnostics tab**: show "N/A (disabled)" instead of "ONVIF OK/Failed" when `is_onvif` is false

## Files to Modify

| File | Change |
|------|--------|
| `pkg/onvif/soap.go` | Default port 8000 |
| `services/camera-control/main.go` | Check `is_onvif` flag |
| `services/ingest/main.go` | Check `is_onvif` flag |
| `web/src/api/client.ts` | Add `config` to payload types |
| `web/src/components/cameras/CameraDialog.tsx` | ONVIF toggle + port field |
| `web/src/components/cameras/CameraDetailsDrawer.tsx` | Show ONVIF config status |
