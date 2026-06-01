# Tier 1: Table Stakes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the 5 most visible gaps that operators notice immediately — multi-stream per camera, pre-recording buffer, LDAP auth, smart motion search, bookmarking.

**Architecture:** Frontend changes in CameraView/CameraCard for multi-stream; recorder ring buffer in Go with configurable N seconds; LDAP as an auth backend alongside local auth; query bounding_box overlap in ai_events for smart search; bookmarks table + API + UI hotkey. All additive — no existing behavior breaks.

**Tech Stack:** Go (recorder, auth, api-gateway), React/TypeScript (UI), PostgreSQL (bookmarks), LDAP/glauth (auth bridge), FFmpeg (thumbnail streams)

---

### Task 1: Multi-stream per camera (thumbnail + live + substream)

**Files:**
- Modify: `web/src/components/CameraView.tsx`
- Modify: `web/src/components/CameraCard.tsx`
- Modify: `web/src/api/client.ts`
- Modify: `services/api-gateway/main.go`
- Create: `web/src/hooks/useStreamSelector.ts`

- [ ] **Step 1: Add stream type selection to API client**

In `web/src/api/client.ts`, add stream type parameter:

```typescript
export type StreamType = 'main' | 'sub' | 'thumbnail';

// In the api object, add or modify:
async getStreamUrl(cameraId: string, type: StreamType = 'main'): Promise<string> {
  const params = new URLSearchParams({ type });
  const resp = await fetch(`${this.baseUrl}/api/stream/${cameraId}?${params}`, {
    headers: { Authorization: `Bearer ${this.token}` },
  });
  if (!resp.ok) throw new Error('failed to get stream url');
  const data = await resp.json();
  return data.url;
}
```

- [ ] **Step 2: Create useStreamSelector hook**

Create `web/src/hooks/useStreamSelector.ts`:

```typescript
import { useState, useCallback } from 'react';
import { StreamType, api } from '../api/client';

interface StreamState {
  main: string;
  sub: string;
  thumbnail: string | null;
}

export function useStreamSelector(cameraId: string) {
  const [streams, setStreams] = useState<StreamState | null>(null);
  const [activeType, setActiveType] = useState<StreamType>('sub');

  const loadStreams = useCallback(async () => {
    const [main, sub] = await Promise.all([
      api.getStreamUrl(cameraId, 'main'),
      api.getStreamUrl(cameraId, 'sub'),
    ]);
    setStreams({ main, sub, thumbnail: null });
  }, [cameraId]);

  return { streams, activeType, setActiveType, loadStreams };
}
```

- [ ] **Step 3: Update CameraView to show thumbnail while main stream loads**

Modify `web/src/components/CameraView.tsx` — add a thumbnail fallback image that displays while the H.264 stream is negotiating:

```typescript
// Inside component, after existing state:
const [streamReady, setStreamReady] = useState(false);
const thumbnailUrl = `${baseUrl}/api/thumbnails/image/${cameraId}/latest.jpg`;

return (
  <div className="relative">
    {!streamReady && (
      <img
        src={thumbnailUrl}
        alt="Loading..."
        className="absolute inset-0 w-full h-full object-cover"
      />
    )}
    <video
      ref={videoRef}
      onCanPlay={() => setStreamReady(true)}
      className={streamReady ? 'block w-full h-full' : 'invisible w-full h-full'}
      autoPlay muted playsInline
    />
  </div>
);
```

- [ ] **Step 4: Add stream quality toggle to CameraCard**

Modify `web/src/components/CameraCard.tsx` — show a "HD/SD" badge that switches between main/sub:

```typescript
// Add after the H.264 / 1080P tags:
{streams && (
  <button
    onClick={() => setActiveType(activeType === 'main' ? 'sub' : 'main')}
    className="text-xs bg-gray-700 px-1 rounded"
  >
    {activeType === 'main' ? 'HD' : 'SD'}
  </button>
)}
```

- [ ] **Step 5: Add gateway route for stream URL resolution**

In `services/api-gateway/main.go`, add stream URL resolution handler:

