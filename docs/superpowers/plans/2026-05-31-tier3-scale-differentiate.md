# Tier 3: Scale & Differentiate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add competitive differentiators that win RFPs — LPR/ALPR, tripwire detection, people counting, archive tiering, camera I/O, and conditional rules engine.

**Architecture:** OpenALPR integration as an ai-worker pipeline stage; direction-aware line segments for tripwires in event-proc; zone crossing counters in TSDB; configurable archive paths with Go goroutine lifecycle; ONVIF IO relay via SOAP calls; JSON rule engine evaluated in event-proc.

**Tech Stack:** Go (event-proc, ai-worker, recorder), OpenALPR (license plates), FFmpeg, S3 SDK (tiering), PostgreSQL/TimescaleDB (counters)

---

### Task 1: LPR/ALPR integration

**Files:**
- Create: `services/ai-worker/lpr.go`
- Create: `services/ai-worker/lpr_test.go`
- Modify: `services/ai-worker/main.go`
- Modify: `services/api-gateway/main.go`
- Modify: `web/src/pages/SearchPage.tsx`

- [ ] **Step 1: Create LPR processor**

Create `services/ai-worker/lpr.go`:

```go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "image"
    "image/jpeg"
    "io"
    "log/slog"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "time"
)

type LPRResult struct {
    Plate     string  `json:"plate"`
    Region    string  `json:"region"`
    Confidence float64 `json:"confidence"`
    Box       [4]int  `json:"box"` // x1,y1,x2,y2
    Timestamp time.Time `json:"timestamp"`
}

type LPRProcessor struct {
    enabled     bool
    openalprCmd string
    hotlist     map[string]string // plate -> reason (e.g. "STOLEN", "WATCHED")
    webhookURL  string
    logger      *slog.Logger
}

func NewLPRProcessor(logger *slog.Logger) *LPRProcessor {
    return &LPRProcessor{
        enabled:     os.Getenv("LPR_ENABLED") == "true",
        openalprCmd: os.Getenv("OPENALPR_CMD"),
        hotlist:     make(map[string]string),
        webhookURL:  os.Getenv("LPR_HOTLIST_WEBHOOK"),
        logger:      logger,
    }
}

func (p *LPRProcessor) Process(frame image.Image) (*LPRResult, error) {
    if !p.enabled {
        return nil, nil
    }

    // Save frame temporarily
    tmpDir := os.TempDir()
    inputPath := filepath.Join(tmpDir, fmt.Sprintf("lpr_%d.jpg", time.Now().UnixNano()))
    f, err := os.Create(inputPath)
    if err != nil {
        return nil, err
    }
    jpeg.Encode(f, frame, &jpeg.Options{Quality: 85})
    f.Close()
    defer os.Remove(inputPath)

    // Run OpenALPR
    cmd := exec.Command(p.openalprCmd, "-j", inputPath)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        p.logger.Warn("OpenALPR failed", "error", err, "stderr", stderr.String())
        return nil, nil // non-fatal
    }

    var result struct {
        Results []struct {
            Plate     string  `json:"plate"`
            Region    string  `json:"region"`
            Confidence float64 `json:"confidence"`
            Coordinates []struct {
                X int `json:"x"`
                Y int `json:"y"`
            } `json:"coordinates"`
        } `json:"results"`
    }
    if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
        return nil, fmt.Errorf("parse alpr output: %w", err)
    }

    if len(result.Results) == 0 {
        return nil, nil
    }

    best := result.Results[0]
    lpr := &LPRResult{
        Plate:      best.Plate,
        Region:     best.Region,
        Confidence: best.Confidence,
        Timestamp:  time.Now(),
    }

    // Check hotlist
    if reason, ok := p.hotlist[best.Plate]; ok && p.webhookURL != "" {
        go p.fireHotlistAlert(lpr, reason)
    }

    return lpr, nil
}

func (p *LPRProcessor) fireHotlistAlert(lpr *LPRResult, reason string) {
    body, _ := json.Marshal(map[string]interface{}{
        "plate": lpr.Plate,
        "reason": reason,
        "confidence": lpr.Confidence,
        "timestamp": lpr.Timestamp,
    })
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    req, _ := http.NewRequestWithContext(ctx, "POST", p.webhookURL, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        p.logger.Error("hotlist webhook failed", "error", err)
        return
    }
    io.Copy(io.Discard, resp.Body)
    resp.Body.Close()
}

func (p *LPRProcessor) UpdateHotlist(plates map[string]string) {
    p.hotlist = plates
}
```

- [ ] **Step 2: Write LPR test**

Create `services/ai-worker/lpr_test.go`:

