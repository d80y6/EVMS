# Tier 5: Nice-to-Have Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add polish features that elevate perception — tour/patrol sequences, facial recognition with watchlist, lens dewarping, POS overlay, crowd density heatmaps, PWA mobile support, blueprint floor plans, storage planning dashboard.

**Architecture:** Tour scheduler as a lightweight goroutine in event-proc; facial recognition via AWS Rekognition or DeepStack in ai-worker; fisheye dewarping via FFmpeg `lenscorrection` filter; POS events via webhook ingestion into ai_events; heatmap aggregation in TSDB; PWA via vite-plugin-pwa; canvas/Leaflet for floor plans.

**Tech Stack:** Go, React/TypeScript, AWS Rekognition (facial), FFmpeg (dewarp), PWA workbox, Leaflet (floor plans), TimescaleDB (heatmaps)

---

### Task 1: Tour/patrol sequences

**Files:**
- Create: `services/event-proc/tour.go`
- Modify: `web/src/pages/SettingsPage.tsx`
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Create tour scheduler**

Create `services/event-proc/tour.go`:

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

type TourStep struct {
    CameraID     string `json:"camera_id"`
    PresetToken  string `json:"preset_token,omitempty"`
    DwellSeconds int    `json:"dwell_seconds"`
}

type Tour struct {
    ID        string     `json:"id"`
    Name      string     `json:"name"`
    Enabled   bool       `json:"enabled"`
    Steps     []TourStep `json:"steps"`
    Interval  int        `json:"interval"` // loop interval in seconds
    CreatedAt time.Time  `json:"created_at"`
}

type TourScheduler struct {
    mu     sync.RWMutex
    tours  map[string]*Tour
    active map[string]context.CancelFunc // tourID -> cancel
    logger *slog.Logger
}

func NewTourScheduler(logger *slog.Logger) *TourScheduler {
    return &TourScheduler{
        tours:  make(map[string]*Tour),
        active: make(map[string]context.CancelFunc),
        logger: logger,
    }
}

func (ts *TourScheduler) AddTour(tour *Tour) {
    ts.mu.Lock()
    defer ts.mu.Unlock()
    ts.tours[tour.ID] = tour
}

func (ts *TourScheduler) RemoveTour(id string) {
    ts.mu.Lock()
    defer ts.mu.Unlock()
    if cancel, ok := ts.active[id]; ok {
        cancel()
    }
    delete(ts.tours, id)
    delete(ts.active, id)
}

func (ts *TourScheduler) StartTour(id string) error {
    ts.mu.RLock()
    tour, ok := ts.tours[id]
    ts.mu.RUnlock()
    if !ok {
        return fmt.Errorf("tour not found")
    }

    ctx, cancel := context.WithCancel(context.Background())
    ts.mu.Lock()
    if oldCancel, ok := ts.active[id]; ok {
        oldCancel()
    }
    ts.active[id] = cancel
    ts.mu.Unlock()

    go ts.runTour(ctx, tour)
    return nil
}

func (ts *TourScheduler) runTour(ctx context.Context, tour *Tour) {
    ticker := time.NewTicker(time.Duration(tour.Interval) * time.Second)
    defer ticker.Stop()

    stepIdx := 0
    for {
        select {
        case <-ticker.C:
            if len(tour.Steps) == 0 {
                continue
            }
            step := tour.Steps[stepIdx]
            ts.logger.Info("tour step", "tour", tour.Name, "camera", step.CameraID,
                "preset", step.PresetToken, "dwell", step.DwellSeconds)

            // Call PTZ goto preset via camera-control
            if step.PresetToken != "" {
                http.Post(fmt.Sprintf("http://camera-control:8088/api/cameras/%s/ptz/preset/%s/goto",
                    step.CameraID, step.PresetToken), "application/json", nil)
            }

            stepIdx = (stepIdx + 1) % len(tour.Steps)

        case <-ctx.Done():
            return
        }
    }
}

