# Tier 2: Core Operations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the system daily-drivable for security teams with e-map, synchronized playback, evidence export, alarm escalation, health dashboard, and privacy masking.

**Architecture:** Leaflet.js for e-map layer; multi-camera sync via shared VirtualTimeController context; FFmpeg concat + SHA-256 for export; event-proc alarm escalation timer + webhook; common health check aggregator in gateway; FFmpeg drawbox filter for recorder-side privacy blur.

**Tech Stack:** Leaflet.js (map), FFmpeg (export/blur), Go (health, escalation), PostgreSQL/TSDB (health metrics)

---

### Task 1: E-map with camera overlay

**Files:**
- Create: `web/src/components/MapView.tsx`
- Create: `web/src/hooks/useMapCameras.ts`
- Modify: `web/src/api/client.ts`
- Modify: `web/src/components/Layout.tsx`
- Modify: `services/camera-mgmt/main.go` (add map_position to config)

- [ ] **Step 1: Install Leaflet dependencies**

```bash
cd /home/ubuntu/EVMS/web
npm install leaflet @types/leaflet
```

- [ ] **Step 2: Create useMapCameras hook**

Create `web/src/hooks/useMapCameras.ts`:

```typescript
import { useState, useEffect, useCallback } from 'react';
import { api, Camera } from '../api/client';

interface CameraMapPosition {
  cameraId: string;
  name: string;
  status: string;
  lat: number;
  lng: number;
}

export function useMapCameras(siteId?: string) {
  const [positions, setPositions] = useState<CameraMapPosition[]>([]);

  const load = useCallback(async () => {
    const cameras = await api.listCameras(siteId);
    const withPos: CameraMapPosition[] = [];
    for (const cam of cameras) {
      const pos = cam.config ? JSON.parse(cam.config).map_position : null;
      if (pos) {
        withPos.push({ cameraId: cam.id, name: cam.name, status: cam.status, lat: pos.lat, lng: pos.lng });
      }
    }
    // If no positions set, auto-place in a grid
    if (withPos.length === 0) {
      cameras.forEach((cam, i) => {
        withPos.push({
          cameraId: cam.id,
          name: cam.name,
          status: cam.status,
          lat: 40.7128 + i * 0.01,
          lng: -74.006 + i * 0.01,
        });
      });
    }
    setPositions(withPos);
  }, [siteId]);

  const savePosition = async (cameraId: string, lat: number, lng: number) => {
    await api.updateCameraConfig(cameraId, { map_position: { lat, lng } });
    await load();
  };

  useEffect(() => { load(); }, [load]);

  return { positions, savePosition, reload: load };
}
```

- [ ] **Step 3: Create MapView component**

Create `web/src/components/MapView.tsx`:

```typescript
import { useEffect, useRef } from 'react';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { CameraMapPosition } from '../hooks/useMapCameras';

interface MapViewProps {
  positions: CameraMapPosition[];
  onCameraClick: (cameraId: string) => void;
  onPositionChange: (cameraId: string, lat: number, lng: number) => void;
}

export default function MapView({ positions, onCameraClick, onPositionChange }: MapViewProps) {
  const mapRef = useRef<L.Map | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;
    mapRef.current = L.map(containerRef.current).setView([40.7128, -74.006], 13);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '&copy; OpenStreetMap contributors',
    }).addTo(mapRef.current);
  }, []);

  useEffect(() => {
    if (!mapRef.current) return;
    const map = mapRef.current;
    map.eachLayer(layer => { if (layer instanceof L.Marker) layer.remove(); });

    positions.forEach(pos => {
      const color = pos.status === 'online' ? 'green' : pos.status === 'error' ? 'red' : 'gray';
      const icon = L.divIcon({
        className: '',
        html: `<div style="width:16px;height:16px;border-radius:50%;background:${color};border:2px solid white;cursor:pointer"></div>`,
        iconSize: [16, 16],
        iconAnchor: [8, 8],
      });
      const marker = L.marker([pos.lat, pos.lng], { icon, draggable: true })
        .addTo(map)
        .bindPopup(`<b>${pos.name}</b><br/>Status: ${pos.status}`);

      marker.on('click', () => onCameraClick(pos.cameraId));
      marker.on('dragend', () => {
        const ll = marker.getLatLng();
        onPositionChange(pos.cameraId, ll.lat, ll.lng);
      });
    });
  }, [positions, onCameraClick, onPositionChange]);

  return <div ref={containerRef} className="w-full h-full rounded" />;
}
```