```go
package main

import (
    "image"
    "image/color"
    "testing"
)

func TestLPRProcessor_Disabled(t *testing.T) {
    logger := &slog.Logger{}
    p := NewLPRProcessor(logger)
    result, err := p.Process(image.NewRGBA(image.Rect(0, 0, 100, 100)))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != nil {
        t.Fatal("expected nil result when disabled")
    }
}

func TestHotlistCheck(t *testing.T) {
    p := &LPRProcessor{
        hotlist: map[string]string{"ABC123": "STOLEN"},
    }
    if _, ok := p.hotlist["ABC123"]; !ok {
        t.Fatal("expected hotlist to contain ABC123")
    }
}
```

- [ ] **Step 3: Wire LPR into ai-worker pipeline**

In `services/ai-worker/main.go`, add LPR stage after object detection:

```go
lpr := NewLPRProcessor(logger)

// In frame processing loop:
if lprResult, err := lpr.Process(frame); err == nil && lprResult != nil {
    // Publish LPR result as ai_event with object_type=license_plate
    event := AIEvent{
        CameraID:    cameraID,
        ObjectType:  "license_plate",
        Confidence:  lprResult.Confidence,
        Metadata:    fmt.Sprintf(`{"plate":"%s","region":"%s"}`, lprResult.Plate, lprResult.Region),
        BoundingBox: fmt.Sprintf(`[%d,%d,%d,%d]`, lprResult.Box[0], lprResult.Box[1], lprResult.Box[2], lprResult.Box[3]),
    }
    publishEvent(nc, event)
}
```

- [ ] **Step 4: Add plate search to SearchPage**

In `web/src/pages/SearchPage.tsx`, add "license_plate" to object type dropdown:

```typescript
<select value={objectType} onChange={e => setObjectType(e.target.value)}
        className="bg-gray-700 p-2 rounded">
  <option value="">All types</option>
  <option value="person">Person</option>
  <option value="car">Car</option>
  <option value="license_plate">License Plate</option>
</select>
```

Add plate text filter:

```typescript
const [plateText, setPlateText] = useState('');

// In search params:
const params: Record<string, string> = {};
if (plateText) params.metadata = JSON.stringify({ plate: plateText });

// Pass metadata filter to API
```

- [ ] **Step 5: Validate**

Run: `cd services/ai-worker && go test ./...`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: LPR/ALPR with OpenALPR integration, hotlist alerting, plate text search"
```

---

### Task 2: Tripwire / line crossing detection

**Files:**
- Modify: `services/event-proc/main.go`
- Create: `services/event-proc/tripwire.go`
- Create: `services/event-proc/tripwire_test.go`

- [ ] **Step 1: Create tripwire detector**

Create `services/event-proc/tripwire.go`:

```go
package main

import (
    "encoding/json"
    "math"
)

type Point struct {
    X float64 `json:"x"`
    Y float64 `json:"y"`
}

type Tripwire struct {
    ID        string  `json:"id"`
    Name      string  `json:"name"`
    Start     Point   `json:"start"`
    End       Point   `json:"end"`
    Direction string  `json:"direction"` // "any", "left_to_right", "right_to_left", "top_to_bottom", "bottom_to_top"
    CameraID  string  `json:"camera_id"`
}

type TrackPosition struct {
    TrackID string
    PrevCenter Point
    CurrCenter Point
}

// crossProduct determines which side of the line the point is on
func crossProduct(lineStart, lineEnd, point Point) float64 {
    return (lineEnd.X-lineStart.X)*(point.Y-lineStart.Y) - (lineEnd.Y-lineStart.Y)*(point.X-lineStart.X)
}

// lineIntersect checks if segment p1-p2 intersects segment q1-q2
func lineIntersect(p1, p2, q1, q2 Point) bool {
    d1 := crossProduct(q1, q2, p1)
    d2 := crossProduct(q1, q2, p2)
    d3 := crossProduct(p1, p2, q1)
    d4 := crossProduct(p1, p2, q2)

    // Check if endpoints straddle each other's lines
    if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
        ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
        return true
    }
    // Collinear cases
    if d1 == 0 && onSegment(q1, q2, p1) { return true }
    if d2 == 0 && onSegment(q1, q2, p2) { return true }
    if d3 == 0 && onSegment(p1, p2, q1) { return true }
    if d4 == 0 && onSegment(p1, p2, q2) { return true }

    return false
}

func onSegment(a, b, c Point) bool {
    return math.Min(a.X, b.X) <= c.X && c.X <= math.Max(a.X, b.X) &&
        math.Min(a.Y, b.Y) <= c.Y && c.Y <= math.Max(a.Y, b.Y)
}

