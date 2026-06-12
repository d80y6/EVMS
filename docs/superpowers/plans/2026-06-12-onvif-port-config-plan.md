# ONVIF Per-Camera Port & Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-camera `is_onvif` toggle and `onvif_port` configuration, with backend skip logic and frontend UI fields.

**Architecture:** Two new fields in camera `config` JSONB control ONVIF behavior per-camera: `is_onvif` (bool, default true) and `onvif_port` (int, default 8000). Backend checks these before any ONVIF call; frontend exposes them as a toggle + number input in the camera dialog and shows status in the detail drawer.

**Tech Stack:** Go (pkg/onvif, services/camera-control, services/ingest, services/api-gateway), React/TypeScript (web/src), Postgres JSONB.

---

### Task 1: Backend — Update `toHTTPURL` default and `getONVIFPort` to check `is_onvif`

**Files:**
- Modify: `pkg/onvif/soap.go:195-196`
- Modify: `services/camera-control/main.go:559-569`

- [ ] **Step 1: Change default ONVIF port in `toHTTPURL` from 80 to 8000**

In `pkg/onvif/soap.go`, change line 196 from:
```go
			u.Host = u.Hostname() + ":80"
```
to:
```go
			u.Host = u.Hostname() + ":8000"
```

This is a safety net for any callers that don't pass an explicit port. The actual port is determined by `getONVIFPort` in all current code paths.

- [ ] **Step 2: Update `getONVIFPort` to check `is_onvif` and default to 8000**

```go
func getONVIFPort(camera *damv1.Camera) int {
	if camera.Config == "" {
		return 0
	}
	var cfg struct {
		OnvifPort int  `json:"onvif_port"`
		IsOnvif   bool `json:"is_onvif"`
	}
	if err := json.Unmarshal([]byte(camera.Config), &cfg); err != nil {
		return 0
	}
	if !cfg.IsOnvif {
		return 0
	}
	if cfg.OnvifPort > 0 {
		return cfg.OnvifPort
	}
	return 8000
}
```

- [ ] **Step 3: Build and verify it compiles**

Run: `docker compose --env-file /home/ubuntu/EVMS/.env -f /home/ubuntu/EVMS/deploy/docker/docker-compose.yml build camera-control 2>&1 | tail -5`
Expected: `Image docker-camera-control Built`

---

### Task 2: Backend — Add early return in camera-control handlers when ONVIF is disabled

**Files:**
- Modify: `services/camera-control/main.go:183-507`

All PTZ handlers that take `camera *damv1.Camera` should check `getONVIFPort(camera) == 0` for onvif-protocol cameras and return early with "ONVIF disabled".

- [ ] **Step 1: Add helper function to check if ONVIF is disabled**

```go
func isONVIFDisabled(camera *damv1.Camera) bool {
	return camera.PtzProtocol == "onvif" && getONVIFPort(camera) == 0
}
```

Add this right after the `getONVIFPort` function (after line 570).

- [ ] **Step 2: Add early return to `handleMove`**

After line 187 (method check), add:
```go
	if isONVIFDisabled(camera) {
		jsonError(w, "ONVIF disabled for this camera", http.StatusBadRequest)
		return
	}
```

- [ ] **Step 3: Add early return to `handleZoom`**

After line 215, add the same check.

- [ ] **Step 4: Add early return to `handleStop`**

After line 290 (check the function start), add the same check.

- [ ] **Step 5: Add early return to `handleListPresets`**

After line 319, add the same check.

- [ ] **Step 6: Add early return to `handleSetPreset`**

After line 330, add the same check.

- [ ] **Step 7: Add early return to `handleGotoPreset`**

After line 352, add the same check.

- [ ] **Step 8: Add early return to `handleRemovePreset`**

After line 380, add the same check.

- [ ] **Step 9: Add early return to `handleGotoHome`**

After line 395, add the same check.

- [ ] **Step 10: Add early return to `handleSetHome`**

After line 404, add the same check.

- [ ] **Step 11: Add early return to `handleAbsoluteMove`**

After line 420, add the same check.

- [ ] **Step 12: Add early return to `handleRelativeMove`**

After line 460, add the same check.

- [ ] **Step 13: Add early return to `handlePTZStatus`**

After line 492, add:
```go
	if getONVIFPort(camera) == 0 {
		jsonError(w, "ONVIF disabled for this camera", http.StatusBadRequest)
		return
	}
```

- [ ] **Step 14: Add early return to focus handlers (`handleMoveFocus`, `handleStopFocus`)**

Add `isONVIFDisabled` check to both.

- [ ] **Step 15: Build and verify**