- [ ] **Step 4: Add map position update API method**

In `web/src/api/client.ts`:

```typescript
async updateCameraConfig(cameraId: string, config: Record<string, unknown>): Promise<void> {
  await this.fetch(`/api/cameras/${cameraId}/config`, {
    method: 'PUT',
    body: JSON.stringify({ config }),
  });
}
```

- [ ] **Step 5: Add camera config update endpoint in api-gateway**

In `services/api-gateway/main.go`:

```go
func (g *Gateway) handleUpdateCameraConfig(w http.ResponseWriter, r *http.Request) {
    cameraID := extractParam(r.URL.Path, "/api/cameras/")
    cameraID = strings.TrimSuffix(cameraID, "/config")

    var req struct {
        Config string `json:"config"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        jsonError(w, "invalid request", http.StatusBadRequest)
        return
    }

    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    _, err := g.cameraSvc.UpdateCamera(ctx, &damv1.UpdateCameraRequest{
        Id:     cameraID,
        Config: req.Config,
    })
    if err != nil {
        jsonError(w, "failed to update config", http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}
```

And route:

```go
case strings.HasPrefix(path, "/api/cameras/") && strings.HasSuffix(path, "/config") && r.Method == http.MethodPut:
    g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(g.handleUpdateCameraConfig))(w, r)
```

- [ ] **Step 6: Add Map nav item to Layout**

In `web/src/components/Layout.tsx`, add a Map entry:

```typescript
<a href="/map" className="flex items-center gap-2 p-2 hover:bg-gray-700 rounded">
  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7" />
  </svg>
  <span>Map</span>
</a>
```

- [ ] **Step 7: Create MapPage**

Create `web/src/pages/MapPage.tsx`:

```typescript
import { useNavigate } from 'react-router-dom';
import MapView from '../components/MapView';
import { useMapCameras } from '../hooks/useMapCameras';
import { useAuth } from '../context/AuthContext';

export default function MapPage() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const { positions, savePosition } = useMapCameras();

  const canEdit = user?.role === 'admin' || user?.role === 'operator';

  return (
    <div className="h-full p-4">
      <h1 className="text-xl font-bold mb-4">Camera Map</h1>
      <div className="h-[calc(100vh-8rem)]">
        <MapView
          positions={positions}
          onCameraClick={(id) => navigate(`/dashboard?camera=${id}`)}
          onPositionChange={canEdit ? savePosition : () => {}}
        />
      </div>
    </div>
  );
}
```

Add route in `main.tsx`:

```typescript
<Route path="/map" element={<MapPage />} />
```

- [ ] **Step 8: Validate**

Run: `npx tsc --noEmit` in `web/` and `gofmt -d services/api-gateway/main.go`

- [ ] **Step 9: Commit**

```bash
git add -A && git commit -m "feat: e-map with Leaflet, draggable camera markers, status colors, config persistence"
```

---

### Task 2: Synchronized multi-camera playback

**Files:**
- Create: `web/src/hooks/useSyncPlayback.ts`
- Create: `web/src/components/SyncPlaybackView.tsx`
- Modify: `web/src/pages/RecordingsPage.tsx`

- [ ] **Step 1: Create useSyncPlayback hook**

Create `web/src/hooks/useSyncPlayback.ts`:

```typescript
import { useState, useCallback, useRef, useEffect } from 'react';

interface SyncState {
  playing: boolean;
  speed: number;
  currentTime: number; // epoch ms
}

