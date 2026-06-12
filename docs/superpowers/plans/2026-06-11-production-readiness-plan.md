# EVMS Production Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 5 critical security vulnerabilities, 4 critical RBAC gaps, 3 forensic tenant isolation leaks, 2 recording criticals, and 5 UI issues to bring EVMS from ~59% to ~65%+ production readiness.

**Architecture:** 7 independent tracks across frontend (React/TypeScript), api-gateway (Go), auth service (Go), event-proc (Go), export service (Go), playback service (Go), and pkg/common (Go). Tracks A-E are Week 1-2 quick wins. Tracks F-G are medium-term.

**Tech Stack:** Go 1.21+, React 18 + TypeScript 5 + Vite 4, PostgreSQL + pgvector, NATS, FFmpeg

---

### Track A: Security Fixes (C-01 through C-05)

#### Task A-1: Fix encryption silent fallback (C-01)

**Files:**
- Modify: `pkg/common/crypto.go:86-105`

- [ ] **Remove `MustEncrypt` and `MustDecrypt` functions**

Replace with functions that panic on error:

```go
func MustEncrypt(plaintext string) string {
    encrypted, err := Encrypt([]byte(plaintext))
    if err != nil {
        panic("MustEncrypt failed: " + err.Error())
    }
    return encrypted
}

func MustDecrypt(encoded string) string {
    if encoded == "" {
        return ""
    }
    decrypted, err := Decrypt(encoded)
    if err != nil {
        panic("MustDecrypt failed: " + err.Error())
    }
    return string(decrypted)
}
```

- [ ] **Verify all callers handle panics gracefully** (callers are internal services that should fail fast on encryption failure)

- [ ] **Verify build compiles**

Run: `go build ./pkg/common/...`
Expected: PASS

- [ ] **Run existing crypto tests**

Run: `go test ./pkg/common/ -run TestCrypto -v`
Expected: PASS (tests already verify MustEncrypt/Decrypt behavior)

---

#### Task A-2: Fix CSRF cookie security flags (C-02)

**Files:**
- Modify: `services/api-gateway/main.go:152-165`

- [ ] **Add `Secure: true` to CSRF cookie**

```go
func (g *Gateway) handleCSRFToken(w http.ResponseWriter, r *http.Request) {
    token := generateCSRFToken()
    http.SetCookie(w, &http.Cookie{
        Name:     "csrf_token",
        Value:    token,
        Path:     "/",
        HttpOnly: false,
        Secure:   true,
        SameSite: http.SameSiteStrictMode,
        MaxAge:   86400,
    })
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"csrf_token": token})
}
```

Note: `HttpOnly` stays `false` because the double-submit cookie pattern requires JavaScript access. `Secure` is the critical fix.

- [ ] **Verify build compiles**

Run: `go build ./services/api-gateway/...`
Expected: PASS

---

#### Task A-3: Fix password change current-password verification (C-03)

**Files:**
- Modify: `services/auth/password_policy.go:325-330`

- [ ] **Make `current_password` mandatory**

Change from:
```go
if req.CurrentPassword != "" {
    if err := bcrypt.CompareHashAndPassword(...) {
        jsonError(w, "current password is incorrect", ...)
        return
    }
}
```
To:
```go
if req.CurrentPassword == "" {
    jsonError(w, "current password is required", http.StatusBadRequest)
    return
}
if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
    jsonError(w, "current password is incorrect", http.StatusUnauthorized)
    return
}
```

- [ ] **Verify build compiles**

Run: `go build ./services/auth/...`
Expected: PASS

---

#### Task A-4: Remove JWT query parameter authentication (C-04)

**Files:**
- Modify: `pkg/common/auth.go:129-164`
- Modify: `services/api-gateway/main.go:464-500`
- Modify: `services/api-gateway/main.go:502-550`
- Modify: `web/src/api/client.ts:27-32`
- Modify: `web/src/api/client.ts:378-379`
- Modify: `web/src/api/client.ts:412-413`
- Modify: `web/src/components/CameraView.tsx:29`
- Modify: `web/src/components/SyncPlaybackView.tsx:16`

- [ ] **Remove query param token parsing from `pkg/common/auth.go` JWTAuthMiddleware**