Run: `docker compose --env-file /home/ubuntu/EVMS/.env -f /home/ubuntu/EVMS/deploy/docker/docker-compose.yml build camera-control 2>&1 | tail -5`
Expected: `Image docker-camera-control Built`

---

### Task 3: Backend — Update ingest-service to skip ONVIF when `is_onvif` is false

**Files:**
- Modify: `services/ingest/main.go:748-758`

- [ ] **Step 1: Add `IsOnvif` to config struct and skip ONVIF if disabled**

Replace the existing onvifPort parsing block (lines 748-756):
```go
		onvifPort := 0
		skipONVIF := false
		if cam.Config != "" {
			var cfg struct {
				OnvifPort int  `json:"onvif_port"`
				IsOnvif   bool `json:"is_onvif"`
			}
			if err := json.Unmarshal([]byte(cam.Config), &cfg); err == nil {
				onvifPort = cfg.OnvifPort
				skipONVIF = !cfg.IsOnvif
			}
		}

		var rtspURL string
		if skipONVIF || onvifPort == 0 || username == "" || password == "" {
			rtspURL = cam.ConnectionURL
		} else {
			rtspURL, err = negotiateRTSPURL(ctx, cam.ConnectionURL, username, password, onvifPort)
			if err != nil {
				s.logger.Warn("ONVIF negotiation failed, using direct RTSP URL",
					"camera_id", cam.ID,
					"error", err)
				rtspURL = cam.ConnectionURL
			}
		}
```

Note: The old code at line 758 had `rtspURL, err := negotiateRTSPURL(...)` which used `:=` (shadowing the outer `rtspURL`). The new code uses `=` so needs a preceding `var rtspURL string`. Remove the old `rtspURL, err := ...` line and ensure `err` is still declared (the `err` variable is used later in the function for the `if err != nil` check of `NewStreamProcessor`).

Actually, look at lines 758-763 more carefully:

```go
rtspURL, err := negotiateRTSPURL(ctx, cam.ConnectionURL, username, password, onvifPort)
if err != nil {
    s.logger.Warn("ONVIF negotiation failed, using direct RTSP URL",
        "camera_id", cam.ID,
        "error", err)
}
```

The `err` from `negotiateRTSPURL` is only used for the warning log. There's no subsequent check of `err`. And the `rtspURL` variable is used at line 755 in `NewStreamProcessor`. Since `rtspURL` is declared with `:=`, it only exists in this scope. The `NewStreamProcessor` at line 765 uses `rtspURL` within the same `for` loop iteration scope.

So the correct replacement is:
```go
		onvifPort := 0
		skipONVIF := false
		if cam.Config != "" {
			var cfg struct {
				OnvifPort int  `json:"onvif_port"`
				IsOnvif   bool `json:"is_onvif"`
			}
			if err := json.Unmarshal([]byte(cam.Config), &cfg); err == nil {
				onvifPort = cfg.OnvifPort
				skipONVIF = !cfg.IsOnvif
			}
		}

		rtspURL := cam.ConnectionURL
		if !skipONVIF && onvifPort > 0 && username != "" && password != "" {
			if negotiated, err := negotiateRTSPURL(ctx, cam.ConnectionURL, username, password, onvifPort); err == nil {
				rtspURL = negotiated
			} else {
				s.logger.Warn("ONVIF negotiation failed, using direct RTSP URL",
					"camera_id", cam.ID,
					"error", err)
			}
		}
```

- [ ] **Step 2: Verify the replacement keeps the surrounding code intact**

Read lines 765-775 to ensure `NewStreamProcessor` still uses `rtspURL` correctly.

- [ ] **Step 3: Build and verify**

Run: `docker compose --env-file /home/ubuntu/EVMS/.env -f /home/ubuntu/EVMS/deploy/docker/docker-compose.yml build ingest-service 2>&1 | tail -5`
Expected: `Image docker-ingest-service Built`

---

### Task 4: Backend — Update api-gateway network/diagnostics handlers to show real ONVIF config

**Files:**
- Modify: `services/api-gateway/main.go:1250-1331`

- [ ] **Step 1: Add config parsing helper in `handleCameraNetwork`**

Replace lines 1292-1303 (the response block) with:
```go
	onvifPort := 80
	onvifEnabled := true
	if camera.Config != "" {
		var cfg struct {
			OnvifPort int  `json:"onvif_port"`
			IsOnvif   bool `json:"is_onvif"`
		}
		if err := json.Unmarshal([]byte(camera.Config), &cfg); err == nil {
			if cfg.OnvifPort > 0 {
				onvifPort = cfg.OnvifPort
			}
			onvifEnabled = cfg.IsOnvif
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hostname":       "",
		"dns":            []string{},
		"ntp":            []string{},
		"interfaces":     interfaces,
		"ip_address":     ipAddress,
		"rtsp_port":      rtspPort,
		"onvif_port":     onvifPort,
		"onvif_enabled":  onvifEnabled,
		"http_port":      80,
		"dhcp":           true,
	})
```