func (t *Tripwire) CheckCrossing(prev, curr Point) bool {
    return lineIntersect(t.Start, t.End, prev, curr)
}

func (t *Tripwire) CheckDirection(prev, curr Point) bool {
    if t.Direction == "any" {
        return true
    }

    dx := curr.X - prev.X
    dy := curr.Y - prev.Y

    switch t.Direction {
    case "left_to_right":
        return dx > 0 && math.Abs(dx) > math.Abs(dy)
    case "right_to_left":
        return dx < 0 && math.Abs(dx) > math.Abs(dy)
    case "top_to_bottom":
        return dy > 0 && math.Abs(dy) > math.Abs(dx)
    case "bottom_to_top":
        return dy < 0 && math.Abs(dy) > math.Abs(dx)
    }
    return false
}
```

- [ ] **Step 2: Write tripwire tests**

Create `services/event-proc/tripwire_test.go`:

```go
package main

import (
    "testing"
)

func TestCrossProduct(t *testing.T) {
    // Line from (0,0) to (10,0), point (5,5) should be positive
    cp := crossProduct(Point{0, 0}, Point{10, 0}, Point{5, 5})
    if cp <= 0 {
        t.Fatalf("expected positive cross product, got %f", cp)
    }
}

func TestLineIntersect(t *testing.T) {
    // Crossing lines
    if !lineIntersect(Point{0, 0}, Point{10, 10}, Point{0, 10}, Point{10, 0}) {
        t.Fatal("expected lines to intersect")
    }
    // Parallel lines
    if lineIntersect(Point{0, 0}, Point{10, 0}, Point{0, 5}, Point{10, 5}) {
        t.Fatal("expected parallel lines not to intersect")
    }
}

func TestTripwireCrossing(t *testing.T) {
    tw := Tripwire{
        Start: Point{0, 5},
        End:   Point{10, 5},
    }
    // Track moving from y=0 to y=10 should cross
    if !tw.CheckCrossing(Point{5, 0}, Point{5, 10}) {
        t.Fatal("expected crossing detection")
    }
    // Track moving within same side should not
    if tw.CheckCrossing(Point{5, 0}, Point{5, 4}) {
        t.Fatal("expected no crossing")
    }
}

func TestDirectionFilter(t *testing.T) {
    tw := Tripwire{Direction: "left_to_right"}
    if !tw.CheckDirection(Point{0, 5}, Point{10, 5}) {
        t.Fatal("expected left_to_right to match")
    }
    if tw.CheckDirection(Point{10, 5}, Point{0, 5}) {
        t.Fatal("expected right_to_left not to match left_to_right")
    }
}
```

- [ ] **Step 3: Wire tripwire into event-proc tracker**

In `services/event-proc/main.go`, after IoU matching updates track positions:

```go
// Load tripwires for this camera
var tripwires []Tripwire
tripwireData, err := s.db.QueryContext(ctx,
    "SELECT config->'tripwires' FROM cameras WHERE id = $1", cameraID)
// Parse into []Tripwire

// For each active track, check tripwire crossing
for _, tw := range tripwires {
    if tw.CheckCrossing(prevPos, currPos) && tw.CheckDirection(prevPos, currPos) {
        // Publish alert
        nc.Publish(fmt.Sprintf("camera.%s.tripwire", cameraID),
            []byte(fmt.Sprintf(`{"tripwire_id":"%s","track_id":"%s","direction":"%s"}`, tw.ID, trackID, tw.Direction)))
    }
}
```

- [ ] **Step 4: Validate**

Run: `cd services/event-proc && go test ./... -v`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: tripwire/line crossing detection with direction filter"
```

---

### Task 3: People counting with TSDB counters

**Files:**
- Create: `services/event-proc/counter.go`
- Create: `migrations/002_people_counters.sql`
- Modify: `web/src/pages/Dashboard.tsx`
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Create people counter**

Create `services/event-proc/counter.go`:

```go
package main

import (
    "database/sql"
    "fmt"
    "time"

    "github.com/jmoiron/sqlx"
)

type PeopleCounter struct {
    db *sqlx.DB
}

type ZoneCrossing struct {
    CameraID   string    `json:"camera_id"`
    ZoneID     string    `json:"zone_id"`
    Direction  string    `json:"direction"` // "enter" or "exit"
    Count      int       `json:"count"`
    WindowStart time.Time `json:"window_start"`
    WindowEnd   time.Time  `json:"window_end"`
}

func NewPeopleCounter(db *sqlx.DB) *PeopleCounter {
    return &PeopleCounter{db: db}
}

func (pc *PeopleCounter) RecordCrossing(cameraID, zoneID, direction string, t time.Time) error {
    _, err := pc.db.Exec(
        `INSERT INTO people_counters (camera_id, zone_id, direction, bucket, count)
         VALUES ($1, $2, $3, date_trunc('hour', $4), 1)
         ON CONFLICT (camera_id, zone_id, direction, bucket)
         DO UPDATE SET count = people_counters.count + 1`,
        cameraID, zoneID, direction, t)
    return err
}

func (pc *PeopleCounter) GetCount(cameraID, zoneID string, start, end time.Time) (int, error) {
    var total int
    err := pc.db.Get(&total,
        `SELECT COALESCE(SUM(CASE WHEN direction='enter' THEN count ELSE -count END), 0)
         FROM people_counters
         WHERE camera_id=$1 AND zone_id=$2 AND bucket BETWEEN $3 AND $4`,
        cameraID, zoneID, start, end)
    return total, err
}

func (pc *PeopleCounter) GetHourlyBreakdown(cameraID string, date time.Time) ([]ZoneCrossing, error) {
    var results []ZoneCrossing
    err := pc.db.Select(&results,
        `SELECT camera_id, zone_id, direction, bucket as window_start,
                bucket + interval '1 hour' as window_end, count
         FROM people_counters
         WHERE camera_id=$1 AND bucket >= $2 AND bucket < $3
         ORDER BY bucket`,
        cameraID, date, date.Add(24*time.Hour))
    return results, err
}
```

- [ ] **Step 2: Create migration for people_counters**

Create `migrations/002_people_counters.sql`:

```sql
CREATE TABLE IF NOT EXISTS people_counters (
    camera_id UUID NOT NULL,
    zone_id TEXT NOT NULL,
    direction TEXT NOT NULL, -- 'enter' or 'exit'
    bucket TIMESTAMPTZ NOT NULL,
    count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (camera_id, zone_id, direction, bucket)
);

SELECT create_hypertable('people_counters', 'bucket', if_not_exists => true);

CREATE INDEX IF NOT EXISTS idx_people_counters_lookup
    ON people_counters(camera_id, zone_id, bucket DESC);
```

- [ ] **Step 3: Add people counter widget to Dashboard**

In `web/src/pages/Dashboard.tsx`:

```typescript
interface PeopleCount {
  camera_id: string;
  zone_id: string;
  count: number;
}

const [counts, setCounts] = useState<Record<string, number>>({});

useEffect(() => {
  const loadCounts = async () => {
    const data = await api.getPeopleCounts();
    const m: Record<string, number> = {};
    data.forEach(c => { m[c.camera_id] = (m[c.camera_id] || 0) + c.count; });
    setCounts(m);
  };
  loadCounts();
  const interval = setInterval(loadCounts, 60000);
  return () => clearInterval(interval);
}, []);

// In each CameraCard, show count overlay:
{counts[cam.id] !== undefined && (
  <span className="text-xs bg-blue-700 px-1 rounded">
    👤 {counts[cam.id]}
  </span>
)}
```

In `web/src/api/client.ts`:

```typescript
async getPeopleCounts(): Promise<PeopleCount[]> {
  const resp = await this.fetch('/api/analytics/people-counts');
  const data = await resp.json();
  return data.counts;
}
```

- [ ] **Step 4: Add analytics routes in gateway**

```go
case strings.HasPrefix(path, "/api/analytics/"):
    g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
        // Query camera-mgmt or event-proc for analytics data
        // Proxy to event-proc admin server
        r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
        http.ReverseProxy{Director: func(r *http.Request) {
            r.URL.Scheme = "http"
            r.URL.Host = "event-proc:8093"
        }}.ServeHTTP(w, r)
    }))(w, r)
```

- [ ] **Step 5: Validate**

Run: `gofmt -d services/event-proc/counter.go`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: people counting with TSDB hourly counters and dashboard widget"
```

---

### Task 4: Archive tiering (hot → warm → cold)

**Files:**
- Create: `services/recorder/tiering.go`
- Modify: `services/recorder/main.go`
- Modify: `api/v1/camera.pb.go` (add tier config)
- Modify: `web/src/pages/SettingsPage.tsx`

- [ ] **Step 1: Create tiering manager**

Create `services/recorder/tiering.go`:

```go
package main

import (
    "context"
    "fmt"
    "io"
    "log/slog"
    "os"
    "os/exec"
    "path/filepath"
    "sync"
    "time"
)

type StorageTier int

const (
    TierHot  StorageTier = iota // NVME, fast
    TierWarm                    // HDD, bulk
    TierCold                    // S3/Glacier
)

type TierConfig struct {
    HotPath  string        `json:"hot_path"`
    WarmPath string        `json:"warm_path"`
    ColdPath string        `json:"cold_path"`
    WarmDays int           `json:"warm_days"`  // move to warm after N days
    ColdDays int           `json:"cold_days"`  // move to cold after N days
    CheckInterval time.Duration `json:"check_interval"`
}