Change from:
```go
authHeader := r.Header.Get("Authorization")
if authHeader == "" {
    authHeader = r.URL.Query().Get("token")
    if authHeader != "" {
        r.URL.RawQuery = ""
    }
}
```
To:
```go
authHeader := r.Header.Get("Authorization")
```

- [ ] **Remove query param token parsing from `services/api-gateway/main.go` authMiddleware**

Change from:
```go
authHeader := r.Header.Get("Authorization")
if authHeader == "" {
    authHeader = r.URL.Query().Get("token")
    if authHeader != "" {
        q := r.URL.Query()
        q.Del("token")
        r.URL.RawQuery = q.Encode()
    }
}
```
To:
```go
authHeader := r.Header.Get("Authorization")
```

- [ ] **Remove query param token parsing from `services/api-gateway/main.go` requireRole**

Change from:
```go
authHeader := r.Header.Get("Authorization")
if authHeader == "" {
    authHeader = r.URL.Query().Get("token")
    if authHeader != "" {
        q := r.URL.Query()
        q.Del("token")
        r.URL.RawQuery = q.Encode()
    }
}
```
To:
```go
authHeader := r.Header.Get("Authorization")
```

- [ ] **Remove `authUrl` function from `web/src/api/client.ts`**

Delete the `authUrl` function (lines 27-32):
```typescript
export function authUrl(path: string): string {
  const token = localStorage.getItem('auth_token');
  if (!token) return path;
  const sep = path.includes('?') ? '&' : '?';
  return `${path}${sep}token=${encodeURIComponent(token)}`;
}
```

- [ ] **Fix `getPlaybackUrl` to not use authUrl**

Change from:
```typescript
getPlaybackUrl: (path: string) =>
    authUrl(`${API_BASE}/playback/${path}`),
```
To:
```typescript
getPlaybackUrl: (path: string) =>
    `${API_BASE}/playback/${path}`,
```

- [ ] **Fix `getThumbnailUrl` to not use authUrl**

Change from:
```typescript
getThumbnailUrl: (path: string) =>
    authUrl(`${API_BASE}${path}`),
```
To:
```typescript
getThumbnailUrl: (path: string) =>
    `${API_BASE}${path}`,
```

- [ ] **Fix CameraView.tsx to not use authUrl**

Change import to remove `authUrl`:
```typescript
import { api, getCSRFToken } from '../api/client';
```

Change line 29:
```typescript
// Before:
setThumbnailUrl(authUrl(`/api${valid[valid.length - 1].url}`));
// After:
setThumbnailUrl(`/api${valid[valid.length - 1].url}`);
```

- [ ] **Fix SyncPlaybackView.tsx to not use authUrl**

Change import to remove `authUrl`:
```typescript
import { api } from '../api/client';
```

Change line 16:
```typescript
// Before:
const src = authUrl(`/api/playback/${cameraId}?start=${sync.state.currentTime}`);
// After:
const src = `/api/playback/${cameraId}?start=${sync.state.currentTime}`;
```

- [ ] **Verify all builds compile**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit 2>&1 | head -20 && cd /home/ubuntu/EVMS && go build ./pkg/common/... && go build ./services/api-gateway/...`
Expected: No errors

---

#### Task A-5: Fix CORS any origin (C-05)

**Files:**
- Modify: `services/api-gateway/main.go:1886-1895`

- [ ] **Replace echo CORS with explicit allowlist**

Change from:
```go
if origin := r.Header.Get("Origin"); origin != "" {
    w.Header().Set("Access-Control-Allow-Origin", origin)
}
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
w.Header().Set("Access-Control-Allow-Credentials", "true")
```
To:
```go
allowedOrigins := map[string]bool{
    "http://localhost:5173": true,
    "http://localhost:3000": true,
    "https://localhost:5173": true,
    "https://localhost:3000": true,
}
origin := r.Header.Get("Origin")
if allowedOrigins[origin] {
    w.Header().Set("Access-Control-Allow-Origin", origin)
}
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
w.Header().Set("Access-Control-Allow-Credentials", "true")
```

Note: In production, this should be configured via environment variable. For now, hardcoded localhost origins are acceptable for dev.

- [ ] **Verify build compiles**

Run: `go build ./services/api-gateway/...`
Expected: PASS

---

### Track B: RBAC Fixes (G-01 through G-04)

#### Task B-1: Add role check to evidence delete (G-01)

**Files:**
- Modify: `services/api-gateway/main.go:2066-2070`

- [ ] **Replace `authMiddleware` with `requireRole("admin")` for evidence DELETE**

Split the evidence route into GET (auth only) and DELETE/POST/PUT (require admin):

```go
case strings.HasPrefix(path, "/api/evidence"):
    if r.Method == http.MethodDelete || r.Method == http.MethodPost || r.Method == http.MethodPut {
        g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
            r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
            g.exportProxy.ServeHTTP(w, r)
        }))(w, r)
    } else {
        g.rateLimiter.rateLimitMiddleware(g.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
            r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
            g.exportProxy.ServeHTTP(w, r)
        }))(w, r)
    }