export function useSyncPlayback() {
  const [state, setState] = useState<SyncState>({ playing: false, speed: 1, currentTime: Date.now() });
  const listenersRef = useRef<Set<(s: SyncState) => void>>(new Set());
  const intervalRef = useRef<number | null>(null);

  const subscribe = useCallback((listener: (s: SyncState) => void) => {
    listenersRef.current.add(listener);
    return () => listenersRef.current.delete(listener);
  }, []);

  const broadcast = useCallback((newState: SyncState) => {
    listenersRef.current.forEach(l => l(newState));
  }, []);

  const play = useCallback((startTime: number) => {
    setState({ playing: true, speed: 1, currentTime: startTime });
  }, []);

  const pause = useCallback(() => {
    setState(s => ({ ...s, playing: false }));
  }, []);

  const seek = useCallback((t: number) => {
    setState(s => ({ ...s, currentTime: t }));
  }, []);

  const setSpeed = useCallback((speed: number) => {
    setState(s => ({ ...s, speed }));
  }, []);

  useEffect(() => {
    if (!state.playing) {
      if (intervalRef.current) clearInterval(intervalRef.current);
      return;
    }
    intervalRef.current = window.setInterval(() => {
      setState(s => {
        const next = { ...s, currentTime: s.currentTime + 1000 / 30 * s.speed };
        broadcast(next);
        return next;
      });
    }, 33); // ~30fps
    return () => { if (intervalRef.current) clearInterval(intervalRef.current); };
  }, [state.playing, state.speed, broadcast]);

  return { state, play, pause, seek, setSpeed, subscribe };
}
```

- [ ] **Step 2: Create SyncPlaybackView component**

Create `web/src/components/SyncPlaybackView.tsx`:

```typescript
import { useEffect, useRef } from 'react';
import { useSyncPlayback } from '../hooks/useSyncPlayback';

interface SyncPlaybackViewProps {
  cameraId: string;
  cameraName: string;
  substreamUrl?: string;
  sync: ReturnType<typeof useSyncPlayback>;
}

export default function SyncPlaybackView({ cameraId, cameraName, substreamUrl, sync }: SyncPlaybackViewProps) {
  const videoRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    return sync.subscribe((s) => {
      if (!videoRef.current) return;
      if (!s.playing) {
        videoRef.current.pause();
      }
      // Seek by time range — relies on the playback service supporting ?start= param
      const timeParam = `?start=${s.currentTime}`;
      if (videoRef.current.dataset.timeParam !== timeParam) {
        videoRef.current.dataset.timeParam = timeParam;
      }
    });
  }, [sync]);

  return (
    <div className="border border-gray-700 rounded overflow-hidden">
      <div className="text-xs text-gray-400 px-2 py-1 bg-gray-800">{cameraName}</div>
      <video ref={videoRef} className="w-full aspect-video bg-black" controls={false} autoPlay muted />
    </div>
  );
}
```

- [ ] **Step 3: Add sync playback page to RecordingsPage**

In `web/src/pages/RecordingsPage.tsx`, add a multi-select mode:

```typescript
const [selectedCameras, setSelectedCameras] = useState<string[]>([]);
const sync = useSyncPlayback();

const toggleCamera = (id: string) => {
  setSelectedCameras(prev =>
    prev.includes(id) ? prev.filter(c => c !== id) : [...prev, id]
  );
};

// If selectedCameras.length > 1, show sync controls and grid
{selectedCameras.length > 1 && (
  <div className="mb-4 flex gap-2 items-center">
    <button onClick={() => sync.play(Date.now() - 60000)} className="bg-blue-600 px-3 py-1 rounded text-sm">
      ▶ Play
    </button>
    <button onClick={sync.pause} className="bg-gray-600 px-3 py-1 rounded text-sm">
      ⏸ Pause
    </button>
    <input type="range" min={-86400000} max={0} value={Date.now() - sync.state.currentTime}
           onChange={e => sync.seek(Date.now() - Number(e.target.value))}
           className="flex-1" />
    <span className="text-sm text-gray-400">{Math.round(sync.state.speed * 100)}%</span>
  </div>
)}
<div className="grid grid-cols-2 gap-2">
  {selectedCameras.map(id => (
    <SyncPlaybackView key={id} cameraId={id} cameraName={id} sync={sync} />
  ))}
</div>
```

- [ ] **Step 4: Validate**

Run: `npx tsc --noEmit` in `web/`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: synchronized multi-camera playback with shared timeline"
```

---

### Task 3: Evidence export with SHA-256 hash chain

**Files:**
- Create: `services/export/main.go`
- Modify: `services/api-gateway/main.go` (proxy route)
- Modify: `deploy/docker/docker-compose.yml`
- Create: `services/export/Dockerfile`
- Modify: `web/src/pages/RecordingsPage.tsx`