```go
func (g *Gateway) handleStreamURL(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    cameraID := extractParam(r.URL.Path, "/api/stream/")
    streamType := r.URL.Query().Get("type")
    if streamType == "" {
        streamType = "main"
    }

    camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
    if err != nil {
        jsonError(w, "camera not found", http.StatusNotFound)
        return
    }

    url := camera.ConnectionUrl
    if streamType == "sub" && camera.SubstreamUrl != "" {
        url = camera.SubstreamUrl
    }

    json.NewEncoder(w).Encode(map[string]string{"url": url})
}
```

Add route in `ServeHTTP`:

```go
case strings.HasPrefix(path, "/api/stream/"):
    g.rateLimiter.rateLimitMiddleware(g.authMiddleware(g.handleStreamURL))(w, r)
```

- [ ] **Step 6: Validate all files compile**

Run: `npx tsc --noEmit` in `web/` and `gofmt -d services/api-gateway/main.go`

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: multi-stream per camera with thumbnail fallback and HD/SD toggle"
```

---

### Task 2: Pre-recording buffer

**Files:**
- Modify: `services/recorder/main.go`
- Modify: `services/camera-mgmt/main.go` (add config field)
- Modify: `api/v1/camera.pb.go` (add preroll field)

- [ ] **Step 1: Add prerecord field to Camera proto**

In `api/v1/camera.pb.go`, add `PrerecordSeconds` field to Camera message:

```go
type Camera struct {
    // ... existing fields ...
    PrerecordSeconds int32  `json:"prerecord_seconds,omitempty"`
}
```

Add to `ListCamerasResponse` field mapping in gateway and camera-mgmt if needed.

- [ ] **Step 2: Add ring buffer to recorder**

In `services/recorder/main.go`, add a ring buffer per camera before the segment writer:

```go
type ringBuffer struct {
    mu       sync.Mutex
    data     []byte
    capacity int // bytes, e.g. 2MB for ~5s of H.264 at 4Mbps
    head     int
    full     bool
}

func newRingBuffer(seconds int, bitrate int) *ringBuffer {
    capBytes := seconds * bitrate * 1024 / 8
    return &ringBuffer{data: make([]byte, capBytes), capacity: capBytes}
}

func (rb *ringBuffer) Write(p []byte) (int, error) {
    rb.mu.Lock()
    defer rb.mu.Unlock()
    for _, b := range p {
        rb.data[rb.head] = b
        rb.head = (rb.head + 1) % rb.capacity
        if rb.head == 0 {
            rb.full = true
        }
    }
    return len(p), nil
}

func (rb *ringBuffer) Bytes() []byte {
    rb.mu.Lock()
    defer rb.mu.Unlock()
    if !rb.full {
        return rb.data[:rb.head]
    }
    out := make([]byte, rb.capacity)
    copy(out, rb.data[rb.head:])
    copy(out[rb.capacity-rb.head:], rb.data[:rb.head])
    return out
}
```

- [ ] **Step 3: Wire ring buffer into recording start**

When a motion/event trigger starts a recording, write the ring buffer contents first:

```go
type CameraRecorder struct {
    buf *ringBuffer
    // ...
}

func (cr *CameraRecorder) handleNALUs(nalus [][]byte) {
    for _, nalu := range nalus {
        cr.buf.Write(nalu)
    }
}

func (cr *CameraRecorder) startSegment(trigger string) error {
    prerecord := cr.buf.Bytes()
    // write preroll to segment file, then continue recording
    if _, err := cr.currentWriter.Write(prerecord); err != nil {
        return err
    }
    cr.prerecorded = true
    return nil
}
```

- [ ] **Step 4: Validate formatting**

Run: `gofmt -d services/recorder/main.go`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: pre-recording ring buffer with configurable seconds per camera"
```

---

### Task 3: LDAP/AD authentication backend

**Files:**
- Modify: `services/auth/main.go`
- Modify: `deploy/docker/docker-compose.yml` (add ldap container)
- Modify: `deploy/k8s/all-services.yaml` (add ldap config)
- Modify: `.github/workflows/go-ci.yml` (no changes needed)

- [ ] **Step 1: Add LDAP dependencies**

Add to `services/auth/main.go`:

```go
import (
    "github.com/go-ldap/ldap/v3"
)
```

Update `go.mod`:

```bash
cd /home/ubuntu/EVMS
go get github.com/go-ldap/ldap/v3@latest
```

- [ ] **Step 2: Add LDAP config to AuthConfig**

```go
type AuthConfig struct {
    // ... existing fields ...
    LDAPEnabled  bool
    LDAPHost     string
    LDAPPort     int
    LDAPBaseDN   string
    LDAPBindDN   string
    LDAPPassword string
    LDAPFilter   string // e.g. "(uid=%s)"
}

func DefaultAuthConfig() AuthConfig {
    return AuthConfig{
        // ... existing ...
        LDAPEnabled:  os.Getenv("LDAP_ENABLED") == "true",
        LDAPHost:     common.GetEnv("LDAP_HOST", "localhost"),
        LDAPPort:     389,
        LDAPBaseDN:   common.GetEnv("LDAP_BASE_DN", "dc=example,dc=com"),
        LDAPBindDN:   common.GetEnv("LDAP_BIND_DN", ""),
        LDAPPassword: os.Getenv("LDAP_PASSWORD"),
        LDAPFilter:   common.GetEnv("LDAP_FILTER", "(uid=%s)"),
    }
}
```

- [ ] **Step 3: Implement LDAP authentication**

```go
func (s *AuthService) authenticateLDAP(ctx context.Context, username, password string) (*User, error) {
    conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", s.config.LDAPHost, s.config.LDAPPort))
    if err != nil {
        return nil, fmt.Errorf("ldap dial: %w", err)
    }
    defer conn.Close()

    // Bind with service account first
    if s.config.LDAPBindDN != "" {
        if err := conn.Bind(s.config.LDAPBindDN, s.config.LDAPPassword); err != nil {
            return nil, fmt.Errorf("ldap service bind: %w", err)
        }
    }

    // Search for user DN
    filter := strings.ReplaceAll(s.config.LDAPFilter, "%s", ldap.EscapeFilter(username))
    searchReq := &ldap.SearchRequest{
        BaseDN:     s.config.LDAPBaseDN,
        Scope:      ldap.ScopeWholeSubtree,
        Filter:     filter,
        Attributes: []string{"uid", "mail", "cn"},
    }
    result, err := conn.Search(searchReq)
    if err != nil {
        return nil, fmt.Errorf("ldap search: %w", err)
    }
    if len(result.Entries) == 0 {
        return nil, errors.New("user not found in ldap")
    }

    userDN := result.Entries[0].DN
    if err := conn.Bind(userDN, password); err != nil {
        return nil, errors.New("ldap bind failed: invalid password")
    }

    // Auto-provision local user if not exists
    var user User
    err = s.db.GetContext(ctx, &user,
        "SELECT id, username, password_hash, role, active FROM users WHERE username = $1",
        username)
    if err != nil {
        // Create local user
        var id string
        err = s.db.QueryRowContext(ctx,
            "INSERT INTO users (username, password_hash, role, active) VALUES ($1, '', 'viewer', true) RETURNING id",
            username).Scan(&id)
        if err != nil {
            return nil, fmt.Errorf("auto-provision user: %w", err)
        }
        user = User{ID: id, Username: username, Role: "viewer", Active: true}
    }

    return &user, nil
}
```

- [ ] **Step 4: Wire LDAP into login flow**

Modify `authenticateUser` to try LDAP first when enabled:

```go
func (s *AuthService) authenticateUser(ctx context.Context, username, password string) (string, error) {
    var user *User
    var err error

    if s.config.LDAPEnabled {
        user, err = s.authenticateLDAP(ctx, username, password)
        if err != nil {
            s.logger.Warn("LDAP auth failed, falling back to local", "username", username, "error", err)
        }
    }

    if user == nil {
        var localUser User
        err = s.db.GetContext(ctx, &localUser,
            "SELECT id, username, password_hash, role FROM users WHERE username = $1 AND active = true AND deleted_at IS NULL",
            username)
        if err != nil {
            return "", fmt.Errorf("user not found: %w", err)
        }
        if err := bcrypt.CompareHashAndPassword([]byte(localUser.PasswordHash), []byte(password)); err != nil {
            return "", errors.New("invalid password")
        }
        user = &localUser
    }

    return s.generateToken(*user)
}
```