func (ts *TourScheduler) HandleHTTP(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        ts.mu.RLock()
        tours := make([]*Tour, 0, len(ts.tours))
        for _, t := range ts.tours {
            tours = append(tours, t)
        }
        ts.mu.RUnlock()
        json.NewEncoder(w).Encode(map[string]interface{}{"tours": tours})

    case http.MethodPost:
        var tour Tour
        if err := json.NewDecoder(r.Body).Decode(&tour); err != nil {
            jsonError(w, "invalid tour", http.StatusBadRequest)
            return
        }
        ts.AddTour(&tour)
        json.NewEncoder(w).Encode(map[string]string{"status": "created", "id": tour.ID})

    case http.MethodDelete:
        id := r.URL.Query().Get("id")
        ts.RemoveTour(id)
        json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
    }
}
```

- [ ] **Step 2: Add tour HTTP routes to event-proc**

Register `tourScheduler.HandleHTTP` at `/api/tours` in event-proc admin server.

Add gateway route:

```go
case strings.HasPrefix(path, "/api/tours"):
    // Proxy to event-proc admin
    r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
    reverseProxyTo("event-proc:8093").ServeHTTP(w, r)
```

- [ ] **Step 3: Add tour UI to SettingsPage**

```typescript
interface Tour {
  id: string;
  name: string;
  enabled: boolean;
  steps: {camera_id: string; preset_token?: string; dwell_seconds: number}[];
  interval: number;
}

const [tours, setTours] = useState<Tour[]>([]);
const [editingTour, setEditingTour] = useState<Tour | null>(null);

{tours.map(tour => (
  <div key={tour.id} className="bg-gray-800 p-3 rounded flex justify-between items-center">
    <div>
      <span className="font-medium">{tour.name}</span>
      <span className="text-xs text-gray-400 ml-2">{tour.steps.length} steps, {tour.interval}s loop</span>
    </div>
    <button onClick={() => setEditingTour(tour)} className="bg-blue-600 px-2 py-1 rounded text-xs">Edit</button>
    <button onClick={() => api.startTour(tour.id)} className="bg-green-600 px-2 py-1 rounded text-xs ml-1">▶ Start</button>
  </div>
))}
```

- [ ] **Step 4: Validate**

Run: `gofmt -d services/event-proc/tour.go`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: PTZ tour scheduler with preset step sequences and HTTP API"
```

---

### Task 2: Facial recognition with watchlist

**Files:**
- Modify: `services/ai-worker/main.go`
- Create: `services/ai-worker/facial.go`
- Modify: `web/src/pages/SearchPage.tsx`
- Create: `migrations/005_facial_whitelist.sql`

- [ ] **Step 1: Create facial recognition worker**

Create `services/ai-worker/facial.go`:

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "image"
    "image/jpeg"
    "net/http"
    "os"
    "path/filepath"
    "time"

    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/rekognition"
)

type FacialResult struct {
    FaceID      string    `json:"face_id"`
    Name        string    `json:"name,omitempty"`
    Confidence  float64   `json:"confidence"`
    Box         [4]int    `json:"box"`
    Watchlisted bool      `json:"watchlisted"`
    Timestamp   time.Time `json:"timestamp"`
}

type FacialProcessor struct {
    enabled   bool
    useAWS    bool
    awsClient *rekognition.Rekognition
    watchlist map[string]string // faceID -> name
    apiURL    string            // self-hosted facial API (DeepStack)
    logger    *slog.Logger
}

func NewFacialProcessor(logger *slog.Logger) *FacialProcessor {
    p := &FacialProcessor{
        enabled:   os.Getenv("FACIAL_ENABLED") == "true",
        useAWS:    os.Getenv("FACIAL_USE_AWS") == "true",
        watchlist: make(map[string]string),
        apiURL:    os.Getenv("FACIAL_API_URL"),
        logger:    logger,
    }

    if p.enabled && p.useAWS {
        sess := session.Must(session.NewSession())
        p.awsClient = rekognition.New(sess)
    }

    return p
}

func (p *FacialProcessor) Detect(frame image.Image) (*FacialResult, error) {
    if !p.enabled {
        return nil, nil
    }

    if p.useAWS {
        return p.detectAWS(frame)
    }
    if p.apiURL != "" {
        return p.detectSelfHosted(frame)
    }
    return nil, nil
}