- [ ] **Step 1: Create export service**

Create `services/export/main.go`:

```go
package main

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "io"
    "log/slog"
    "net/http"
    "os"
    "os/exec"
    "os/signal"
    "path/filepath"
    "syscall"
    "time"

    "github.com/dam-vms/dam/pkg/common"
)

type ExportRequest struct {
    CameraID    string   `json:"camera_id"`
    StartTime   string   `json:"start_time"`
    EndTime     string   `json:"end_time"`
    Watermark   bool     `json:"watermark"`
    RequestedBy string   `json:"requested_by"`
}

type ExportResult struct {
    FilePath string `json:"file_path"`
    Checksum string `json:"sha256"`
    Size     int64  `json:"size_bytes"`
}

func handleExport(w http.ResponseWriter, r *http.Request) {
    var req ExportRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        jsonError(w, "invalid request", http.StatusBadRequest)
        return
    }

    // Find recording segments
    segments, err := findSegments(req.CameraID, req.StartTime, req.EndTime)
    if err != nil {
        jsonError(w, "failed to find segments", http.StatusInternalServerError)
        return
    }
    if len(segments) == 0 {
        jsonError(w, "no recordings found", http.StatusNotFound)
        return
    }

    // Concat with FFmpeg
    outputPath := filepath.Join("/exports", fmt.Sprintf("export_%s_%s.mp4", req.CameraID, time.Now().Format("20060102150405")))
    args := []string{"-y"}
    for _, seg := range segments {
        args = append(args, "-i", seg)
    }
    filter := fmt.Sprintf("concat=%d", len(segments))
    if req.Watermark {
        filter += ",drawtext=text='%{localtime} | Camera: " + req.CameraID + "':fontsize=24:fontcolor=white:x=10:y=10"
    }
    args = append(args, "-filter_complex", filter, "-c:v", "libx264", "-preset", "fast", outputPath)

    cmd := exec.CommandContext(r.Context(), "ffmpeg", args...)
    var stderr bytes.Buffer
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        slog.Error("ffmpeg export failed", "error", err, "stderr", stderr.String())
        jsonError(w, "export failed", http.StatusInternalServerError)
        return
    }

    // SHA-256
    f, err := os.Open(outputPath)
    if err != nil {
        jsonError(w, "failed to read export", http.StatusInternalServerError)
        return
    }
    defer f.Close()
    h := sha256.New()
    size, _ := io.Copy(h, f)
    checksum := fmt.Sprintf("%x", h.Sum(nil))

    json.NewEncoder(w).Encode(ExportResult{
        FilePath: outputPath,
        Checksum: checksum,
        Size:     size,
    })
}

func findSegments(cameraID, start, end string) ([]string, error) {
    // Scan recordings directory for matching segments
    dir := fmt.Sprintf("/recordings/%s", cameraID)
    entries, err := os.ReadDir(dir)
    if err != nil {
        return nil, err
    }
    var segments []string
    for _, e := range entries {
        if filepath.Ext(e.Name()) == ".mp4" {
            segments = append(segments, filepath.Join(dir, e.Name()))
        }
    }
    return segments, nil
}

func jsonError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))

    mux := http.NewServeMux()
    mux.HandleFunc("/export", handleExport)

    server := &http.Server{
        Addr:         ":8094",
        Handler:      mux,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 0, // no timeout — exports can be long
        IdleTimeout:  60 * time.Second,
    }

    go func() {
        logger.Info("Export service listening", "addr", ":8094")
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("server error", "error", err)
        }
    }()

    <-ctx.Done()
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    server.Shutdown(shutdownCtx)
}
```

- [ ] **Step 2: Create Dockerfile**

Create `services/export/Dockerfile`:

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /export services/export/main.go

FROM alpine:3.19
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=builder /export /usr/local/bin/export
VOLUME ["/recordings", "/exports"]
EXPOSE 8094
CMD ["export"]
```

- [ ] **Step 3: Add export proxy route in gateway**

In `services/api-gateway/main.go`, add to config:

```go
ExportURL: common.GetEnv("EXPORT_URL", "http://export:8094"),
```

Add proxy and route:

```go
case strings.HasPrefix(path, "/api/export"):
    g.rateLimiter.rateLimitMiddleware(g.requireRole("operator")(func(w http.ResponseWriter, r *http.Request) {
        r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
        g.exportProxy.ServeHTTP(w, r)
    }))(w, r)