- [ ] **Step 5: Add test LDAP server to docker-compose**

In `deploy/docker/docker-compose.yml`:

```yaml
  openldap:
    image: osixia/openldap:latest
    environment:
      LDAP_ORGANISATION: "DAM VMS"
      LDAP_DOMAIN: "dam.vms"
      LDAP_ADMIN_PASSWORD: "admin"
    ports:
      - "389:389"
    networks: [dam-net]

  ldap-admin:
    image: osixia/phpldapadmin:latest
    ports:
      - "6443:443"
    environment:
      PHPLDAPADMIN_LDAP_HOSTS: openldap
    networks: [dam-net]
```

- [ ] **Step 6: Validate formatting**

Run: `gofmt -d services/auth/main.go`

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: LDAP/AD authentication with auto-provisioning and local fallback"
```

---

### Task 4: Smart search by motion region

**Files:**
- Modify: `web/src/pages/SearchPage.tsx`
- Modify: `web/src/components/TimelineScrubber.tsx`
- Modify: `services/api-gateway/main.go` (add bounding_box filter)

- [ ] **Step 1: Add bounding box filter to smart search gateway handler**

In `services/api-gateway/main.go`, add bounding box query params to `handleSmartSearch`:

```go
func (g *Gateway) handleSmartSearch(w http.ResponseWriter, r *http.Request) {
    var req struct {
        CameraID      string  `json:"camera_id"`
        ObjectType    string  `json:"object_type"`
        MinConfidence float64 `json:"min_confidence"`
        StartTime     string  `json:"start_time"`
        EndTime       string  `json:"end_time"`
        Limit         int32   `json:"limit"`
        BoundingBox   string  `json:"bounding_box"` // "x1,y1,x2,y2" — motion region
    }

    if r.Method == http.MethodGet {
        req.CameraID = r.URL.Query().Get("camera_id")
        req.ObjectType = r.URL.Query().Get("object_type")
        req.StartTime = r.URL.Query().Get("start_time")
        req.EndTime = r.URL.Query().Get("end_time")
        req.BoundingBox = r.URL.Query().Get("bounding_box")
    }
    // ... decode body for POST ...

    // Pass bounding box to camera-mgmt
    g.cameraSvc.SmartSearch(ctx, &damv1.SmartSearchRequest{
        // ...existing fields...
        BoundingBox: req.BoundingBox,
    })
}
```

- [ ] **Step 2: Add bounding box filter to camera-mgmt SmartSearch**

In `services/camera-mgmt/main.go`, add bounding box overlap check:

```go
if req.BoundingBox != "" {
    parts := strings.Split(req.BoundingBox, ",")
    if len(parts) == 4 {
        // PostGIS-style overlap: ST_Intersects on bounding boxes
        // Using raw JSONB overlap: bounding_box @> overlap_region
        query += fmt.Sprintf(" AND bounding_box @> $%d::jsonb", argIdx)
        args = append(args, fmt.Sprintf(
            `[%s,%s,%s,%s]`, parts[0], parts[1], parts[2], parts[3]))
        argIdx++
    }
}
```

Add `BoundingBox` field to `SmartSearchRequest` proto in `api/v1/camera.pb.go`:

```go
type SmartSearchRequest struct {
    CameraID      string  `json:"camera_id"`
    ObjectType    string  `json:"object_type"`
    MinConfidence float64 `json:"min_confidence"`
    StartTime     string  `json:"start_time"`
    EndTime       string  `json:"end_time"`
    Limit         int32   `json:"limit"`
    BoundingBox   string  `json:"bounding_box"`
}
```

- [ ] **Step 3: Add region selector UI component**

In `web/src/pages/SearchPage.tsx`, add a click-and-drag region selector:

```typescript
const [drawing, setDrawing] = useState(false);
const [region, setRegion] = useState<{x1: number; y1: number; x2: number; y2: number} | null>(null);
const containerRef = useRef<HTMLDivElement>(null);