func (p *FacialProcessor) detectAWS(frame image.Image) (*FacialResult, error) {
    var buf bytes.Buffer
    jpeg.Encode(&buf, frame, &jpeg.Options{Quality: 85})

    input := &rekognition.SearchFacesByImageInput{
        CollectionId: aws.String(os.Getenv("FACIAL_COLLECTION_ID")),
        Image:        &rekognition.Image{Bytes: buf.Bytes()},
    }

    result, err := p.awsClient.SearchFacesByImage(input)
    if err != nil {
        return nil, err
    }

    if len(result.FaceMatches) == 0 {
        return nil, nil
    }

    best := result.FaceMatches[0]
    name := ""
    if best.Face != nil && best.Face.ExternalImageId != nil {
        name = *best.Face.ExternalImageId
    }

    return &FacialResult{
        FaceID:      *best.Face.FaceId,
        Name:        name,
        Confidence:  float64(*best.Face.Confidence),
        Timestamp:   time.Now(),
        Watchlisted: p.isWatchlisted(*best.Face.FaceId),
    }, nil
}

func (p *FacialProcessor) detectSelfHosted(frame image.Image) (*FacialResult, error) {
    var buf bytes.Buffer
    jpeg.Encode(&buf, frame, &jpeg.Options{Quality: 85})

    resp, err := http.Post(p.apiURL+"/v1/vision/face/recognize",
        "application/octet-stream", &buf)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result struct {
        Predictions []struct {
            Confidence float64 `json:"confidence"`
            UserID     string  `json:"userid"`
            YMin       int     `json:"y_min"`
            XMin       int     `json:"x_min"`
            YMax       int     `json:"y_max"`
            XMax       int     `json:"x_max"`
        } `json:"predictions"`
    }
    json.NewDecoder(resp.Body).Decode(&result)

    if len(result.Predictions) == 0 {
        return nil, nil
    }

    best := result.Predictions[0]
    return &FacialResult{
        Name:       best.UserID,
        Confidence: best.Confidence,
        Box:        [4]int{best.XMin, best.YMin, best.XMax, best.YMax},
        Timestamp:  time.Now(),
    }, nil
}

func (p *FacialProcessor) isWatchlisted(faceID string) bool {
    _, ok := p.watchlist[faceID]
    return ok
}