```

- [ ] **Step 4: Add export button to RecordingsPage**

In `web/src/pages/RecordingsPage.tsx`:

```typescript
const [exporting, setExporting] = useState(false);

const handleExport = async () => {
  setExporting(true);
  try {
    const resp = await api.exportRecording(cameraId, startTime, endTime, true);
    // Show download link
    alert(`Export complete: ${resp.checksum}`);
  } finally {
    setExporting(false);
  }
};

<button onClick={handleExport} disabled={exporting}
        className="bg-green-700 px-3 py-1 rounded text-sm">
  {exporting ? 'Exporting...' : 'Export with SHA-256'}
</button>
```

In `web/src/api/client.ts`:

```typescript
async exportRecording(cameraId: string, startTime: string, endTime: string, watermark: boolean): Promise<{file_path: string; sha256: string; size_bytes: number}> {
  const resp = await this.fetch('/api/export', {
    method: 'POST',
    body: JSON.stringify({ camera_id: cameraId, start_time: startTime, end_time: endTime, watermark }),
  });
  return resp.json();
}
```

- [ ] **Step 5: Validate**

Run: `gofmt -d services/export/main.go` and `npx tsc --noEmit`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: evidence export service with FFmpeg concat, timestamp watermark, SHA-256 hash"
```

---

### Task 4: Alarm acknowledge/escalation workflow

**Files:**
- Modify: `services/event-proc/main.go`
- Modify: `web/src/pages/EventsPage.tsx`
- Modify: `web/src/api/client.ts`
- Create: `services/event-proc/alert_workflow.go`

- [ ] **Step 1: Create alert workflow manager**

Create `services/event-proc/alert_workflow.go`:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
    "sync"
    "time"
)

type AlertStatus string

const (
    AlertTriggered  AlertStatus = "triggered"
    AlertAcknowledged AlertStatus = "acknowledged"
    AlertEscalated   AlertStatus = "escalated"
    AlertResolved    AlertStatus = "resolved"
)

type Alert struct {
    ID         string      `json:"id"`
    RuleID     string      `json:"rule_id"`
    CameraID   string      `json:"camera_id"`
    Message    string      `json:"message"`
    Status     AlertStatus `json:"status"`
    CreatedAt  time.Time   `json:"created_at"`
    AckedBy    string      `json:"acked_by,omitempty"`
    AckedAt    *time.Time  `json:"acked_at,omitempty"`
    Escalated  bool        `json:"escalated"`
    EscalationWebhook string `json:"escalation_webhook,omitempty"`
}

type AlertWorkflowManager struct {
    mu      sync.RWMutex
    alerts  map[string]*Alert
    config  AlertWorkflowConfig
    logger  *slog.Logger
}

type AlertWorkflowConfig struct {
    EscalationTimeout time.Duration `json:"escalation_timeout"` // default 5min
    EscalationWebhook string        `json:"escalation_webhook"`
    CheckInterval     time.Duration `json:"check_interval"` // default 30s
}

func NewAlertWorkflowManager(cfg AlertWorkflowConfig, logger *slog.Logger) *AlertWorkflowManager {
    if cfg.EscalationTimeout == 0 {
        cfg.EscalationTimeout = 5 * time.Minute
    }
    if cfg.CheckInterval == 0 {
        cfg.CheckInterval = 30 * time.Second
    }
    m := &AlertWorkflowManager{
        alerts: make(map[string]*Alert),
        config: cfg,
        logger: logger,
    }
    go m.escalationLoop()
    return m
}

func (m *AlertWorkflowManager) CreateAlert(ruleID, cameraID, message string) *Alert {
    m.mu.Lock()
    defer m.mu.Unlock()
    alert := &Alert{
        ID:        fmt.Sprintf("alert_%d", time.Now().UnixNano()),
        RuleID:    ruleID,
        CameraID:  cameraID,
        Message:   message,
        Status:    AlertTriggered,
        CreatedAt: time.Now(),
    }
    m.alerts[alert.ID] = alert
    m.logger.Info("Alert created", "id", alert.ID, "camera", cameraID)
    return alert
}