- [ ] **Step 2: Update `handleCameraDiagnostics` to show N/A when ONVIF disabled**

Replace lines 1321-1331 with:
```go
	onvifStatus := reachable
	if camera.Config != "" {
		var cfg struct {
			IsOnvif bool `json:"is_onvif"`
		}
		if err := json.Unmarshal([]byte(camera.Config), &cfg); err == nil && !cfg.IsOnvif {
			onvifStatus = false
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reachable":        reachable,
		"onvif":            onvifStatus,
		"onvif_enabled":    onvifStatus,
		"rtsp":             reachable,
		"latency_ms":       0,
		"last_error":       "",
		"status":           camera.Status,
		"uptime_pct":       99.5,
		"response_time_ms": 45,
	})
```

Wait, the actual change for diagnostics should be: if ONVIF is disabled, set "onvif" field to a special value so the frontend can show "N/A". The simplest approach is to send `"onvif": null` when disabled. But since JSON encoding in Go would render `null` for nil interface, we need to handle this.

Better approach: add an `"onvif_enabled"` boolean field and keep `"onvif"` as is. The frontend checks `onvif_enabled` first, and if false shows "N/A" instead of the onvif status.

Actually, the simplest approach for diagnostics: just keep the current `"onvif": reachable` but also send `"onvif_enabled": true/false`. The frontend will use `onvif_enabled` to decide whether to show OK/Failed or N/A.

Let me restructure:
```go
	onvifEnabled := true
	if camera.Config != "" {
		var cfg struct {
			IsOnvif bool `json:"is_onvif"`
		}
		if err := json.Unmarshal([]byte(camera.Config), &cfg); err == nil {
			onvifEnabled = cfg.IsOnvif
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reachable":      reachable,
		"onvif":          onvifEnabled && reachable,
		"onvif_enabled":  onvifEnabled,
		"rtsp":           reachable,
		"latency_ms":     0,
		"last_error":     "",
		"status":         camera.Status,
		"uptime_pct":     99.5,
		"response_time_ms": 45,
	})
```

- [ ] **Step 3: Build and verify**

Run: `docker compose --env-file /home/ubuntu/EVMS/.env -f /home/ubuntu/EVMS/deploy/docker/docker-compose.yml build api-gateway 2>&1 | tail -5`
Expected: `Image docker-api-gateway Built`

---

### Task 5: Frontend — Add `config` to createCamera/updateCamera payload types

**Files:**
- Modify: `web/src/api/client.ts:695-718`

- [ ] **Step 1: Add `config` field to createCamera and updateCamera payloads**

For `createCamera` (line 695), add `config?: string` to the parameter type.
For `updateCamera` (line 701), add `config?: string` to the `data` partial type.

```typescript
  createCamera: (data: {
    site_id: string;
    name: string;
    connection_url: string;
    substream_url?: string;
    ptz_protocol?: string;
    retention_days?: number;
    onvif_username?: string;
    onvif_password?: string;
    config?: string;
  }) =>

  updateCamera: (
    id: string,
    data: Partial<{
      site_id: string;
      name: string;
      description: string;
      connection_url: string;
      substream_url: string;
      ptz_protocol: string;
      retention_days: number;
      onvif_username: string;
      onvif_password: string;
      config?: string;
    }>
  ) =>
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors related to client.ts

---

### Task 6: Frontend — Add ONVIF toggle + port field to CameraDialog

**Files:**
- Modify: `web/src/components/cameras/CameraDialog.tsx`

- [ ] **Step 1: Add state for config fields**

After line 41 (`const [form, setForm] = useState(emptyForm);`), add:
```typescript
const [config, setConfig] = useState<{ onvif_port: number; is_onvif: boolean }>({
  onvif_port: 8000,
  is_onvif: true,
});
```

- [ ] **Step 2: Load config on edit**

In the `useEffect` that populates the form from `camera` (lines 43-66), after setting `onvif_password: ''`, add:
```typescript
if (camera.config) {
  try {
    const parsed = JSON.parse(camera.config);
    setConfig({
      onvif_port: parsed.onvif_port || 8000,
      is_onvif: parsed.is_onvif !== false,
    });
  } catch {
    setConfig({ onvif_port: 8000, is_onvif: true });
  }
} else {
  setConfig({ onvif_port: 8000, is_onvif: true });
}
```

- [ ] **Step 3: Reset config on new camera**

In the `if (!camera)` branch of the same useEffect, add `setConfig({ onvif_port: 8000, is_onvif: true });`

- [ ] **Step 4: Add config to save payload**

In `handleSave`, after the `payload.onvif_password` assignment (line 136), add:
```typescript
payload.config = JSON.stringify(config);
```

- [ ] **Step 5: Add UI fields after the PTZ Protocol row**

After the closing `</div>` of the PTZ Protocol + Retention Days grid (line 370), insert a new two-column grid:

```tsx
          <div className="grid grid-cols-2 gap-4">

            <div>
              <label className="block text-xs text-slate-500 mb-1">
                ONVIF Enabled
              </label>

              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={config.is_onvif}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      is_onvif: e.target.checked,
                    })
                  }
                  className="w-4 h-4 rounded border-slate-600 bg-slate-800 text-indigo-600 focus:ring-indigo-500"
                />
                <span className="text-sm text-slate-400">
                  {config.is_onvif ? 'Enabled' : 'Disabled'}
                </span>
              </label>
            </div>

            <div>
              <label className="block text-xs text-slate-500 mb-1">
                ONVIF Port
              </label>

              <input
                type="number"
                min={1}
                max={65535}
                value={config.onvif_port}
                disabled={!config.is_onvif}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    onvif_port:
                      Number(e.target.value) || 8000,
                  })
                }
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300 disabled:opacity-50 disabled:cursor-not-allowed"
              />
            </div>

          </div>