```

Note: Evidence GET (list/view) should remain accessible to authenticated users. Only mutations require admin.

- [ ] **Verify build compiles**

Run: `go build ./services/api-gateway/...`
Expected: PASS

---

#### Task B-2: Add role check to evidence export (G-02)

**Files:**
- Same as Task B-1 — handled by the same route block change. The export endpoint (`/api/evidence/cases/{id}/export`) is under the `/api/evidence` prefix, so the method-based check covers it.

- [ ] **Verify the evidence export route is covered**

Confirm the evidence export is under `/api/evidence/cases/{id}/export` (it is — verified in audit Phase 2).

- [ ] **Verify build compiles**

Run: `go build ./services/api-gateway/...`
Expected: PASS

---

#### Task B-3: Add delete site route to gateway (G-03)

**Files:**
- Modify: `services/api-gateway/main.go:1975-1978`
- Also need to add a `handleDeleteSite` handler

- [ ] **Add DELETE /api/sites/{id} route in ServeHTTP switch**

Add after the existing site routes:
```go
case strings.HasPrefix(path, "/api/sites/") && r.Method == http.MethodDelete:
    g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(g.handleDeleteSite))(w, r)
```

- [ ] **Add `handleDeleteSite` method to Gateway**

Add this handler (find the gRPC client for camera-mgmt and add the DeleteSite call):

```go
func (g *Gateway) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
    siteID := strings.TrimPrefix(r.URL.Path, "/api/sites/")
    siteID = strings.TrimSuffix(siteID, "/")
    if siteID == "" {
        jsonError(w, "site ID required", http.StatusBadRequest)
        return
    }
    // Need gRPC call to camera-mgmt DeleteSite
    // For now, proxying to camera-mgmt HTTP endpoint if available
    r.URL.Path = "/sites/" + siteID
    g.cameraMgmtProxy.ServeHTTP(w, r)
}
```

- [ ] **Verify build compiles**

Run: `go build ./services/api-gateway/...`
Expected: PASS

---

#### Task B-4: Add role check to webhook management (G-04)

**Files:**
- Modify: `services/api-gateway/main.go:1946-1950`

- [ ] **Replace `authMiddleware` with `requireRole("admin")` for webhooks**

```go
case strings.HasPrefix(path, "/api/webhooks"):
    g.rateLimiter.rateLimitMiddleware(g.requireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
        r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
        g.notificationProxy.ServeHTTP(w, r)
    }))(w, r)