type TieringManager struct {
    config TierConfig
    logger *slog.Logger
    mu     sync.Mutex
    active map[string]bool // currently moving — prevent duplicates
}

func NewTieringManager(config TierConfig, logger *slog.Logger) *TieringManager {
    if config.CheckInterval == 0 {
        config.CheckInterval = 1 * time.Hour
    }
    m := &TieringManager{
        config: config,
        logger: logger,
        active: make(map[string]bool),
    }
    return m
}

func (m *TieringManager) Start(ctx context.Context) {
    ticker := time.NewTicker(m.config.CheckInterval)
    defer ticker.Stop()

    // Run once immediately
    m.tierSegments()

    for {
        select {
        case <-ticker.C:
            m.tierSegments()
        case <-ctx.Done():
            return
        }
    }
}

func (m *TieringManager) tierSegments() {
    if m.config.WarmPath == "" && m.config.ColdPath == "" {
        return
    }

    filepath.Walk(m.config.HotPath, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() {
            return nil
        }

        age := time.Since(info.ModTime())
        m.mu.Lock()
        if m.active[path] {
            m.mu.Unlock()
            return nil
        }
        m.active[path] = true
        m.mu.Unlock()

        defer func() {
            m.mu.Lock()
            delete(m.active, path)
            m.mu.Unlock()
       	}()

        if m.config.ColdDays > 0 && age.Hours() > float64(m.config.ColdDays*24) {
            m.moveToCold(path)
        } else if m.config.WarmDays > 0 && age.Hours() > float64(m.config.WarmDays*24) {
            m.moveToWarm(path)
        }

        return nil
    })
}

func (m *TieringManager) moveToWarm(src string) {
    rel, _ := filepath.Rel(m.config.HotPath, src)
    dst := filepath.Join(m.config.WarmPath, rel)

    os.MkdirAll(filepath.Dir(dst), 0755)

    // Use rsync or copy for warm (local)
    if err := copyFile(src, dst); err != nil {
        m.logger.Error("failed to move to warm", "src", src, "error", err)
        return
    }
    os.Remove(src)
    m.logger.Info("tiered to warm", "file", rel)
}

func (m *TieringManager) moveToCold(src string) {
    rel, _ := filepath.Rel(m.config.HotPath, src)
    dst := filepath.Join(m.config.ColdPath, rel)

    os.MkdirAll(filepath.Dir(dst), 0755)

    // Use aws CLI for S3 upload
    cmd := exec.Command("aws", "s3", "cp", src, dst)
    if output, err := cmd.CombinedOutput(); err != nil {
        m.logger.Error("failed to move to cold (S3)", "src", src, "error", err, "output", string(output))
        return
    }
    os.Remove(src)
    m.logger.Info("tiered to cold (S3)", "file", rel)
}

func copyFile(src, dst string) error {
    in, err := os.Open(src)
    if err != nil {
        return err
    }
    defer in.Close()

    out, err := os.Create(dst)
    if err != nil {
        return err
    }
    defer out.Close()

    _, err = io.Copy(out, in)
    return err
}
```

- [ ] **Step 2: Wire tiering into recorder main**

In `services/recorder/main.go`, start tiering manager:

```go
tierConfig := TierConfig{
    HotPath:  common.GetEnv("RECORDING_PATH", "/recordings"),
    WarmPath: os.Getenv("WARM_STORAGE_PATH"),
    ColdPath: os.Getenv("COLD_STORAGE_PATH"),
    WarmDays: 7,
    ColdDays: 30,
}

tm := NewTieringManager(tierConfig, logger)
go tm.Start(ctx)
```

- [ ] **Step 3: Add tier config to SettingsPage**

In `web/src/pages/SettingsPage.tsx`, add archive section:

```typescript
const [hotPath, setHotPath] = useState('/recordings/hot');
const [warmPath, setWarmPath] = useState('/recordings/warm');
const [warmDays, setWarmDays] = useState(7);
const [coldDays, setColdDays] = useState(30);

<div className="bg-gray-800 p-4 rounded">
  <h3 className="font-bold mb-2">Archive Tiering</h3>
  <div className="space-y-2 text-sm">
    <div><label className="text-gray-400">Hot→Warm after (days):</label>
      <input type="number" value={warmDays} onChange={e => setWarmDays(Number(e.target.value))}
             className="bg-gray-700 ml-2 p-1 rounded w-16" min={1} max={90} /></div>
    <div><label className="text-gray-400">Warm→Cold after (days):</label>
      <input type="number" value={coldDays} onChange={e => setColdDays(Number(e.target.value))}
             className="bg-gray-700 ml-2 p-1 rounded w-16" min={1} max={365} /></div>
  </div>