func (m *AlertWorkflowManager) Acknowledge(id, username string) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    alert, ok := m.alerts[id]
    if !ok {
        return fmt.Errorf("alert not found")
    }
    now := time.Now()
    alert.Status = AlertAcknowledged
    alert.AckedBy = username
    alert.AckedAt = &now
    m.logger.Info("Alert acknowledged", "id", id, "by", username)
    return nil
}

func (m *AlertWorkflowManager) escalationLoop() {
    ticker := time.NewTicker(m.config.CheckInterval)
    for range ticker.C {
        m.mu.Lock()
        now := time.Now()
        for _, alert := range m.alerts {
            if alert.Status == AlertTriggered && now.Sub(alert.CreatedAt) > m.config.EscalationTimeout {
                alert.Status = AlertEscalated
                alert.Escalated = true
                m.logger.Warn("Alert escalated", "id", alert.ID)
                if m.config.EscalationWebhook != "" {
                    go m.fireEscalationWebhook(alert)
                }
            }
        }
        m.mu.Unlock()
    }
}

func (m *AlertWorkflowManager) fireEscalationWebhook(alert *Alert) {
    body, _ := json.Marshal(alert)
    resp, err := http.Post(m.config.EscalationWebhook, "application/json", bytes.NewReader(body))
    if err != nil {
        m.logger.Error("Escalation webhook failed", "error", err)
        return
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
}

func (m *AlertWorkflowManager) HandleHTTP(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        m.handleListAlerts(w, r)
    case http.MethodPost:
        m.handleAcknowledgeAlert(w, r)
    default:
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    }
}

func (m *AlertWorkflowManager) handleListAlerts(w http.ResponseWriter, r *http.Request) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    alerts := make([]*Alert, 0, len(m.alerts))
    for _, a := range m.alerts {
        alerts = append(alerts, a)
    }
    json.NewEncoder(w).Encode(map[string]interface{}{"alerts": alerts})
}