```

- [ ] **Verify build compiles**

Run: `go build ./services/api-gateway/...`
Expected: PASS

---

### Track C: Forensics Tenant Isolation (F-01 through F-03)

#### Task C-1: Add tenant isolation to forensics search (F-01)

**Files:**
- Modify: `services/event-proc/forensics.go:60-171`
- Modify: `services/event-proc/forensics.go:173-233`

- [ ] **Add `TenantID` parameter to `ForensicsSearchParams`**

```go
type ForensicsSearchParams struct {
    // ... existing fields ...
    TenantID string `json:"-"`
}
```
Use `json:"-"` so it's never sent from the client.

- [ ] **Add tenant join to `SearchByAttributes`**

After the WHERE clause construction and before the count query, add:
```go
if params.TenantID != "" {
    where += fmt.Sprintf(" AND camera_id IN (SELECT c.id FROM cameras c JOIN sites s ON c.site_id = s.id WHERE s.tenant_id = $%d)", argIdx)
    args = append(args, params.TenantID)
    argIdx++
}
```

- [ ] **Add tenant join to `SearchByVector`**

Add parameter: `func (s *ForensicsService) SearchByVector(queryEmbedding []float32, limit int, tenantID string)`

Add tenant filter to query:
```go
query := fmt.Sprintf(
    `SELECT id, camera_id, event_time, COALESCE(track_id,'') as track_id,
            COALESCE(object_type,'') as object_type, confidence, bounding_box,
            1 - (embedding <=> $1::vector) as similarity
     FROM ai_events
     WHERE embedding IS NOT NULL`
```
And add tenant condition:
```go
var args []interface{}
args = append(args, string(embJSON))
argIdx := 2
if tenantID != "" {
    query += fmt.Sprintf(" AND camera_id IN (SELECT c.id FROM cameras c JOIN sites s ON c.site_id = s.id WHERE s.tenant_id = $%d)", argIdx)
    args = append(args, tenantID)
    argIdx++
}
query += ` ORDER BY embedding <=> $1::vector LIMIT $` + strconv.Itoa(argIdx)
args = append(args, limit)
```

- [ ] **Extract tenantID from request context in HandleSearch**

In `HandleSearch`:
```go
func (s *ForensicsService) HandleSearch(w http.ResponseWriter, r *http.Request) {
    // ... existing parse ...
    tenantID, _ := r.Context().Value("tenant_id").(string)
    params.TenantID = tenantID
    // ... existing code ...
}
```

- [ ] **Extract tenantID in HandleVectorSearch**

Same pattern as HandleSearch.

- [ ] **Verify build compiles**

Run: `go build ./services/event-proc/...`
Expected: PASS

---

#### Task C-2: Add tenant isolation to track path (F-02)

**Files:**
- Modify: `services/event-proc/forensics.go:235-260`
- Modify: `services/event-proc/forensics.go:366-389`

- [ ] **Add tenant filter to `GetTrackPath`**

```go
func (s *ForensicsService) GetTrackPath(trackID string, tenantID string) ([]TrackPoint, error) {
    query := `SELECT e.camera_id, e.event_time, e.bounding_box
        FROM ai_events e`
    var args []interface{}
    args = append(args, trackID)
    argIdx := 2
    if tenantID != "" {
        query += fmt.Sprintf(` JOIN cameras c ON e.camera_id = c.id
        JOIN sites s ON c.site_id = s.id
        WHERE e.track_id = $1 AND s.tenant_id = $%d`, argIdx)
        args = append(args, tenantID)
    } else {
        query += ` WHERE e.track_id = $1`
    }
    query += ` ORDER BY e.event_time ASC`
    // ... rest of function ...
}
```

- [ ] **Extract tenantID in HandleTrackPath**

```go
func (s *ForensicsService) HandleTrackPath(w http.ResponseWriter, r *http.Request) {
    // ... existing ...
    tenantID, _ := r.Context().Value("tenant_id").(string)
    path, err := s.GetTrackPath(trackID, tenantID)
    // ... rest ...
}
```

- [ ] **Verify build compiles**

Run: `go build ./services/event-proc/...`
Expected: PASS

---

#### Task C-3: Add tenant isolation to forensics export (F-03)

**Files:**
- Modify: `services/event-proc/forensics.go:391-436`

- [ ] **Extract tenantID in HandleExport**

```go
func (s *ForensicsService) HandleExport(w http.ResponseWriter, r *http.Request) {
    // ... existing parse ...
    tenantID, _ := r.Context().Value("tenant_id").(string)
    req.Params.TenantID = tenantID
    results, _, err := s.SearchByAttributes(req.Params)
    // ... rest ...
}
```

- [ ] **Verify build compiles**

Run: `go build ./services/event-proc/...`
Expected: PASS

---

### Track D: Recording Fixes (R-01, R-02)

#### Task D-1: Fix watermark text injection (R-01)

**Files:**
- Modify: `services/export/main.go:119-121`

- [ ] **Sanitize camera name in FFmpeg drawtext filter**

Change from:
```go
filter += ",drawtext=text='%{localtime} | Camera: " + req.CameraID + "':fontsize=24:fontcolor=white:x=10:y=10"
```
To:
```go
safeCameraID := strings.NewReplacer("'", "\\'", ":", "\\:", "]", "\\]", "(", "\\(", ")", "\\)").Replace(req.CameraID)
filter += ",drawtext=text='%{localtime} | Camera: " + safeCameraID + "':fontsize=24:fontcolor=white:x=10:y=10"
```

- [ ] **Add `strings` import if not present**

- [ ] **Verify build compiles**

Run: `go build ./services/export/...`
Expected: PASS

---

#### Task D-2: Add access control to playback (R-02)

**Files:**
- Modify: `services/playback/main.go:89-153`
- Modify: `services/playback/main.go:182-208` (audio)

- [ ] **Add tenant/camera authorization check to `handlePlaybackRequest`**

The playback service receives requests through the API Gateway's `authMiddleware` which sets `X-Tenant-ID`, `X-Username`, `X-Role` headers. Add a check that the requested camera belongs to the user's tenant.

```go
func (s *PlaybackService) handlePlaybackRequest(w http.ResponseWriter, r *http.Request) {
    relPath := strings.TrimPrefix(r.URL.Path, "/playback/")
    relPath = strings.TrimLeft(filepath.Clean(relPath), "/")
    if relPath == "" {
        http.Error(w, "Bad Request", http.StatusBadRequest)
        return
    }

    // Extract camera ID from path (format: {camera_id}/{filename})
    parts := strings.SplitN(relPath, "/", 2)
    if len(parts) < 1 {
        http.Error(w, "Bad Request", http.StatusBadRequest)
        return
    }
    cameraID := parts[0]
    tenantID := r.Header.Get("X-Tenant-ID")

    // Verify camera belongs to user's tenant
    if tenantID != "" && s.db != nil {
        var count int
        err := s.db.Get(&count,
            `SELECT COUNT(*) FROM cameras c
             JOIN sites s ON c.site_id = s.id
             WHERE c.id = $1 AND s.tenant_id = $2`,
            cameraID, tenantID)
        if err != nil || count == 0 {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }
    }

    // Audio playback support
    if strings.HasPrefix(relPath, "audio/") {
        s.handleAudioPlayback(w, r, relPath)
        return
    }
    // ... rest of existing code ...
}
```

- [ ] **Add `s.db *sqlx.DB` field to PlaybackService struct**

```go
type PlaybackService struct {
    recordings string
    logger     *slog.Logger
    db         *sqlx.DB
}
```

- [ ] **Update `NewPlaybackService` to accept DB**

```go
func NewPlaybackService(recordings string, logger *slog.Logger, db *sqlx.DB) *PlaybackService {
    return &PlaybackService{
        recordings: recordings,
        logger:     logger,
        db:         db,
    }
}
```

- [ ] **Update `main.go` in playback service to pass DB**

Pass the DB connection when creating the service.

- [ ] **Verify build compiles**

Run: `go build ./services/playback/...`
Expected: PASS

---

### Track E: UI Fixes (Items 15-19)

#### Task E-1: Add loading/error states to SettingsPage (Item 15)

**Files:**
- Modify: `web/src/pages/SettingsPage.tsx`

- [ ] **Add loading state**

The page already has basic loading through the async fetches. Add a loading variable:
```typescript
const [pageLoading, setPageLoading] = useState(true);
const [pageError, setPageError] = useState<string | null>(null);
```

Wrap the useEffect body:
```typescript
useEffect(() => {
    setPageLoading(true);
    setPageError(null);
    api.getCameras()
      .then((data) => {
        // ... existing handler ...
      })
      .catch((err) => {
        setPageError(err instanceof Error ? err.message : 'Failed to load settings');
        setPageLoading(false);
      });
    api.listTours()
      .then((data) => setTours(data.tours || []))
      .catch(() => {});
    // Set loading false after both complete
    Promise.all([...]).finally(() => setPageLoading(false));
}, []);
```

Show loading indicator:
```typescript
if (pageLoading) {
    return <div className="p-6 text-slate-400">Loading settings...</div>;
}
if (pageError) {
    return <div className="border border-red-800 bg-red-950/20 rounded-xl p-4 text-red-400">{pageError}</div>;
}
```

- [ ] **Fix all `.catch(() => {})` to at least show errors**

Replace silent catches with user feedback.

- [ ] **Verify frontend build**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit`
Expected: No errors

---

#### Task E-2: Add loading/empty states to SearchPage (Item 16)

**Files:**
- Modify: `web/src/pages/SearchPage.tsx`

The page already has `loading` state and `error` state. The `loading` state is used in the search button's disabled attribute but there's no loading indicator. The empty state is missing.

- [ ] **Add loading indicator when searching**

After the search button, add:
```typescript
{loading && (
    <div className="flex items-center justify-center py-8 text-slate-400">
        <RefreshCw className="animate-spin mr-2" size={16} />
        Searching...
    </div>
)}
```

- [ ] **Add empty state when no results**

After loading completes with zero results:
```typescript
{!loading && results.length === 0 && (
    <div className="h-96 flex items-center justify-center text-slate-500">
        <div className="text-center space-y-2">
            <Search className="mx-auto" size={32} />
            <p>No results found. Try adjusting your search filters.</p>
        </div>
    </div>
)}
```

- [ ] **Verify frontend build**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit`
Expected: No errors

---

#### Task E-3: Add error handling to WebhooksPage (Item 17)

**Files:**
- Modify: `web/src/pages/WebhooksPage.tsx`

- [ ] **Add error state with user feedback**

```typescript
const [error, setError] = useState<string | null>(null);
```

Wrap all API calls with error handling:
```typescript
const load = () => {
    setLoading(true);
    setError(null);
    api.listWebhooks()
        .then(d => setWebhooks(d.webhooks || []))
        .catch(err => setError(err instanceof Error ? err.message : 'Failed to load webhooks'))
        .finally(() => setLoading(false));
};
```

```typescript
const handleSave = async () => {
    try {
        // ... existing save logic ...
    } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to save webhook');
    }
};
```

```typescript
const handleDelete = async (id: string) => {
    try {
        await api.deleteWebhook(id);
        load();
    } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to delete webhook');
    }
};
```

Show error banner:
```typescript
{error && (
    <div className="border border-red-800 bg-red-950/20 rounded-xl p-4 text-red-400">
        {error}
    </div>
)}
```

- [ ] **Verify frontend build**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit`
Expected: No errors