</div>
```

- [ ] **Step 4: Validate**

Run: `gofmt -d services/recorder/tiering.go`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: archive tiering hot→warm→cold with configurable days and S3 support"
```

---

### Task 5: I/O port management (ONVIF relays + alarm inputs)

**Files:**
- Modify: `services/camera-control/main.go`
- Modify: `services/onvif-events/main.go`
- Modify: `web/src/pages/SettingsPage.tsx`
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Add ONVIF IO relay control to camera-control**

In `services/camera-control/main.go`, add IO relay handler:

```go
func handleIOControl(logger *slog.Logger) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        cameraID := extractParam(r.URL.Path, "/api/cameras/")
        cameraID = strings.TrimSuffix(cameraID, "/io")

        var req struct {
            RelayID   string `json:"relay_id"`
            State     string `json:"state"` // "on" or "off"
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            jsonError(w, "invalid request", http.StatusBadRequest)
            return
        }

        // Get camera ONVIF address
        cam := getCamera(cameraID)
        if cam.PtzProtocol != "ONVIF" {
            jsonError(w, "ONVIF required for IO control", http.StatusBadRequest)
            return
        }

        // SOAP SetRelayOutputState
        soapBody := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tr2="http://www.onvif.org/ver10/deviceio/wsdl">
  <s:Body>
    <tr2:SetRelayOutputState>
      <tr2:RelayOutputToken>%s</tr2:RelayOutputToken>
      <tr2:LogicalState>%s</tr2:LogicalState>
    </tr2:SetRelayOutputState>
  </s:Body>
</s:Envelope>`, req.RelayID, req.State)

        // POST to camera ONVIF device IO service
        ioURL := fmt.Sprintf("http://%s/onvif/deviceio_service", cam.OnvifAddress)
        resp, err := http.Post(ioURL, "application/soap+xml", strings.NewReader(soapBody))
        if err != nil {
            jsonError(w, "IO control failed", http.StatusInternalServerError)
            return
        }
        resp.Body.Close()

        json.NewEncoder(w).Encode(map[string]string{"status": "relay " + req.State})
    }
}
```

Add routes:

```go
case strings.HasSuffix(r.URL.Path, "/io"):
    handleIOControl(logger)(w, r)
```

- [ ] **Step 2: Handle alarm inputs in onvif-events**

In `services/onvif-events/main.go`, parse alarm input events:

```go
type AlarmInputEvent struct {
    CameraID  string `json:"camera_id"`
    InputID   string `json:"input_id"`
    State     string `json:"state"` // "active" or "inactive"
    Timestamp string `json:"timestamp"`
}

// In PullMessages response parsing:
func parseAlarmInput(rawXML string) *AlarmInputEvent {
    // Parse tns1:Device/Trigger/DigitalInput or similar ONVIF event
    // ONVIF uses /DeviceIO/DigitalInput:State or /Device/Trigger/Alarm
    return &AlarmInputEvent{
        State: extractState(rawXML),
    }
}
```

- [ ] **Step 3: Add IO control UI to SettingsPage**

In `web/src/pages/SettingsPage.tsx`:

```typescript
interface RelayInfo {
  token: string;
  name: string;
  state: boolean;
}

const [relays, setRelays] = useState<RelayInfo[]>([]);

const toggleRelay = async (relayID: string, state: boolean) => {
  await api.setRelayState(cameraId, relayID, state);
  setRelays(prev => prev.map(r => r.token === relayID ? { ...r, state } : r));
};

{relays.length > 0 && (
  <div className="bg-gray-800 p-4 rounded mt-4">
    <h3 className="font-bold mb-2">I/O Ports</h3>
    {relays.map(r => (
      <div key={r.token} className="flex items-center justify-between py-1">
        <span className="text-sm">{r.name}</span>
        <button onClick={() => toggleRelay(r.token, !r.state)}
                className={`px-3 py-1 rounded text-xs ${r.state ? 'bg-green-700' : 'bg-gray-600'}`}>
          {r.state ? 'ON' : 'OFF'}
        </button>
      </div>
    ))}
  </div>
)}
```

In `web/src/api/client.ts`:

```typescript
async setRelayState(cameraId: string, relayId: string, state: boolean): Promise<void> {
  await this.fetch(`/api/cameras/${cameraId}/io`, {
    method: 'POST',
    body: JSON.stringify({ relay_id: relayId, state: state ? 'on' : 'off' }),
  });
}
```

- [ ] **Step 4: Validate**

Run: `gofmt -d services/camera-control/main.go services/onvif-events/main.go`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: ONVIF IO relay control and alarm input handling"
```