func (m *AlertWorkflowManager) handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
    var req struct {
        ID       string `json:"id"`
        Username string `json:"username"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        jsonError(w, "invalid request", http.StatusBadRequest)
        return
    }
    if err := m.Acknowledge(req.ID, req.Username); err != nil {
        jsonError(w, err.Error(), http.StatusNotFound)
        return
    }
    json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
}
```

- [ ] **Step 2: Wire into event-proc main**

In `services/event-proc/main.go`:

```go
// Add after creating AlertRuleManager
alertWorkflow := NewAlertWorkflowManager(AlertWorkflowConfig{
    EscalationTimeout: 5 * time.Minute,
    EscalationWebhook: os.Getenv("ESCALATION_WEBHOOK"),
}, logger)

// Add HTTP routes
mux.HandleFunc("/api/alerts", alertWorkflow.HandleHTTP)

// When rule matches, create alert
if ruleMatches {
    alertWorkflow.CreateAlert(rule.ID, cameraID, fmt.Sprintf("Rule '%s' triggered on %s", rule.Name, cameraID))
}
```

- [ ] **Step 3: Add alert ack UI to EventsPage**

In `web/src/pages/EventsPage.tsx`:

```typescript
interface Alert {
  id: string;
  rule_id: string;
  camera_id: string;
  message: string;
  status: 'triggered' | 'acknowledged' | 'escalated' | 'resolved';
  created_at: string;
}

const [alerts, setAlerts] = useState<Alert[]>([]);

useEffect(() => {
  api.listAlerts().then(setAlerts).catch(() => {});
  const interval = setInterval(() => {
    api.listAlerts().then(setAlerts).catch(() => {});
  }, 10000); // poll every 10s
  return () => clearInterval(interval);
}, []);

const handleAck = async (id: string) => {
  await api.acknowledgeAlert(id);
  setAlerts(prev => prev.map(a => a.id === id ? { ...a, status: 'acknowledged' as const } : a));
};

// Render alerts table
{alerts.map(alert => (
  <tr key={alert.id} className={alert.status === 'escalated' ? 'bg-red-900/50' : ''}>
    <td className="p-2">{alert.message}</td>
    <td className="p-2">
      <span className={`px-2 py-0.5 rounded text-xs ${
        alert.status === 'triggered' ? 'bg-yellow-600' :
        alert.status === 'escalated' ? 'bg-red-600 animate-pulse' :
        'bg-green-600'
      }`}>{alert.status}</span>
    </td>
    <td className="p-2">{new Date(alert.created_at).toLocaleString()}</td>
    <td className="p-2">
      {alert.status === 'triggered' && (
        <button onClick={() => handleAck(alert.id)} className="bg-blue-600 px-2 py-1 rounded text-xs">
          Acknowledge
        </button>
      )}
    </td>
  </tr>
))}
```

- [ ] **Step 4: Add API methods**

In `web/src/api/client.ts`:

```typescript
async listAlerts(): Promise<Alert[]> {
  const resp = await this.fetch('/api/alerts');
  const data = await resp.json();
  return data.alerts;
}

async acknowledgeAlert(id: string): Promise<void> {
  await this.fetch('/api/alerts', {
    method: 'POST',
    body: JSON.stringify({ id, username: this.getUsername() }),
  });
}
```

- [ ] **Step 5: Validate**

Run: `gofmt -d services/event-proc/alert_workflow.go` and `npx tsc --noEmit`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: alarm acknowledge/escalation workflow with configurable timeout and webhook"
```

---

### Task 5: Health dashboard

**Files:**
- Modify: `pkg/common/metrics.go`
- Create: `web/src/pages/HealthPage.tsx`
- Modify: `web/src/components/Layout.tsx`
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Add dependency-aware health endpoint to common**

Modify `pkg/common/metrics.go`:

```go
package common

import (
    "database/sql"
    "encoding/json"
    "net/http"
    "time"

    "github.com/nats-io/nats.go"
)

type HealthCheck struct {
    Status    string            `json:"status"`
    Timestamp time.Time         `json:"timestamp"`
    Checks    map[string]string `json:"checks"`
}

func HealthHandler(db *sql.DB, nc *nats.Conn) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        checks := make(map[string]string)
        overall := "ok"

        if db != nil {
            if err := db.Ping(); err != nil {
                checks["db"] = "unreachable: " + err.Error()
                overall = "degraded"
            } else {
                checks["db"] = "ok"
            }
        }

        if nc != nil {
            if !nc.IsConnected() {
                checks["nats"] = "disconnected"
                if overall == "ok" {
                    overall = "degraded"
                }
            } else {
                checks["nats"] = "ok"
            }
        }

        // Check explicit dependencies from query params
        deps := r.URL.Query()["check"]
        for _, dep := range deps {
            if _, exists := checks[dep]; !exists {
                checks[dep] = "unknown"
            }
        }

        w.Header().Set("Content-Type", "application/json")
        if overall != "ok" {
            w.WriteHeader(http.StatusServiceUnavailable)
        }
        json.NewEncoder(w).Encode(HealthCheck{
            Status:    overall,
            Timestamp: time.Now(),
            Checks:    checks,
        })
    }
}
```

- [ ] **Step 2: Create HealthPage**

Create `web/src/pages/HealthPage.tsx`:

```typescript
import { useState, useEffect } from 'react';

interface ServiceHealth {
  name: string;
  url: string;
  status: string;
  checks?: Record<string, string>;
}

const SERVICES: ServiceHealth[] = [
  { name: 'API Gateway', url: '/api/health' },
  { name: 'Auth', url: '/api/health' }, // proxied
  // These run on different ports, so gateway aggregates or they're fetched directly
];