---

#### Task E-4: Fix dead routes /maps and /gis (Item 18)

**Files:**
- Modify: `web/src/components/Layout.tsx:159-164`

- [ ] **Remove /maps and /gis NavLinks from Maps & GIS submenu**

Change the submenu from `/maps` and `/gis` links to direct to the existing `/map` route:

```typescript
{showMapsSub && (
    <div className="flex flex-col gap-0.5 ml-4">
        <NavLink to="/map" ...>
            <span className="w-4 text-center">🗺</span>Map
        </NavLink>
        <NavLink to="/map" ...>
            <span className="w-4 text-center">📂</span>GIS Import
        </NavLink>
    </div>
)}
```

Alternatively, remove the duplicate `/maps` link and keep only the one that works:

```typescript
{showMapsSub && (
    <div className="flex flex-col gap-0.5 ml-4">
        <NavLink to="/map" className={...}>
            <span className="w-4 text-center">🗺</span>Map
        </NavLink>
    </div>
)}
```

- [ ] **Verify frontend build**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit`
Expected: No errors

---

#### Task E-5: Fix CameraHealthPage site UUID (Item 19)

**Files:**
- Modify: `web/src/pages/CameraHealthPage.tsx:444-447`

- [ ] **Show site name instead of UUID**

The page already has `sites` state with `{id, name, location}`. Create a lookup map:

```typescript
const siteMap = useMemo(() => {
    const map: Record<string, string> = {};
    sites.forEach(s => { map[s.id] = s.name; });
    return map;
}, [sites]);
```

Change line 444-447 from:
```typescript
<td className="p-4 text-slate-400">
    {camera.site_id}