```

- [ ] **Step 6: Add warning when PTZ is ONVIF but toggle is off**

After the new grid, add:
```tsx
          {form.ptz_protocol === 'onvif' && !config.is_onvif && (
            <div className="rounded-lg border border-amber-800 bg-amber-950/30 p-3 text-sm text-amber-400">
              PTZ protocol is set to ONVIF but ONVIF is disabled. PTZ will not work.
            </div>
          )}
```

- [ ] **Step 7: Verify TypeScript compiles**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

---

### Task 7: Frontend — Update CameraDetailsDrawer to show ONVIF config status

**Files:**
- Modify: `web/src/components/cameras/CameraDetailsDrawer.tsx`

- [ ] **Step 1: Update Network tab — show actual ONVIF port and enabled/disabled**

Replace lines 519-524 (the ONVIF Port InfoRow) with:
```tsx
                  <InfoRow
                    label="ONVIF"
                    value={
                      camera?.config
                        ? (() => {
                            try {
                              const cfg = JSON.parse(camera.config);
                              return cfg.is_onvif !== false
                                ? `Port ${cfg.onvif_port || 8000}`
                                : 'Disabled';
                            } catch {
                              return 'Port 80';
                            }
                          })()
                        : 'Port 80'
                    }
                  />
```

- [ ] **Step 2: Update Diagnostics tab — show N/A when ONVIF disabled**

Replace lines 589-596 (the ONVIF InfoRow) with:
```tsx
                  <InfoRow
                    label="ONVIF"
                    value={
                      diagnostics?.onvif_enabled === false
                        ? 'N/A (disabled)'
                        : diagnostics?.onvif
                          ? 'OK'
                          : 'Failed'
                    }
                  />
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

---

### Task 8: Build and restart all affected services

- [ ] **Step 1: Rebuild all services**

```bash
docker compose --env-file /home/ubuntu/EVMS/.env -f /home/ubuntu/EVMS/deploy/docker/docker-compose.yml build camera-control ingest-service api-gateway 2>&1 | tail -5
```

Expected: All three images built successfully.

- [ ] **Step 2: Restart all services**

```bash
docker compose --env-file /home/ubuntu/EVMS/.env -f /home/ubuntu/EVMS/deploy/docker/docker-compose.yml up -d camera-control ingest-service api-gateway
```

Expected: Containers started successfully.

- [ ] **Step 3: Verify all 31 containers running**

```bash
docker compose --env-file /home/ubuntu/EVMS/.env -f /home/ubuntu/EVMS/deploy/docker/docker-compose.yml ps --services --filter "status=running" | wc -l
```

Expected: `31`

- [ ] **Step 4: Verify ingest-service logs show correct ONVIF handling**

```bash
docker compose --env-file /home/ubuntu/EVMS/.env -f /home/ubuntu/EVMS/deploy/docker/docker-compose.yml logs --tail=10 ingest-service
```

Expected: Camera starts ingesting with or without ONVIF negotiation depending on config.

- [ ] **Step 5: Commit**

```bash
git add pkg/onvif/soap.go services/camera-control/main.go services/ingest/main.go services/api-gateway/main.go web/src/api/client.ts web/src/components/cameras/CameraDialog.tsx web/src/components/cameras/CameraDetailsDrawer.tsx docs/superpowers/specs/2026-06-12-onvif-port-config-design.md
git commit -m "feat: add per-camera ONVIF toggle and port configuration"
```