func (p *FacialProcessor) AddToWatchlist(faceID, name string) {
    p.watchlist[faceID] = name
}
```

- [ ] **Step 2: Create facial watchlist migration**

Create `migrations/005_facial_whitelist.sql`:

```sql
CREATE TABLE IF NOT EXISTS face_watchlist (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    face_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    notes TEXT,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOTERTY face_detections (
    id UUID DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL,
    event_time TIMESTAMPTZ NOT NULL,
    face_id TEXT,
    name TEXT,
    confidence FLOAT,
    bounding_box JSONB,
    watchlisted BOOLEAN DEFAULT false,
    metadata JSONB
);

SELECT create_hypertable('face_detections', 'event_time', if_not_exists => true);
```

- [ ] **Step 3: Wire facial into ai-worker pipeline**

In `services/ai-worker/main.go`:

```go
facial := NewFacialProcessor(logger)

// After object detection, if person detected:
if objType == "person" && confidence > 0.7 {
    faceResult, err := facial.Detect(frame)
    if err == nil && faceResult != nil {
        // Publish facial event
        nc.Publish(fmt.Sprintf("camera.%s.facial", cameraID),
            marshal(map[string]interface{}{
                "face_id":    faceResult.FaceID,
                "name":       faceResult.Name,
                "confidence": faceResult.Confidence,
                "box":        faceResult.Box,
                "watchlisted": faceResult.Watchlisted,
            }))
    }
}
```

- [ ] **Step 4: Add face search to SearchPage**

```typescript
const [faceName, setFaceName] = useState('');

<select value={objectType} onChange={e => setObjectType(e.target.value)}>
  <option value="person">Person</option>
  <option value="face">Face</option>
  ...
</select>

{objectType === 'face' && (
  <input value={faceName} onChange={e => setFaceName(e.target.value)}
         placeholder="Search by name..." className="bg-gray-700 p-2 rounded" />
)}
```

- [ ] **Step 5: Validate**

Run: `gofmt -d services/ai-worker/facial.go`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: facial recognition with AWS Rekognition/DeepStack, watchlist alerts, face search"
```

---

### Task 3: Lens dewarping (fisheye)

**Files:**
- Modify: `services/recorder/main.go`
- Modify: `web/src/components/CameraView.tsx`
- Modify: `api/v1/camera.pb.go` (add dewarp config)

- [ ] **Step 1: Add dewarp config to camera proto**

```go
type DewarpConfig struct {
    Enabled      bool    `json:"enabled"`
    LensType     string  `json:"lens_type"` // "fisheye_360", "fisheye_180", "panoramic"
    HFOV         float64 `json:"hfov"`      // horizontal FOV in degrees
    VFOV         float64 `json:"vfov"`      // vertical FOV
    ViewMode     string  `json:"view_mode"` // "equirectangular", "perspective", "panoramic"
}
```

- [ ] **Step 2: Apply dewarp filter in recorder**

In `services/recorder/main.go`, add FFmpeg filter for fisheye:

```go
func buildDewarpFilter(cameraID string) string {
    cfg := getCameraConfig(cameraID)
    if !cfg.Dewarp.Enabled {
        return ""
    }

    // FFmpeg lenscorrection for fisheye correction
    k1 := -0.15 // radial distortion coefficient
    k2 := 0.05

    return fmt.Sprintf("lenscorrection=cx=0.5:cy=0.5:k1=%f:k2=%f", k1, k2)
}
```

Apply in segment encoding:

```go
filter := buildDewarpFilter(cameraID)
if filter != "" {
    args = append(args, "-vf", filter)
}
```

- [ ] **Step 3: Add dewarp toggle to CameraView**

```typescript
const [dewarped, setDewarped] = useState(false);

<button onClick={() => setDewarped(!dewarped)}
        className={`text-xs px-1 rounded ${dewarped ? 'bg-blue-600' : 'bg-gray-700'}`}>
  {dewarped ? 'Dewarped' : 'Fisheye'}
</button>
```

- [ ] **Step 4: Validate**

Run: `gofmt -d services/recorder/main.go`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: fisheye lens dewarping with FFmpeg lenscorrection filter and UI toggle"
```

---

### Task 4: POS / retail transaction overlay

**Files:**
- Create: `services/pos-ingest/main.go`
- Modify: `services/event-proc/main.go`
- Modify: `web/src/pages/RecordingsPage.tsx`
- Create: `migrations/006_pos_events.sql`

- [ ] **Step 1: Create POS ingestion service**

Create `services/pos-ingest/main.go`:

```go
package main

import (
    "encoding/json"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/dam-vms/dam/pkg/common"
    "github.com/nats-io/nats.go"
)

type POSTransaction struct {
    ID            string    `json:"id"`
    CameraID      string    `json:"camera_id"`
    StoreID       string    `json:"store_id"`
    RegisterID    string    `json:"register_id"`
    TransactionID string    `json:"transaction_id"`
    Timestamp     time.Time `json:"timestamp"`
    Items         []struct {
        SKU         string  `json:"sku"`
        Description string  `json:"description"`
        Quantity    int     `json:"quantity"`
        UnitPrice   float64 `json:"unit_price"`
        Total       float64 `json:"total"`
    } `json:"items"`
    Subtotal  float64 `json:"subtotal"`
    Tax       float64 `json:"tax"`
    Total     float64 `json:"total"`
    TenderType string `json:"tender_type"`
}

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    nc, _ := nats.Connect(common.GetEnv("NATS_URL", "nats://localhost:4222"),
        nats.RetryOnFailedConnect(true), nats.MaxReconnects(-1), nats.ReconnectWait(2*time.Second))
    defer nc.Close()

    http.HandleFunc("/api/pos/transaction", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        var tx POSTransaction
        if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
            jsonError(w, "invalid transaction", http.StatusBadRequest)
            return
        }

        data, _ := json.Marshal(tx)
        nc.Publish("pos.transaction", data)

        w.WriteHeader(http.StatusAccepted)
        json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
    })

    server := &http.Server{
        Addr: ":8096", Handler: nil,
        ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
    }
    go server.ListenAndServe()

    <-ctx.Done()
    server.Shutdown(context.Background())
}

func jsonError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
```

- [ ] **Step 2: Create POS events migration**

Create `migrations/006_pos_events.sql`:

```sql
CREATE TABLE IF NOT EXISTS pos_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id),
    transaction_id TEXT NOT NULL,
    total NUMERIC(10,2) NOT NULL,
    items JSONB,
    metadata JSONB,
    event_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pos_camera_time ON pos_transactions(camera_id, event_time DESC);