---

### Task 6: Conditional rules engine

**Files:**
- Create: `services/event-proc/rule_engine.go`
- Create: `services/event-proc/rule_engine_test.go`
- Modify: `web/src/pages/EventsPage.tsx`

- [ ] **Step 1: Create rule engine**

Create `services/event-proc/rule_engine.go`:

```go
package main

import (
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
    "strings"
    "sync"
    "time"
)

type Condition struct {
    Source    string `json:"source"`    // "motion", "tripwire", "lpr", "alarm_input", "schedule"
    CameraID  string `json:"camera_id"`
    Operator  string `json:"operator"`  // "equals", "contains", "gt", "lt"
    Value     string `json:"value"`     // e.g. "STOLEN" for LPR, "active" for alarm input
    Schedule  string `json:"schedule"`  // cron expression or "business_hours"
}

type Action struct {
    Type   string `json:"type"`    // "relay", "webhook", "record", "alert", "email"
    Target string `json:"target"`  // relay token, URL, camera ID
    Params map[string]string `json:"params"`
}

type Rule struct {
    ID         string      `json:"id"`
    Name       string      `json:"name"`
    Enabled    bool        `json:"enabled"`
    Conditions []Condition `json:"conditions"`
    Actions    []Action    `json:"actions"`
    Logic      string      `json:"logic"` // "AND" or "OR"
}

type RuleEngine struct {
    mu     sync.RWMutex
    rules  map[string]*Rule
    logger *slog.Logger
}

func NewRuleEngine(logger *slog.Logger) *RuleEngine {
    return &RuleEngine{
        rules:  make(map[string]*Rule),
        logger: logger,
    }
}

func (re *RuleEngine) AddRule(rule *Rule) {
    re.mu.Lock()
    defer re.mu.Unlock()
    re.rules[rule.ID] = rule
}

func (re *RuleEngine) RemoveRule(id string) {
    re.mu.Lock()
    defer re.mu.Unlock()
    delete(re.rules, id)
}

func (re *RuleEngine) Evaluate(event map[string]interface{}) []Action {
    re.mu.RLock()
    defer re.mu.RUnlock()

    var triggered []Action
    for _, rule := range re.rules {
        if !rule.Enabled {
            continue
        }
        if re.matches(rule, event) {
            triggered = append(triggered, rule.Actions...)
        }
    }
    return triggered
}

func (re *RuleEngine) matches(rule *Rule, event map[string]interface{}) bool {
    if len(rule.Conditions) == 0 {
        return true
    }

    results := make([]bool, len(rule.Conditions))
    for i, cond := range rule.Conditions {
        results[i] = re.evaluateCondition(cond, event)
    }

    if rule.Logic == "OR" {
        for _, r := range results {
            if r {
                return true
            }
        }
        return false
    }
    // AND (default)
    for _, r := range results {
        if !r {
            return false
        }
    }
    return true
}

func (re *RuleEngine) evaluateCondition(cond Condition, event map[string]interface{}) bool {
    // Check camera ID match
    if cond.CameraID != "" && cond.CameraID != event["camera_id"] {
        return false
    }

    // Get the source value from event
    sourceVal, ok := event[cond.Source]
    if !ok {
        return false
    }

    valStr := fmt.Sprintf("%v", sourceVal)
    switch cond.Operator {
    case "equals":
        return valStr == cond.Value
    case "contains":
        return strings.Contains(valStr, cond.Value)
    case "gt":
        var v, t float64
        fmt.Sscanf(valStr, "%f", &v)
        fmt.Sscanf(cond.Value, "%f", &t)
        return v > t
    case "lt":
        var v, t float64
        fmt.Sscanf(valStr, "%f", &v)
        fmt.Sscanf(cond.Value, "%f", &t)
        return v < t
    }
    return false
}

func (re *RuleEngine) HandleHTTP(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        re.mu.RLock()
        rules := make([]*Rule, 0, len(re.rules))
        for _, r := range re.rules {
            rules = append(rules, r)
        }
        re.mu.RUnlock()
        json.NewEncoder(w).Encode(map[string]interface{}{"rules": rules})

    case http.MethodPost:
        var rule Rule
        if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
            jsonError(w, "invalid rule", http.StatusBadRequest)
            return
        }
        re.AddRule(&rule)
        json.NewEncoder(w).Encode(map[string]string{"status": "created", "id": rule.ID})

    case http.MethodDelete:
        id := strings.TrimPrefix(r.URL.Path, "/api/rules/")
        re.RemoveRule(id)
        json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
    }
}
```

- [ ] **Step 2: Write rule engine tests**