const handleMouseDown = (e: React.MouseEvent) => {
    if (!drawing) return;
    const rect = containerRef.current!.getBoundingClientRect();
    setRegion({
        x1: (e.clientX - rect.left) / rect.width,
        y1: (e.clientY - rect.top) / rect.height,
        x2: (e.clientX - rect.left) / rect.width,
        y2: (e.clientY - rect.top) / rect.height,
    });
};

const handleMouseMove = (e: React.MouseEvent) => {
    if (!drawing || !region) return;
    const rect = containerRef.current!.getBoundingClientRect();
    setRegion(prev => prev ? { ...prev, x2: (e.clientX - rect.left) / rect.width, y2: (e.clientY - rect.top) / rect.height } : null);
};

// In render, add a toggle button + overlay:
<label className="flex items-center gap-2">
  <input type="checkbox" checked={drawing} onChange={e => setDrawing(e.target.checked)} />
  Draw motion region
</label>
{region && (
  <div className="relative border border-gray-600" ref={containerRef}
       onMouseDown={handleMouseDown} onMouseMove={handleMouseMove}>
    <div className="absolute border-2 border-red-500 bg-red-500/20"
         style={{ left: `${Math.min(region.x1, region.x2) * 100}%`, top: `${Math.min(region.y1, region.y2) * 100}%`,
                  width: `${Math.abs(region.x2 - region.x1) * 100}%`, height: `${Math.abs(region.y2 - region.y1) * 100}%` }} />
  </div>
)}
```

- [ ] **Step 4: Pass region to search API**

```typescript
const handleSearch = async () => {
    const bbox = region ? `${region.x1},${region.y1},${region.x2},${region.y2}` : undefined;
    const results = await api.smartSearch({ camera_id, object_type, min_confidence, start_time, end_time, bounding_box: bbox });
    // ...
};
```

Add to `api/client.ts`:

```typescript
interface SmartSearchParams {
    camera_id?: string;
    object_type?: string;
    min_confidence?: number;
    start_time?: string;
    end_time?: string;
    bounding_box?: string;
}
```

- [ ] **Step 5: Validate**

Run: `npx tsc --noEmit` in `web/` and `gofmt -d services/api-gateway/main.go services/camera-mgmt/main.go`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: smart search by motion region with click-draw bounding box"
```

---

### Task 5: Bookmarking

**Files:**
- Create: `services/recorder/bookmarks.go`
- Modify: `web/src/components/TimelineScrubber.tsx`
- Modify: `web/src/pages/RecordingsPage.tsx`
- Modify: `web/src/api/client.ts`
- Modify: `services/api-gateway/main.go`

- [ ] **Step 1: Create bookmarks Go service handler**

Create `services/recorder/bookmarks.go`:

```go
package main

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/jmoiron/sqlx"
)

type Bookmark struct {
    ID        string    `json:"id" db:"id"`
    CameraID  string    `json:"camera_id" db:"camera_id"`
    Timestamp time.Time `json:"timestamp" db:"timestamp"`
    Label     string    `json:"label" db:"label"`
    CreatedAt time.Time `json:"created_at" db:"created_at"`
    CreatedBy string    `json:"created_by" db:"created_by"`
}

func handleCreateBookmark(db *sqlx.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            CameraID  string `json:"camera_id"`
            Timestamp string `json:"timestamp"`
            Label     string `json:"label"`
            CreatedBy string `json:"created_by"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            jsonError(w, "invalid request", http.StatusBadRequest)
            return
        }

        var id string
        err := db.QueryRowContext(r.Context(),
            "INSERT INTO bookmarks (camera_id, timestamp, label, created_by) VALUES ($1, $2, $3, $4) RETURNING id",
            req.CameraID, req.Timestamp, req.Label, req.CreatedBy).Scan(&id)
        if err != nil {
            jsonError(w, "failed to create bookmark", http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusCreated)
        json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "created"})
    }
}

func handleListBookmarks(db *sqlx.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        cameraID := r.URL.Query().Get("camera_id")
        var bookmarks []Bookmark
        var err error
        if cameraID != "" {
            err = db.SelectContext(r.Context(), &bookmarks,
                "SELECT id, camera_id, timestamp, label, created_at, created_by FROM bookmarks WHERE camera_id = $1 ORDER BY timestamp DESC",
                cameraID)
        } else {
            err = db.SelectContext(r.Context(), &bookmarks,
                "SELECT id, camera_id, timestamp, label, created_at, created_by FROM bookmarks ORDER BY timestamp DESC LIMIT 100")
        }
        if err != nil {
            jsonError(w, "failed to list bookmarks", http.StatusInternalServerError)
            return
        }
        json.NewEncoder(w).Encode(map[string]interface{}{"bookmarks": bookmarks})
    }
}