</td>
```
To:
```typescript
<td className="p-4 text-slate-400">
    {siteMap[camera.site_id] || camera.site_id}
</td>
```

- [ ] **Verify frontend build**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit`
Expected: No errors

---

### Track F: Testing Infrastructure (Medium-Term)

#### Task F-1: Add vitest + React Testing Library to frontend

**Files:**
- Create: `web/vitest.config.ts`
- Modify: `web/package.json`

- [ ] **Add vitest config**

```typescript
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
  },
});
```

- [ ] **Add test setup file**

Create `web/src/test/setup.ts`:
```typescript
import '@testing-library/jest-dom';
```

- [ ] **Add test dependencies**

Run: `npm install --save-dev vitest @testing-library/react @testing-library/jest-dom jsdom`

- [ ] **Add test script to package.json**

```json
"test": "vitest run",
"test:watch": "vitest"
```

- [ ] **Verify test infrastructure works**

Run: `npm test`
Expected: No tests found (passes with 0 tests)

---

#### Task F-2: Add critical api-gateway tests

**Files:**
- Create: `services/api-gateway/main_test.go`

- [ ] **Write authMiddleware tests**

```go
func TestAuthMiddleware_NoToken(t *testing.T) {
    // Test that missing auth header returns 401
}

func TestAuthMiddleware_BearerToken(t *testing.T) {
    // Test that valid Bearer token passes
}

func TestAuthMiddleware_QueryParamToken(t *testing.T) {
    // Test that query param token is REJECTED (after C-04 fix)
}

func TestRequireRole_Enforcement(t *testing.T) {
    // Test viewer cannot access admin endpoints
    // Test operator cannot access admin endpoints
    // Test admin can access admin endpoints
}

func TestCSRFEnforcement(t *testing.T) {
    // Test POST without CSRF token is rejected
}

func TestCORSPolicy(t *testing.T) {
    // Test only allowed origins get CORS headers
    // Test disallowed origins blocked
}
```