```

- [ ] **Step 3: Show POS overlay in RecordingsPage**

```typescript
const [posOverlay, setPosOverlay] = useState<boolean>(false);
const [currentTx, setCurrentTx] = useState<POSTransaction | null>(null);

// Subscribe to POS events matching current playback time
useEffect(() => {
  if (!posOverlay) return;
  const interval = setInterval(async () => {
    const txns = await api.getPOSTransactions(cameraId, currentTime);
    if (txns.length > 0) setCurrentTx(txns[0]);
  }, 5000);
  return () => clearInterval(interval);
}, [posOverlay, currentTime]);

// Render overlay
{posOverlay && currentTx && (
  <div className="absolute bottom-0 right-0 bg-black/80 text-white p-2 text-xs font-mono">
    <div>Trans #{currentTx.transaction_id}</div>
    {currentTx.items.map((item, i) => (
      <div key={i} className="flex justify-between gap-4">
        <span>{item.description}</span>
        <span>${item.total.toFixed(2)}</span>
      </div>
    ))}
    <div className="border-t border-gray-600 mt-1 pt-1 flex justify-between font-bold">
      <span>Total</span>
      <span>${currentTx.total.toFixed(2)}</span>
    </div>
  </div>
)}
```

- [ ] **Step 4: Validate**

Run: `gofmt -d services/pos-ingest/main.go`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: POS transaction ingestion with NATS and video overlay"
```

---

### Task 5: Crowd density heatmaps

**Files:**
- Create: `services/ai-worker/heatmap.go`
- Modify: `web/src/pages/Dashboard.tsx`
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Create heatmap aggregator**

Create `services/ai-worker/heatmap.go`:

```go
package main

import (
    "encoding/json"
    "fmt"
    "time"

    "github.com/jmoiron/sqlx"
)

type HeatmapCell struct {
    CameraID string    `json:"camera_id"`
    X        int       `json:"x"`        // grid cell column (0-19)
    Y        int       `json:"y"`        // grid cell row (0-19)
    Count    int       `json:"count"`
    Bucket   time.Time `json:"bucket"`   // hour-aligned
}

type HeatmapAggregator struct {
    db         *sqlx.DB
    gridSize   int // 20 = 20x20 grid
    cellWidth  float64
    cellHeight float64
}

func NewHeatmapAggregator(db *sqlx.DB) *HeatmapAggregator {
    return &HeatmapAggregator{
        db:         db,
        gridSize:   20,
        cellWidth:  1.0 / 20,
        cellHeight: 1.0 / 20,
    }
}

func (ha *HeatmapAggregator) RecordDetection(cameraID string, bbox [4]float64, t time.Time) {
    // bbox: [x1, y1, x2, y2] normalized 0-1
    centerX := (bbox[0] + bbox[2]) / 2
    centerY := (bbox[1] + bbox[3]) / 2

    cellX := int(centerX / ha.cellWidth)
    cellY := int(centerY / ha.cellHeight)

    if cellX >= ha.gridSize {
        cellX = ha.gridSize - 1
    }
    if cellY >= ha.gridSize {
        cellY = ha.gridSize - 1
    }

    bucket := t.Truncate(1 * time.Hour)

    ha.db.Exec(
        `INSERT INTO crowd_heatmaps (camera_id, cell_x, cell_y, bucket, count)
         VALUES ($1, $2, $3, $4, 1)
         ON CONFLICT (camera_id, cell_x, cell_y, bucket)
         DO UPDATE SET count = crowd_heatmaps.count + 1`,
        cameraID, cellX, cellY, bucket)
}

func (ha *HeatmapAggregator) GetHeatmap(cameraID string, start, end time.Time) ([]HeatmapCell, error) {
    var cells []HeatmapCell
    err := ha.db.Select(&cells,
        `SELECT camera_id, cell_x, cell_y, SUM(count) as count, bucket
         FROM crowd_heatmaps
         WHERE camera_id=$1 AND bucket BETWEEN $2 AND $3
         GROUP BY camera_id, cell_x, cell_y, bucket
         ORDER BY bucket`,
        cameraID, start, end)
    return cells, err
}
```

- [ ] **Step 2: Create heatmap migration**

Append to `migrations/002` or create separate:

```sql
CREATE TABLE IF NOT EXISTS crowd_heatmaps (
    camera_id UUID NOT NULL,
    cell_x INT NOT NULL,
    cell_y INT NOT NULL,
    bucket TIMESTAMPTZ NOT NULL,
    count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (camera_id, cell_x, cell_y, bucket)
);

SELECT create_hypertable('crowd_heatmaps', 'bucket', if_not_exists => true);
```

- [ ] **Step 3: Render heatmap overlay on CameraView**

```typescript
interface HeatmapCell {
  x: number;
  y: number;
  count: number;
  camera_id: string;
}

const [heatmap, setHeatmap] = useState<boolean>(false);
const [heatmapData, setHeatmapData] = useState<HeatmapCell[]>([]);

useEffect(() => {
  if (!heatmap) return;
  api.getHeatmap(cameraId).then(setHeatmapData).catch(() => {});
}, [heatmap, cameraId]);

{heatmap && (
  <div className="absolute inset-0 pointer-events-none">
    {heatmapData.map((cell, i) => {
      const intensity = Math.min(cell.count / 100, 1);
      return (
        <div key={i} className="absolute"
             style={{
               left: `${cell.x * 5}%`,
               top: `${cell.y * 5}%`,
               width: '5%',
               height: '5%',
               backgroundColor: `rgba(255, 0, 0, ${intensity})`,
             }} />
      );
    })}
  </div>
)}
```

- [ ] **Step 4: Validate**

Run: `gofmt -d services/ai-worker/heatmap.go`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: crowd density heatmaps with 20x20 cell grid and hour-aligned aggregation"
```

---

### Task 6: PWA mobile support

**Files:**
- Modify: `web/package.json`
- Create: `web/sw.js`
- Create: `web/manifest.json`
- Modify: `web/index.html`

- [ ] **Step 1: Install PWA dependencies**

```bash
cd /home/ubuntu/EVMS/web
npm install vite-plugin-pwa workbox-precaching
```

- [ ] **Step 2: Add PWA plugin to Vite config**

Modify `vite.config.ts` (or `web/vite.config.ts`):

```typescript
import { VitePWA } from 'vite-plugin-pwa';

export default defineConfig({
  plugins: [
    VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['favicon.ico', 'robots.txt'],
      manifest: {
        name: 'DAM VMS',
        short_name: 'VMS',
        description: 'Video Management System',
        theme_color: '#1a1a2e',
        background_color: '#1a1a2e',
        display: 'standalone',
        orientation: 'any',
        icons: [
          { src: '/icons/192.png', sizes: '192x192', type: 'image/png' },
          { src: '/icons/512.png', sizes: '512x512', type: 'image/png' },
        ],
      },
      workbox: {
        globPatterns: ['**/*.{js,css,html,ico,png,svg,woff2}'],
        runtimeCaching: [
          {
            urlPattern: /^\/api\/(?!login).*/,
            handler: 'NetworkFirst',
            options: { cacheName: 'api-cache', expiration: { maxEntries: 100, maxAgeSeconds: 60 } },
          },
          {
            urlPattern: /\.(jpg|jpeg|png|gif)$/,
            handler: 'CacheFirst',
            options: { cacheName: 'image-cache', expiration: { maxEntries: 200, maxAgeSeconds: 86400 } },
          },
        ],
      },
    }),
  ],
});
```

- [ ] **Step 3: Validate**

```bash
npx tsc --noEmit
```

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat: PWA support with service worker, manifest, and API caching"
```

---

### Task 7: Blueprint floor plan support

**Files:**
- Create: `web/src/components/FloorPlanView.tsx`
- Modify: `web/src/pages/SettingsPage.tsx`
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Create FloorPlanView component**

Create `web/src/components/FloorPlanView.tsx`:

```typescript
import { useEffect, useRef, useState } from 'react';
import { api, Camera } from '../api/client';

interface FloorPlanProps {
  imageUrl: string;
  cameras: Camera[];
  onCameraClick: (id: string) => void;
}

export default function FloorPlanView({ imageUrl, cameras, onCameraClick }: FloorPlanProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [positions, setPositions] = useState<Record<string, {x: number; y: number}>>({});
  const [dragging, setDragging] = useState<string | null>(null);

  // Load saved positions from camera config
  useEffect(() => {
    const pos: Record<string, {x: number; y: number}> = {};
    cameras.forEach(cam => {
      const config = cam.config ? JSON.parse(cam.config) : {};
      if (config.floor_plan_position) {
        pos[cam.id] = config.floor_plan_position;
      }
    });
    setPositions(pos);
  }, [cameras]);

  const savePosition = async (cameraId: string, x: number, y: number) => {
    const updated = { ...positions, [cameraId]: { x, y } };
    setPositions(updated);
    await api.updateCameraConfig(cameraId, { floor_plan_position: { x, y } });
  };

  return (
    <div ref={containerRef} className="relative w-full h-full bg-gray-900 rounded overflow-hidden">
      <img src={imageUrl} alt="Floor Plan" className="w-full h-full object-contain" />

      {cameras.map(cam => {
        const pos = positions[cam.id];
        if (!pos) return null;
        const statusColor = cam.status === 'online' ? 'bg-green-500' :
                            cam.status === 'error' ? 'bg-red-500' : 'bg-gray-500';

        return (
          <div key={cam.id}
               className={`absolute w-6 h-6 rounded-full ${statusColor} border-2 border-white cursor-pointer
                           flex items-center justify-center text-xs font-bold shadow-lg hover:scale-125 transition-transform`}
               style={{ left: `${pos.x * 100}%`, top: `${pos.y * 100}%`, transform: 'translate(-50%, -50%)' }}
               onClick={() => onCameraClick(cam.id)}
               draggable
               onDragStart={() => setDragging(cam.id)}
               onDragEnd={(e) => {
                 if (!containerRef.current) return;
                 const rect = containerRef.current.getBoundingClientRect();
                 const x = (e.clientX - rect.left) / rect.width;
                 const y = (e.clientY - rect.top) / rect.height;
                 savePosition(cam.id, Math.max(0, Math.min(1, x)), Math.max(0, Math.min(1, y)));
                 setDragging(null);
               }}>
            <span className="text-white">{cam.name.charAt(0)}</span>
          </div>
        );
      })}
    </div>
  );
}
```

- [ ] **Step 2: Add floor plan upload to SettingsPage**

```typescript
const [floorPlanImage, setFloorPlanImage] = useState<string | null>(null);

const handleFloorPlanUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
  const file = e.target.files?.[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = () => setFloorPlanImage(reader.result as string);
  reader.readAsDataURL(file);
  // Upload to server
  const formData = new FormData();
  formData.append('floor_plan', file);
  await api.uploadFloorPlan(siteId, formData);
};

{floorPlanImage && (
  <FloorPlanView
    imageUrl={floorPlanImage}
    cameras={siteCameras}
    onCameraClick={(id) => navigate(`/dashboard?camera=${id}`)}
  />
)}
```

- [ ] **Step 3: Validate**

Run: `npx tsc --noEmit`

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat: SVG/blueprint floor plan overlay with draggable camera icons"
```

---

### Task 8: Storage planning dashboard

**Files:**
- Create: `services/recorder/storage.go`
- Modify: `web/src/pages/SettingsPage.tsx`
- Create: `web/src/pages/StoragePage.tsx`

- [ ] **Step 1: Create storage calculator**

Create `services/recorder/storage.go`:

```go
package main

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/jmoiron/sqlx"
)

type StorageEstimate struct {
    CameraID         string  `json:"camera_id"`
    CameraName       string  `json:"camera_name"`
    RetentionDays    int     `json:"retention_days"`
    DailyUsageGB     float64 `json:"daily_usage_gb"`
    CurrentUsageGB   float64 `json:"current_usage_gb"`
    EstimatedTotalGB float64 `json:"estimated_total_gb"`
    DaysRemaining    float64 `json:"days_remaining"`
}