func jsonError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
```

- [ ] **Step 2: Add migration for bookmarks table**

Append to `migrations/001_initial_schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS bookmarks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id),
    timestamp TIMESTAMPTZ NOT NULL,
    label TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    created_by TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_bookmarks_camera ON bookmarks(camera_id);
CREATE INDEX IF NOT EXISTS idx_bookmarks_time ON bookmarks(timestamp DESC);
```

- [ ] **Step 3: Add bookmark routes to recorder service**

In `services/recorder/main.go`, add routes:

```go
mux.HandleFunc("/bookmarks", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        handleListBookmarks(s.db)(w, r)
    case http.MethodPost:
        handleCreateBookmark(s.db)(w, r)
    default:
        jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
    }
})
```

- [ ] **Step 4: Add bookmark proxy route in gateway**

In `services/api-gateway/main.go`:

```go
case strings.HasPrefix(path, "/api/bookmarks"):
    g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
        r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
        g.playbackProxy.ServeHTTP(w, r)
    }))(w, r)
```

- [ ] **Step 5: Add bookmark UI to TimelineScrubber**

In `web/src/components/TimelineScrubber.tsx`, add bookmark markers and a B-key handler:

```typescript
interface Bookmark {
  id: string;
  camera_id: string;
  timestamp: string;
  label: string;
}

// Add state
const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
const [showBookmarkDialog, setShowBookmarkDialog] = useState(false);

// Load bookmarks
useEffect(() => {
  api.listBookmarks(cameraId).then(setBookmarks).catch(() => {});
}, [cameraId]);

// Keyboard shortcut
useEffect(() => {
  const handler = (e: KeyboardEvent) => {
    if (e.key === 'b' && !e.ctrlKey && !e.metaKey) {
      setShowBookmarkDialog(true);
    }
  };
  window.addEventListener('keydown', handler);
  return () => window.removeEventListener('keydown', handler);
}, []);

// Render bookmark pins
{bookmarks.map(bm => (
  <div key={bm.id}
    className="absolute top-0 w-1 h-full bg-yellow-400 cursor-pointer"
    style={{ left: `${(new Date(bm.timestamp).getTime() - startTime) / range * 100}%` }}
    title={bm.label}
  />
))}
```

Add bookmark dialog:

```typescript
{showBookmarkDialog && (
  <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
    <div className="bg-gray-800 p-4 rounded">
      <h3>Add Bookmark</h3>
      <input
        autoFocus
        className="w-full p-2 bg-gray-700 rounded mt-2"
        placeholder="Label (optional)"
        onKeyDown={async (e) => {
          if (e.key === 'Enter') {
            await api.createBookmark(cameraId, currentTime.toISOString(), (e.target as HTMLInputElement).value);
            setShowBookmarkDialog(false);
          }
          if (e.key === 'Escape') setShowBookmarkDialog(false);
        }}
      />
    </div>
  </div>
)}
```

- [ ] **Step 6: Add API methods**

In `web/src/api/client.ts`:

```typescript
async listBookmarks(cameraId?: string): Promise<Bookmark[]> {
  const params = cameraId ? `?camera_id=${cameraId}` : '';
  const resp = await this.fetch(`/api/bookmarks${params}`);
  const data = await resp.json();
  return data.bookmarks;
}

async createBookmark(cameraId: string, timestamp: string, label: string): Promise<void> {
  await this.fetch('/api/bookmarks', {
    method: 'POST',
    body: JSON.stringify({ camera_id: cameraId, timestamp, label, created_by: this.getUsername() }),
  });
}
```

- [ ] **Step 7: Validate**

Run: `npx tsc --noEmit` in `web/` and `gofmt -d services/recorder/bookmarks.go`

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "feat: bookmarking with B-key shortcut, timeline markers, and CRUD API"
```