Create `services/event-proc/rule_engine_test.go`:

```go
package main

import (
    "testing"
)

func TestSimpleRuleMatch(t *testing.T) {
    re := NewRuleEngine(nil)
    re.AddRule(&Rule{
        ID: "test1", Enabled: true,
        Conditions: []Condition{
            {Source: "object_type", Operator: "equals", Value: "person"},
        },
        Logic: "AND",
    })

    actions := re.Evaluate(map[string]interface{}{
        "camera_id":   "cam1",
        "object_type": "person",
        "confidence":  0.95,
    })
    if len(actions) == 0 {
        t.Fatal("expected rule to match")
    }
}

func TestRuleNoMatch(t *testing.T) {
    re := NewRuleEngine(nil)
    re.AddRule(&Rule{
        ID: "test2", Enabled: true,
        Conditions: []Condition{
            {Source: "object_type", Operator: "equals", Value: "car"},
        },
    })

    // Wrong object type
    actions := re.Evaluate(map[string]interface{}{
        "camera_id":   "cam1",
        "object_type": "person",
    })
    if len(actions) > 0 {
        t.Fatal("expected no match")
    }
}

func TestORLogic(t *testing.T) {
    re := NewRuleEngine(nil)
    re.AddRule(&Rule{
        ID: "test3", Enabled: true, Logic: "OR",
        Conditions: []Condition{
            {Source: "object_type", Operator: "equals", Value: "person"},
            {Source: "object_type", Operator: "equals", Value: "car"},
        },
    })

    actions := re.Evaluate(map[string]interface{}{"object_type": "car"})
    if len(actions) == 0 {
        t.Fatal("expected OR rule to match car")
    }
}

func TestDisabledRule(t *testing.T) {
    re := NewRuleEngine(nil)
    re.AddRule(&Rule{
        ID: "test4", Enabled: false,
        Conditions: []Condition{
            {Source: "object_type", Operator: "equals", Value: "person"},
        },
    })

    actions := re.Evaluate(map[string]interface{}{"object_type": "person"})
    if len(actions) > 0 {
        t.Fatal("expected disabled rule not to match")
    }
}
```

- [ ] **Step 3: Wire rule engine into event-proc HTTP server**

In `services/event-proc/main.go`:

```go
ruleEngine := NewRuleEngine(logger)

// Add HTTP handlers for rule CRUD
mux.HandleFunc("/api/rules", ruleEngine.HandleHTTP)
mux.HandleFunc("/api/rules/", ruleEngine.HandleHTTP)

// In event processing loop, after detection:
actions := ruleEngine.Evaluate(eventData)
for _, action := range actions {
    switch action.Type {
    case "relay":
        // Trigger relay via camera-control
        http.Post(fmt.Sprintf("http://camera-control:8088/api/cameras/%s/io", eventData["camera_id"]),
            "application/json", strings.NewReader(`{"relay_id":"`+action.Target+`","state":"on"}`))
    case "webhook":
        body, _ := json.Marshal(eventData)
        http.Post(action.Target, "application/json", bytes.NewReader(body))
    case "alert":
        // Create alert via alert workflow
        alertWorkflow.CreateAlert(ruleID, cameraID, action.Params["message"])
    }
}
```

- [ ] **Step 4: Add rules UI to EventsPage**

In `web/src/pages/EventsPage.tsx`:

```typescript
interface Rule {
  id: string;
  name: string;
  enabled: boolean;
  conditions: {source: string; camera_id: string; operator: string; value: string}[];
  actions: {type: string; target: string; params: Record<string, string>}[];
  logic: string;
}

const [rules, setRules] = useState<Rule[]>([]);
const [showRuleEditor, setShowRuleEditor] = useState(false);

{rules.map(rule => (
  <div key={rule.id} className="bg-gray-800 p-3 rounded flex items-center justify-between">
    <div>
      <span className="font-medium">{rule.name}</span>
      <span className="text-xs text-gray-400 ml-2">
        IF {rule.conditions.map(c => `${c.source} ${c.operator} ${c.value}`).join(` ${rule.logic} `)}
        → {rule.actions.map(a => a.type).join(', ')}
      </span>
    </div>
    <label className="relative inline-flex items-center cursor-pointer">
      <input type="checkbox" checked={rule.enabled}
             onChange={() => toggleRule(rule.id)} className="sr-only peer" />
      <div className="w-9 h-5 bg-gray-600 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:bg-blue-600 after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all" />
    </label>
  </div>
))}
```

- [ ] **Step 5: Validate**

Run: `cd services/event-proc && go test ./... -v`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: conditional rules engine with AND/OR logic, relay/webhook/alert actions"
```