func handleStorageEstimate(db *sqlx.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var cameras []struct {
            ID            string `db:"id"`
            Name          string `db:"name"`
            RetentionDays int    `db:"retention_days"`
        }
        db.Select(&cameras, "SELECT id, name, retention_days FROM cameras")

        var estimates []StorageEstimate
        for _, cam := range cameras {
            var dailyGB sql.NullFloat64
            db.Get(&dailyGB,
                `SELECT COALESCE(SUM(file_size) / NULLIF(EXTRACT(DAY FROM NOW() - MIN(start_time)), 0), 0) / 1073741824.0
                 FROM recordings WHERE camera_id=$1 AND start_time > NOW() - INTERVAL '7 days'`,
                cam.ID)

            var currentGB sql.NullFloat64
            db.Get(&currentGB,
                `SELECT COALESCE(SUM(file_size), 0) / 1073741824.0 FROM recordings WHERE camera_id=$1`,
                cam.ID)

            daily := dailyGB.Float64
            if daily < 0.1 {
                daily = 2.0 // default estimate if no data
            }

            estimates = append(estimates, StorageEstimate{
                CameraID:         cam.ID,
                CameraName:       cam.Name,
                RetentionDays:    cam.RetentionDays,
                DailyUsageGB:     daily,
                CurrentUsageGB:   currentGB.Float64,
                EstimatedTotalGB: daily * float64(cam.RetentionDays),
                DaysRemaining:    (float64(cam.RetentionDays) * daily) / daily,
            })
        }

        json.NewEncoder(w).Encode(map[string]interface{}{
            "estimates": estimates,
            "total_daily_gb": sumDaily(estimates),
            "total_storage_gb": sumCurrent(estimates),
        })
    }
}
```

- [ ] **Step 2: Create StoragePage**

Create `web/src/pages/StoragePage.tsx`:

```typescript
import { useState, useEffect } from 'react';
import { api } from '../api/client';

interface StorageEstimate {
  camera_id: string;
  camera_name: string;
  retention_days: number;
  daily_usage_gb: number;
  current_usage_gb: number;
  estimated_total_gb: number;
  days_remaining: number;
}

export default function StoragePage() {
  const [estimates, setEstimates] = useState<StorageEstimate[]>([]);
  const [totals, setTotals] = useState({ total_daily_gb: 0, total_storage_gb: 0 });

  useEffect(() => {
    api.getStorageEstimates().then(data => {
      setEstimates(data.estimates);
      setTotals({ total_daily_gb: data.total_daily_gb, total_storage_gb: data.total_storage_gb });
    }).catch(() => {});
  }, []);

  const totalEstimated = estimates.reduce((s, e) => s + e.estimated_total_gb, 0);

  return (
    <div className="p-4">
      <h1 className="text-xl font-bold mb-4">Storage Planning</h1>

      <div className="grid grid-cols-3 gap-4 mb-6">
        <div className="bg-gray-800 p-4 rounded">
          <div className="text-sm text-gray-400">Daily Ingest</div>
          <div className="text-2xl font-bold">{totals.total_daily_gb.toFixed(1)} GB</div>
        </div>
        <div className="bg-gray-800 p-4 rounded">
          <div className="text-sm text-gray-400">Current Usage</div>
          <div className="text-2xl font-bold">{totals.total_storage_gb.toFixed(1)} GB</div>
        </div>
        <div className="bg-gray-800 p-4 rounded">
          <div className="text-sm text-gray-400">Estimated @ Retention</div>
          <div className="text-2xl font-bold">{totalEstimated.toFixed(1)} GB</div>
        </div>
      </div>

      <table className="w-full text-sm">
        <thead>
          <tr className="text-gray-400 border-b border-gray-700">
            <th className="text-left p-2">Camera</th>
            <th className="text-right p-2">Retention</th>
            <th className="text-right p-2">Daily</th>
            <th className="text-right p-2">Current</th>
            <th className="text-right p-2">Est. Total</th>
          </tr>
        </thead>
        <tbody>
          {estimates.map(e => (
            <tr key={e.camera_id} className="border-b border-gray-800 hover:bg-gray-800/50">
              <td className="p-2">{e.camera_name}</td>
              <td className="p-2 text-right">{e.retention_days}d</td>
              <td className="p-2 text-right">{e.daily_usage_gb.toFixed(1)} GB</td>
              <td className="p-2 text-right">{e.current_usage_gb.toFixed(1)} GB</td>
              <td className="p-2 text-right">{e.estimated_total_gb.toFixed(1)} GB</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

- [ ] **Step 3: Add route and nav**

Add `/storage` route and "Storage" nav item.

- [ ] **Step 4: Validate**

Run: `gofmt -d services/recorder/storage.go` and `npx tsc --noEmit`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: storage planning dashboard with per-camera estimates and totals"
```