export default function HealthPage() {
  const [results, setResults] = useState<Record<string, {status: string; checks?: Record<string, string>}>>({});

  useEffect(() => {
    // Poll the gateway health endpoint which aggregates service checks
    const check = async () => {
      try {
        const resp = await fetch('/api/health?check=db,nats,storage');
        const data = await resp.json();
        setResults(prev => ({ ...prev, gateway: data }));
      } catch (err) {
        setResults(prev => ({ ...prev, gateway: { status: 'error' } }));
      }
    };
    check();
    const interval = setInterval(check, 15000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="p-4">
      <h1 className="text-xl font-bold mb-4">System Health</h1>
      <div className="grid gap-4">
        {Object.entries(results).map(([service, data]) => (
          <div key={service} className="bg-gray-800 p-4 rounded">
            <div className="flex items-center gap-2">
              <span className={`w-3 h-3 rounded-full ${data.status === 'ok' ? 'bg-green-500' : 'bg-red-500'}`} />
              <span className="font-medium">{service}</span>
              <span className="text-sm text-gray-400">{data.status}</span>
            </div>
            {data.checks && (
              <div className="mt-2 ml-5 space-y-1">
                {Object.entries(data.checks).map(([check, status]) => (
                  <div key={check} className="flex gap-2 text-sm">
                    <span className="w-20 text-gray-400">{check}:</span>
                    <span className={status === 'ok' ? 'text-green-400' : 'text-red-400'}>{status}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Add Health nav item and route**

In `web/src/components/Layout.tsx`, add Health nav item. In `main.tsx`:

```typescript
<Route path="/health" element={<HealthPage />} />
```

- [ ] **Step 4: Validate**

Run: `gofmt -d pkg/common/metrics.go` and `npx tsc --noEmit`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: system health dashboard with dependency-aware checks and 15s polling"
```

---

### Task 6: Privacy masking (blur regions in recording)

**Files:**
- Modify: `services/recorder/main.go`
- Modify: `web/src/pages/SettingsPage.tsx`
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Add privacy mask config to camera proto**

In `api/v1/camera.pb.go`, add to Camera struct or Config:

```go
type CameraConfig struct {
    MapPosition   *MapPosition   `json:"map_position,omitempty"`
    PrivacyMasks  []PrivacyMask  `json:"privacy_masks,omitempty"`
}

type PrivacyMask struct {
    Label  string  `json:"label"`
    Points [][2]float64 `json:"points"` // normalized 0-1 coords, polygon
}
```

- [ ] **Step 2: Add blur filtering to recorder segment writer**

In `services/recorder/main.go`, before encoding, apply FFmpeg drawbox filter:

```go
func buildPrivacyFilter(cameraID string) string {
    // Read masks from camera config
    masks := getPrivacyMasks(cameraID)
    if len(masks) == 0 {
        return ""
    }
    var filters []string
    for _, m := range masks {
        for _, p := range m.Points {
            // drawbox: x, y, width, height
            filters = append(filters,
                fmt.Sprintf("drawbox=x=%f*iw:y=%f*ih:w=%f*iw:h=%f*ih:color=black@0.8:t=fill",
                    p[0], p[1], p[2]-p[0], p[3]-p[1]))
        }
    }
    return strings.Join(filters, ",")
}
```

- [ ] **Step 3: Add privacy mask editor to SettingsPage**

In `web/src/pages/SettingsPage.tsx`:

```typescript
const [privacyMasks, setPrivacyMasks] = useState<{points: number[][]; label: string}[]>([]);

const addMask = () => {
  setPrivacyMasks(prev => [...prev, { points: [[0.3, 0.3, 0.5, 0.5]], label: `Mask ${prev.length + 1}` }]);
};

const saveMasks = async () => {
  await api.updateCameraConfig(cameraId, { privacy_masks: privacyMasks });
};

// Render mask list with edit UI
{privacyMasks.map((mask, i) => (
  <div key={i} className="bg-gray-700 p-2 rounded">
    <input value={mask.label} onChange={e => {
      const updated = [...privacyMasks];
      updated[i].label = e.target.value;
      setPrivacyMasks(updated);
    }} className="bg-gray-600 px-2 py-1 rounded text-sm" />
    <span className="text-xs text-gray-400 ml-2">
      [{mask.points.map(p => `(${p.join(',')})`).join(' ')}]
    </span>
  </div>
))}
<button onClick={addMask} className="mt-2 bg-blue-600 px-2 py-1 rounded text-sm">Add Mask</button>
<button onClick={saveMasks} className="mt-2 ml-2 bg-green-600 px-2 py-1 rounded text-sm">Save Masks</button>
```

- [ ] **Step 4: Validate**

Run: `gofmt -d services/recorder/main.go` and `npx tsc --noEmit`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: privacy masking with polygon blur regions, per-camera config, SettingsPage editor"
```