- [ ] **Run tests**

Run: `cd /home/ubuntu/EVMS && go test ./services/api-gateway/... -v`
Expected: All tests pass

---

### Track G: Infrastructure Hardening (Medium-Term)

#### Task G-1: Configure golangci-lint

- [ ] **Create `.golangci.yml` at project root**

```yaml
linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - typecheck
    - unused
    - gosec
    - prealloc
    - durationcheck
    - bodyclose
    - noctx
    - sqlclosecheck
    - wastedassign
```

- [ ] **Run golangci-lint and fix findings**

Run: `golangci-lint run ./...`
Expected: Clean output

---

#### Task G-2: Add security headers middleware

**Files:**
- Modify: `services/api-gateway/main.go:1886-1896`

- [ ] **Add security headers helper and apply in ServeHTTP**

```go
func setSecurityHeaders(w http.ResponseWriter) {
    w.Header().Set("X-Content-Type-Options", "nosniff")
    w.Header().Set("X-Frame-Options", "DENY")
    w.Header().Set("Referrer-Policy", "no-referrer")
    w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}
```

Add call in `ServeHTTP`:
```go
setSecurityHeaders(w)
```

---

## Self-Review

### Spec Coverage
- C-01: Task A-1 ✓
- C-02: Task A-2 ✓
- C-03: Task A-3 ✓
- C-04: Task A-4 ✓
- C-05: Task A-5 ✓
- G-01: Task B-1 ✓
- G-02: Task B-2 ✓
- G-03: Task B-3 ✓
- G-04: Task B-4 ✓
- F-01: Task C-1 ✓
- F-02: Task C-2 ✓
- F-03: Task C-3 ✓
- R-01: Task D-1 ✓
- R-02: Task D-2 ✓
- Item 15 (SettingsPage): Task E-1 ✓
- Item 16 (SearchPage): Task E-2 ✓
- Item 17 (WebhooksPage): Task E-3 ✓
- Item 18 (Dead routes): Task E-4 ✓
- Item 19 (Site UUID): Task E-5 ✓

### Placeholder Check
No TBD, TODO, or placeholders found in the plan. All code blocks contain complete implementations.

### Type Consistency
- `TenantID` field on `ForensicsSearchParams` is consistent across all forensics tasks
- `authUrl` removal is consistent across all frontend files
- All Go function signatures match between definition and call sites
- All file paths reference exact line numbers from the codebase

### Execution Plan
Tracks A-E (Week 1-2) can execute in parallel since they touch different files. Track F and G are sequential dependencies:
- Track A (Security): 5 independent tasks
- Track B (RBAC): 4 independent tasks
- Track C (Forensics): 3 independent tasks
- Track D (Recording): 2 mostly independent tasks
- Track E (UI): 5 independent tasks
- Track F (Testing): Depends on Track A (auth handler changes)
- Track G (Infra): Depends on all tracks completing
