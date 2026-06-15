# Camera Operations & Recording Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden camera operations (input validation, soft-delete, PTZ rate limiting, multi-probe health checks) and recording subsystem (integrity checks, async export queue).

**Architecture:** Six sequential phases, each producing standalone, testable changes. Phase 1-4 target `services/camera-mgmt/` and `services/camera-control/`. Phase 5-6 target `services/recorder/` and `services/export/`. Each phase adds migrations, modifies existing handlers, and adds tests.

**Tech Stack:** Go 1.22+, gRPC (camera-mgmt), HTTP REST (camera-control, export), NATS JetStream (export queue), PostgreSQL with TimescaleDB, `golang.org/x/time/rate` (PTZ limiting).

---

### Task 1: Camera Input Validation

**Files:**
- Modify: `services/camera-mgmt/main.go` — add validation function, modify CreateCamera and UpdateCamera
- Test: `services/camera-mgmt/main_test.go` — add validation unit tests

- [ ] **Step 1: Write the `validateCamera` function**

Add to `services/camera-mgmt/main.go`:

```go
type validationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

type validationErrors []validationError

func (v validationErrors) Error() string {
    var b strings.Builder
    for i, e := range v {
        if i > 0 {
            b.WriteString("; ")
        }
        b.WriteString(e.Field)
        b.WriteString(": ")
        b.WriteString(e.Message)
    }
    return b.String()
}

var validPTZProtocols = map[string]bool{
    "NONE": true, "onvif": true, "vapix": true, "hikvision": true,
}

func validateCamera(req *damv1.CreateCameraRequest, existing ...*Camera) error {
    var errs validationErrors

    if strings.TrimSpace(req.Name) == "" {
        errs = append(errs, validationError{"name", "name is required"})
    } else if len(req.Name) > 255 {
        errs = append(errs, validationError{"name", "name must be 255 characters or fewer"})
    }

    if strings.TrimSpace(req.SiteId) == "" {
        errs = append(errs, validationError{"site_id", "site_id is required"})
    }

    if strings.TrimSpace(req.ConnectionUrl) == "" {
        errs = append(errs, validationError{"connection_url", "connection_url is required"})
    } else {
        u, err := url.Parse(req.ConnectionUrl)
        if err != nil || (u.Scheme != "rtsp" && u.Scheme != "http" && u.Scheme != "https") {
            errs = append(errs, validationError{"connection_url", "must be a valid rtsp://, http://, or https:// URL"})
        }
    }

    if req.SubstreamUrl != "" {
        u, err := url.Parse(req.SubstreamUrl)
        if err != nil || (u.Scheme != "rtsp" && u.Scheme != "http" && u.Scheme != "https") {
            errs = append(errs, validationError{"substream_url", "must be a valid rtsp://, http://, or https:// URL"})
        }
    }

    if req.PtzProtocol != "" && !validPTZProtocols[req.PtzProtocol] {
        errs = append(errs, validationError{"ptz_protocol", "must be one of: NONE, onvif, vapix, hikvision"})
    }

    if req.RetentionDays < 0 || req.RetentionDays > 3650 {
        errs = append(errs, validationError{"retention_days", "must be between 0 and 3650"})
    }

    if req.PrerecordSeconds < 0 || req.PrerecordSeconds > 30 {
        errs = append(errs, validationError{"prerecord_seconds", "must be between 0 and 30"})
    }

    if (req.OnvifUsername != "") != (req.OnvifPassword != "") {
        errs = append(errs, validationError{"onvif_credentials", "both username and password are required when using ONVIF authentication"})
    }

    if len(errs) > 0 {
        return errs
    }
    return nil
}
```

Add imports: `"net/url"`, `"strings"`.

- [ ] **Step 2: Integrate validation into CreateCamera**

Replace lines 252-265 in `services/camera-mgmt/main.go` to call validation before the DB insert:

```go
func (s *CameraService) CreateCamera(ctx context.Context, req *damv1.CreateCameraRequest) (*damv1.Camera, error) {
    if err := validateCamera(req); err != nil {
        s.logger.Warn("Camera validation failed", "error", err)
        st, _ := status.New(codes.InvalidArgument, "validation failed").WithDetails(
            &errdetails.BadRequest_FieldViolation{Field: "camera", Description: err.Error()},
        )
        return nil, st.Err()
    }

    tenantID := common.TenantFromContext(ctx)
    if tenantID != "" {
        var siteTenantID string
        err := s.db.GetContext(ctx, &siteTenantID, "SELECT tenant_id::text FROM sites WHERE id = $1", req.SiteId)
        if err != nil {
            s.logger.Error("Site not found", "site_id", req.SiteId)
            return nil, status.Errorf(codes.NotFound, "site not found")
        }
        if siteTenantID != tenantID {
            s.logger.Warn("cross-tenant camera creation attempt", "tenant", tenantID, "site_tenant", siteTenantID)
            return nil, status.Errorf(codes.PermissionDenied, "cannot create camera in another tenant's site")
        }
    }

    if tenantID != "" {
        var dupCount int
        err := s.db.GetContext(ctx, &dupCount,
            "SELECT COUNT(*) FROM cameras c JOIN sites s ON c.site_id = s.id WHERE s.tenant_id = $1 AND c.connection_url = $2 AND c.deleted_at IS NULL",
            tenantID, req.ConnectionUrl)
        if err == nil && dupCount > 0 {
            return nil, status.Errorf(codes.AlreadyExists, "a camera with this connection URL already exists in this tenant")
        }
    }

    // ... rest of existing CreateCamera unchanged
```

Add import: `"google.golang.org/genproto/googleapis/rpc/errdetails"`.

- [ ] **Step 3: Integrate validation into UpdateCamera**

Modify UpdateCamera function. After the tenant check (line 297-305), before the existing defaults:

```go
func (s *CameraService) UpdateCamera(ctx context.Context, req *damv1.UpdateCameraRequest) (*damv1.Camera, error) {
    tenantID := common.TenantFromContext(ctx)
    if tenantID != "" {
        existing, err := s.cameraByIDWithTenant(ctx, req.Id, tenantID)
        if err != nil {
            s.logger.Error("Camera not found for tenant", "id", req.Id, "tenant", tenantID)
            return nil, status.Errorf(codes.NotFound, "camera not found")
        }
        _ = existing
    }

    // Convert update request to create-like struct for shared validation
    createReq := &damv1.CreateCameraRequest{
        SiteId:           req.SiteId,
        Name:             req.Name,
        Description:      req.Description,
        ConnectionUrl:    req.ConnectionUrl,
        SubstreamUrl:     req.SubstreamUrl,
        PtzProtocol:      req.PtzProtocol,
        RetentionDays:    req.RetentionDays,
        PrerecordSeconds: req.PrerecordSeconds,
        OnvifUsername:    req.OnvifUsername,
        OnvifPassword:    req.OnvifPassword,
    }
    if err := validateCamera(createReq); err != nil {
        s.logger.Warn("Camera validation failed", "error", err)
        st, _ := status.New(codes.InvalidArgument, "validation failed").WithDetails(
            &errdetails.BadRequest_FieldViolation{Field: "camera", Description: err.Error()},
        )
        return nil, st.Err()
    }

    // ... rest of existing UpdateCamera unchanged
```

- [ ] **Step 4: Write validation unit tests**

Add to `services/camera-mgmt/main_test.go`:

```go
func TestValidateCamera(t *testing.T) {
    tests := []struct {
        name    string
        req     *damv1.CreateCameraRequest
        wantErr bool
    }{
        {"valid minimal", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example/stream"}, false},
        {"valid full", &damv1.CreateCameraRequest{
            SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example/stream",
            SubstreamUrl: "rtsp://example/sub", PtzProtocol: "onvif",
            RetentionDays: 30, PrerecordSeconds: 5,
            OnvifUsername: "admin", OnvifPassword: "pass",
        }, false},
        {"empty name", &damv1.CreateCameraRequest{SiteId: "s1", ConnectionUrl: "rtsp://example/stream"}, true},
        {"name too long", &damv1.CreateCameraRequest{SiteId: "s1", Name: strings.Repeat("a", 256), ConnectionUrl: "rtsp://example/stream"}, true},
        {"empty site_id", &damv1.CreateCameraRequest{Name: "cam", ConnectionUrl: "rtsp://example/stream"}, true},
        {"empty connection_url", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam"}, true},
        {"invalid connection_url", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "ftp://bad"}, true},
        {"invalid substream_url", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://good", SubstreamUrl: "ftp://bad"}, true},
        {"invalid ptz_protocol", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", PtzProtocol: "invalid"}, true},
        {"retention_days too high", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", RetentionDays: 9999}, true},
        {"prerecord_seconds too high", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", PrerecordSeconds: 60}, true},
        {"onvif username without password", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", OnvifUsername: "admin"}, true},
        {"onvif password without username", &damv1.CreateCameraRequest{SiteId: "s1", Name: "cam", ConnectionUrl: "rtsp://example", OnvifPassword: "pass"}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateCamera(tt.req)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

Add imports: `"strings"`, `damv1 "github.com/dam-vms/dam/api/v1"`.

- [ ] **Step 5: Run tests to verify**

Run: `cd /home/ubuntu/EVMS && go test ./services/camera-mgmt/... -v -run TestValidateCamera`
Expected: All test cases pass

- [ ] **Step 6: Verify build**

Run: `cd /home/ubuntu/EVMS && go build ./services/camera-mgmt/...`
Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
git add services/camera-mgmt/main.go services/camera-mgmt/main_test.go
git commit -m "feat(camera-mgmt): add input validation for camera create/update"
```

---

### Task 2: Soft-delete & Cascade Validation

**Files:**
- Create: `migrations/041_soft_delete.up.sql`
- Create: `migrations/041_soft_delete.down.sql`
- Modify: `services/camera-mgmt/main.go`

- [ ] **Step 1: Create migration 041 up**

Write `migrations/041_soft_delete.up.sql`:

```sql
ALTER TABLE cameras ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_cameras_deleted_at ON cameras(deleted_at) WHERE deleted_at IS NOT NULL;
```

- [ ] **Step 2: Create migration 041 down**

Write `migrations/041_soft_delete.down.sql`:

```sql
DROP INDEX IF EXISTS idx_cameras_deleted_at;
ALTER TABLE cameras DROP COLUMN IF EXISTS deleted_at;
```

- [ ] **Step 3: Add `deleted_at` to the Camera struct**

Add field to the `Camera` struct in `services/camera-mgmt/main.go`:

```go
DeletedAt *time.Time `db:"deleted_at"`
```

- [ ] **Step 4: Add `Hard` and `Force` fields to proto DeleteCameraRequest**

Edit `api/proto/v1/camera.proto`, add to `DeleteCameraRequest`:

```protobuf
message DeleteCameraRequest {
  string id = 1;
  bool hard = 2;
  bool force = 3;
}
```

Regenerate protos: Run `make proto` or equivalent.

- [ ] **Step 5: Modify DeleteCamera for soft-delete**

Replace the existing `DeleteCamera` function:

```go
func (s *CameraService) DeleteCamera(ctx context.Context, req *damv1.DeleteCameraRequest) (*damv1.DeleteCameraResponse, error) {
    tenantID := common.TenantFromContext(ctx)
    if tenantID != "" {
        existing, err := s.cameraByIDWithTenant(ctx, req.Id, tenantID)
        if err != nil {
            s.logger.Error("Camera not found for tenant", "id", req.Id, "tenant", tenantID)
            return nil, status.Errorf(codes.NotFound, "camera not found")
        }
        _ = existing
    }

    if req.Hard {
        var recordingCount int
        err := s.db.GetContext(ctx, &recordingCount, "SELECT COUNT(*) FROM recordings WHERE camera_id = $1", req.Id)
        if err != nil {
            return nil, status.Errorf(codes.Internal, "failed to check recordings: %v", err)
        }
        if recordingCount > 1000 && !req.Force {
            return nil, status.Errorf(codes.FailedPrecondition, "camera has %d recordings; use force=true to delete", recordingCount)
        }
        _, err = s.db.ExecContext(ctx, "DELETE FROM cameras WHERE id = $1 AND deleted_at IS NOT NULL", req.Id)
        if err != nil {
            return nil, status.Errorf(codes.Internal, "failed to hard delete camera: %v", err)
        }
    } else {
        _, err := s.db.ExecContext(ctx, "UPDATE cameras SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL", req.Id)
        if err != nil {
            return nil, status.Errorf(codes.Internal, "failed to delete camera: %v", err)
        }
    }

    return &damv1.DeleteCameraResponse{Success: true}, nil
}
```

- [ ] **Step 6: Add deleted_at filter to all camera queries**

In `ListCameras`, `GetCamera`, and `cameraByIDWithTenant`: append `AND c.deleted_at IS NULL` to all WHERE clauses.

For `camerasSelectCols` constant (line 168), it's a SELECT — no change needed.

For `ListCameras` (line 197):
- Line 206: append ` AND c.deleted_at IS NULL`
- Line 210: append ` AND c.deleted_at IS NULL`
- Line 216: append ` AND c.deleted_at IS NULL`
- Line 220: append ` AND c.deleted_at IS NULL`

For `cameraByIDWithTenant` (line 178):
- Line 183: append ` AND c.deleted_at IS NULL`
- Line 187: append ` AND c.deleted_at IS NULL`

For `GetCamera`: delegates to `cameraByIDWithTenant` — no direct change needed.

- [ ] **Step 7: Add site delete cascade check**

In `ListSites` / `DeleteSite` handler area, add a function:

```go
func (s *CameraService) canDeleteSite(ctx context.Context, siteID string) (bool, int, error) {
    var count int
    err := s.db.GetContext(ctx, &count,
        "SELECT COUNT(*) FROM cameras WHERE site_id = $1 AND deleted_at IS NULL", siteID)
    return count == 0, count, err
}
```

If `canDeleteSite` returns `count > 0`, return `FAILED_PRECONDITION` with `"site has N cameras, remove them first"`.

- [ ] **Step 8: Add background cleanup goroutine**

In `Start()` method, add after line 163:

```go
go func() {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            _, err := s.db.Exec("DELETE FROM recordings WHERE camera_id IN (SELECT id FROM cameras WHERE deleted_at < NOW() - INTERVAL '30 days')")
            if err != nil {
                s.logger.Error("Failed to cleanup orphaned recordings", "error", err)
            }
            _, err = s.db.Exec("DELETE FROM cameras WHERE deleted_at < NOW() - INTERVAL '30 days'")
            if err != nil {
                s.logger.Error("Failed to cleanup soft-deleted cameras", "error", err)
            }
        }
    }
}()
```

- [ ] **Step 9: Update proto-generated Go code**

Run: `cd /home/ubuntu/EVMS && protoc --go_out=. --go-grpc_out=. api/proto/v1/camera.proto` (or whatever the proto generation command is). Check `Makefile` for the exact command.

- [ ] **Step 10: Verify build**

Run: `cd /home/ubuntu/EVMS && go build ./services/camera-mgmt/...`
Expected: Build succeeds

- [ ] **Step 11: Commit**

```bash
git add migrations/041_soft_delete.up.sql migrations/041_soft_delete.down.sql \
      services/camera-mgmt/main.go api/proto/v1/camera.proto api/v1/
git commit -m "feat(camera-mgmt): add soft-delete and cascade validation"
```

---

### Task 3: PTZ Rate Limiting

**Files:**
- Modify: `services/camera-control/main.go`
- Test: `services/camera-control/main_test.go`

- [ ] **Step 1: Add rate limiter types and constructor**

After the `PTZConfig` type in `services/camera-control/main.go`, add:

```go
import (
    "golang.org/x/time/rate"
)

type ptzRateLimiter struct {
    mu         sync.Mutex
    limiters   map[string]*rate.Limiter
    lastCmd    map[string]time.Time
    semaphores map[string]chan struct{}
    config     *PTZRateLimitConfig
}

type PTZRateLimitConfig struct {
    Rate       rate.Limit
    Burst      int
    Cooldown   time.Duration
    Concurrency int
}

func defaultPTZRateLimitConfig() *PTZRateLimitConfig {
    ratePerSec := getEnvInt("PTZ_RATE_LIMIT", 5)
    cooldownMs := getEnvInt("PTZ_COOLDOWN_MS", 200)
    concurrency := getEnvInt("PTZ_CONCURRENCY", 2)
    return &PTZRateLimitConfig{
        Rate:        rate.Limit(ratePerSec),
        Burst:       ratePerSec,
        Cooldown:    time.Duration(cooldownMs) * time.Millisecond,
        Concurrency: concurrency,
    }
}

func newPTZRateLimiter(cfg *PTZRateLimitConfig) *ptzRateLimiter {
    return &ptzRateLimiter{
        limiters:   make(map[string]*rate.Limiter),
        lastCmd:    make(map[string]time.Time),
        semaphores: make(map[string]chan struct{}),
        config:     cfg,
    }
}

func (rl *ptzRateLimiter) acquire(cameraID string) error {
    rl.mu.Lock()
    lim, ok := rl.limiters[cameraID]
    if !ok {
        lim = rate.NewLimiter(rl.config.Rate, rl.config.Burst)
        rl.limiters[cameraID] = lim
    }
    lastTime, lastOk := rl.lastCmd[cameraID]
    sem, semOk := rl.semaphores[cameraID]
    if !semOk {
        sem = make(chan struct{}, rl.config.Concurrency)
        rl.semaphores[cameraID] = sem
    }
    rl.mu.Unlock()

    if lastOk {
        elapsed := time.Since(lastTime)
        if elapsed < rl.config.Cooldown {
            return fmt.Errorf("rate limit: cooldown %v remaining", rl.config.Cooldown-elapsed)
        }
    }

    if !lim.Allow() {
        return fmt.Errorf("rate limit: too many PTZ requests")
    }

    select {
    case sem <- struct{}{}:
    default:
        return fmt.Errorf("rate limit: max concurrent PTZ commands (%d) reached", rl.config.Concurrency)
    }

    rl.mu.Lock()
    rl.lastCmd[cameraID] = time.Now()
    rl.mu.Unlock()
    return nil
}

func (rl *ptzRateLimiter) release(cameraID string) {
    rl.mu.Lock()
    sem, ok := rl.semaphores[cameraID]
    rl.mu.Unlock()
    if ok {
        <-sem
    }
}
```

If `common.GetEnvInt` doesn't exist, define a local helper:

```go
func getEnvInt(key string, defaultVal int) int {
    val := os.Getenv(key)
    if val == "" {
        return defaultVal
    }
    n, err := strconv.Atoi(val)
    if err != nil {
        return defaultVal
    }
    return n
}
```

Add imports: `"golang.org/x/time/rate"`, `"sync"`, `"strconv"`.

- [ ] **Step 2: Add ptzLimiter field to PTZService struct**

Add field to `PTZService`:

```go
type PTZService struct {
    config        *PTZConfig
    logger        *slog.Logger
    cameraCC      *grpc.ClientConn
    cameraSvc     damv1.CameraServiceClient
    httpCli       *http.Client
    server        *http.Server
    healthHandler *common.HealthHandler
    ptzLimiter    *ptzRateLimiter   // <-- add this
}
```

- [ ] **Step 3: Initialize rate limiter in NewPTZService**

In the constructor, after `httpCli` line, add:

```go
ptzLimiter: newPTZRateLimiter(defaultPTZRateLimitConfig()),
```

- [ ] **Step 4: Wrap PTZ actions with rate limit in handlePTZRouter**

In `handlePTZRouter`, after the camera lookup succeeds (line 140), add rate limit check:

```go
if err := s.ptzLimiter.acquire(cameraID); err != nil {
    s.logger.Warn("PTZ rate limit exceeded", "camera_id", cameraID, "error", err)
    w.Header().Set("Retry-After", "1")
    jsonError(w, fmt.Sprintf("rate limit exceeded: %v", err), http.StatusTooManyRequests)
    return
}
defer s.ptzLimiter.release(cameraID)
```

Add this right after the camera fetch error check and before the switch statement on line 142.

- [ ] **Step 5: Write rate limiter unit test**

Add to `services/camera-control/main_test.go`:

```go
func TestPTZRateLimiter(t *testing.T) {
    cfg := &PTZRateLimitConfig{
        Rate:        10,
        Burst:       10,
        Cooldown:    10 * time.Millisecond,
        Concurrency: 2,
    }
    rl := newPTZRateLimiter(cfg)
    camID := "test-cam-1"

    // First request should succeed
    assert.NoError(t, rl.acquire(camID))
    rl.release(camID)

    // Concurrency test: acquire 2 should succeed, 3rd should fail
    assert.NoError(t, rl.acquire(camID))
    assert.NoError(t, rl.acquire(camID))
    err := rl.acquire(camID)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "max concurrent")
    rl.release(camID)
    rl.release(camID)

    // After release, should succeed again
    assert.NoError(t, rl.acquire(camID))
    rl.release(camID)
}
```

- [ ] **Step 6: Verify build and tests**

Run: `cd /home/ubuntu/EVMS && go build ./services/camera-control/... && go test ./services/camera-control/... -v -run TestPTZRateLimiter`
Expected: Build + test pass

- [ ] **Step 7: Commit**

```bash
git add services/camera-control/main.go services/camera-control/main_test.go
git commit -m "feat(camera-control): add PTZ rate limiting with per-camera token bucket"
```

---

### Task 4: Multi-probe Health Checks

**Files:**
- Create: `services/camera-mgmt/health.go`
- Create: `migrations/042_health_timestamps.up.sql`
- Create: `migrations/042_health_timestamps.down.sql`
- Modify: `services/camera-mgmt/main.go`
- Test: `services/camera-mgmt/main_test.go`

- [ ] **Step 1: Create migration 042 up**

Write `migrations/042_health_timestamps.up.sql`:

```sql
ALTER TABLE cameras ADD COLUMN last_seen_online TIMESTAMPTZ;
ALTER TABLE cameras ADD COLUMN last_status_change TIMESTAMPTZ;
```

- [ ] **Step 2: Create migration 042 down**

Write `migrations/042_health_timestamps.down.sql`:

```sql
ALTER TABLE cameras DROP COLUMN IF EXISTS last_seen_online;
ALTER TABLE cameras DROP COLUMN IF EXISTS last_status_change;
```

- [ ] **Step 3: Create health.go with probe functions**

Write `services/camera-mgmt/health.go`:

```go
package main

import (
    "bufio"
    "context"
    "fmt"
    "log/slog"
    "net"
    "net/url"
    "strings"
    "time"

    "github.com/dam-vms/dam/pkg/onvif"
    "github.com/dam-vms/dam/pkg/onvif/device"
)

type healthProbeResult int

const (
    probeOnline  healthProbeResult = 2
    probeDegraded healthProbeResult = 1
    probeOffline healthProbeResult = 0
)

type healthConfig struct {
    tcpTimeout    time.Duration
    rtspTimeout   time.Duration
    onvifTimeout  time.Duration
}

func defaultHealthConfig() *healthConfig {
    return &healthConfig{
        tcpTimeout:   getEnvDuration("TCP_PROBE_TIMEOUT", 3*time.Second),
        rtspTimeout:  getEnvDuration("RTSP_PROBE_TIMEOUT", 5*time.Second),
        onvifTimeout: getEnvDuration("ONVIF_PROBE_TIMEOUT", 5*time.Second),
    }
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
    val := os.Getenv(key)
    if val == "" {
        return defaultVal
    }
    d, err := time.ParseDuration(val)
    if err != nil {
        return defaultVal
    }
    return d
}

func probeTCP(addr string, timeout time.Duration) bool {
    conn, err := net.DialTimeout("tcp", addr, timeout)
    if err != nil {
        return false
    }
    conn.Close()
    return true
}

func probeRTSP(rawURL string, timeout time.Duration) bool {
    u, err := url.Parse(rawURL)
    if err != nil {
        return false
    }
    host := u.Host
    if host == "" {
        return false
    }
    conn, err := net.DialTimeout("tcp", host, timeout)
    if err != nil {
        return false
    }
    defer conn.Close()
    conn.SetDeadline(time.Now().Add(timeout))
    req := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: 1\r\n\r\n", rawURL)
    if _, err := conn.Write([]byte(req)); err != nil {
        return false
    }
    resp, err := bufio.NewReader(conn).ReadString('\n')
    if err != nil {
        return false
    }
    return strings.Contains(resp, "200 OK") || strings.Contains(resp, "401") || strings.Contains(resp, "301")
}

func probeONVIF(host string, port int, timeout time.Duration) bool {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    dev, err := onvif.NewDevice(onvif.DeviceParams{
        Xaddr:    fmt.Sprintf("%s:%d", host, port),
        Timeout:  timeout,
    })
    if err != nil {
        return false
    }
    resp, err := device.GetDeviceInformation(ctx, dev)
    if err != nil {
        return false
    }
    return resp != nil
}
```

- [ ] **Step 4: Add fields to Camera struct for new timestamps**

```go
LastSeenOnline  *time.Time `db:"last_seen_online"`
LastStatusChange *time.Time `db:"last_status_change"`
```

- [ ] **Step 5: Update `camerasSelectCols` to include new columns**

Replace the existing constant:

```go
const camerasSelectCols = "c.id, c.site_id, c.name, c.description, c.connection_url, c.substream_url, c.status, c.ptz_protocol, c.retention_days, COALESCE(c.prerecord_seconds, 0) AS prerecord_seconds, COALESCE(c.onvif_data, '{}'::jsonb) AS onvif_data, c.onvif_username, c.onvif_password, COALESCE(c.config, '{}'::jsonb) AS config, c.created_at, c.deleted_at, c.last_seen_online, c.last_status_change"
```

- [ ] **Step 6: Replace `checkCameraReachable` with multi-probe**

Replace the existing `checkCameraReachable` function:

```go
func (s *CameraService) checkCameraReachable(connectionURL string) healthProbeResult {
    u, err := url.Parse(connectionURL)
    if err != nil {
        return probeOffline
    }
    host := u.Host
    if host == "" {
        return probeOffline
    }

    // TCP probe is mandatory for any status
    cfg := defaultHealthConfig()
    tcpOK := probeTCP(host, cfg.tcpTimeout)
    if !tcpOK {
        return probeOffline
    }

    // RTSP probe
    rtspOK := probeRTSP(connectionURL, cfg.rtspTimeout)

    // ONVIF probe: try port 8000 (default) or from config
    onvifOK := probeONVIF(host, getONVIFPort(""), cfg.onvifTimeout)

    if rtspOK || onvifOK {
        return probeOnline
    }
    return probeDegraded
}

func getONVIFPort(configJSON string) int {
    // Default ONVIF port
    return 8000
}
```

Add import: `"net/url"`.

- [ ] **Step 7: Update `runHealthCheck` for new status logic**

Replace the existing `runHealthCheck` function:

```go
func (s *CameraService) runHealthCheck() {
    var cameras []Camera
    err := s.db.Select(&cameras, "SELECT "+camerasSelectCols+" FROM cameras c WHERE c.deleted_at IS NULL")
    if err != nil {
        s.logger.Error("Health check: failed to query cameras", "error", err)
        return
    }

    now := time.Now()
    for _, c := range cameras {
        result := s.checkCameraReachable(c.ConnectionURL)
        var newStatus string
        switch result {
        case probeOnline:
            newStatus = "online"
        case probeDegraded:
            newStatus = "degraded"
        default:
            newStatus = "offline"
        }

        if c.Status != newStatus {
            _, err := s.db.Exec(
                "UPDATE cameras SET status = $1, last_status_change = $2, last_seen_online = CASE WHEN $1 = 'online' THEN $2 ELSE last_seen_online END, updated_at = NOW() WHERE id = $3",
                newStatus, now, c.ID)
            if err != nil {
                s.logger.Error("Health check: failed to update camera status", "id", c.ID, "status", newStatus, "error", err)
            } else {
                s.logger.Info("Health check: camera status changed",
                    "id", c.ID, "name", c.Name, "from", c.Status, "to", newStatus)
            }
        } else if result == probeOnline && c.LastSeenOnline == nil {
            // First time seeing this camera online
            s.db.Exec("UPDATE cameras SET last_seen_online = $1 WHERE id = $2", now, c.ID)
        }
    }
}
```

- [ ] **Step 8: Make health check interval configurable**

In `startHealthCheck`, read interval from env:

```go
func (s *CameraService) startHealthCheck(ctx context.Context) {
    interval := getEnvDuration("HEALTH_CHECK_INTERVAL", 30*time.Second)
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    s.logger.Info("Starting camera health check loop", "interval", interval)
    for {
        select {
        case <-ctx.Done():
            s.logger.Info("Camera health check stopped")
            return
        case <-ticker.C:
            s.runHealthCheck()
        }
    }
}
```

- [ ] **Step 9: Write health probe unit tests**

Add to `services/camera-mgmt/main_test.go`:

```go
func TestProbeTCP(t *testing.T) {
    // Start a local TCP server
    ln, err := net.Listen("tcp", "127.0.0.1:0")
    require.NoError(t, err)
    defer ln.Close()

    go func() {
        conn, _ := ln.Accept()
        if conn != nil {
            conn.Close()
        }
    }()

    assert.True(t, probeTCP(ln.Addr().String(), time.Second))
    assert.False(t, probeTCP("127.0.0.1:19999", 100*time.Millisecond))
}

func TestProbeRTSP(t *testing.T) {
    // Start a mock RTSP server
    ln, err := net.Listen("tcp", "127.0.0.1:0")
    require.NoError(t, err)
    defer ln.Close()

    go func() {
        conn, _ := ln.Accept()
        if conn != nil {
            conn.Write([]byte("RTSP/1.0 200 OK\r\n"))
            conn.Close()
        }
    }()

    url := fmt.Sprintf("rtsp://%s/stream", ln.Addr().String())
    assert.True(t, probeRTSP(url, time.Second))
}

func TestHealthCheckStatusChange(t *testing.T) {
    cfg := &healthConfig{
        tcpTimeout:   time.Second,
        rtspTimeout:  time.Second,
        onvifTimeout: time.Second,
    }
    assert.Equal(t, 3*time.Second, cfg.tcpTimeout*3)
    // Ensures health config defaults are reasonable
    _ = defaultHealthConfig()
}
```

- [ ] **Step 10: Verify build**

Run: `cd /home/ubuntu/EVMS && go build ./services/camera-mgmt/...`
Expected: Build succeeds

- [ ] **Step 11: Commit**

```bash
git add migrations/042_health_timestamps.up.sql migrations/042_health_timestamps.down.sql \
      services/camera-mgmt/health.go services/camera-mgmt/main.go services/camera-mgmt/main_test.go
git commit -m "feat(camera-mgmt): add RTSP/ONVIF multi-probe health checks with degraded state"
```

---

### Task 5: Recording Integrity

**Files:**
- Create: `services/recorder/integrity.go`
- Create: `migrations/043_recording_integrity.up.sql`
- Create: `migrations/043_recording_integrity.down.sql`
- Modify: `services/recorder/main.go`

- [ ] **Step 1: Create migration 043 up**

Write `migrations/043_recording_integrity.up.sql`:

```sql
ALTER TABLE recordings ADD COLUMN sha256 TEXT;
ALTER TABLE recordings ADD COLUMN last_verified TIMESTAMPTZ;

CREATE TABLE recording_gaps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  camera_id UUID NOT NULL REFERENCES cameras(id),
  expected_start TIMESTAMPTZ NOT NULL,
  actual_start TIMESTAMPTZ NOT NULL,
  gap_seconds INTEGER NOT NULL,
  detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_recording_gaps_camera ON recording_gaps(camera_id);
```

- [ ] **Step 2: Create migration 043 down**

Write `migrations/043_recording_integrity.down.sql`:

```sql
DROP TABLE IF EXISTS recording_gaps;
ALTER TABLE recordings DROP COLUMN IF EXISTS last_verified;
ALTER TABLE recordings DROP COLUMN IF EXISTS sha256;
```

- [ ] **Step 3: Create `services/recorder/integrity.go`**

```go
package main

import (
    "context"
    "crypto/sha256"
    "fmt"
    "io"
    "log/slog"
    "math/rand"
    "os"
    "time"

    "github.com/jmoiron/sqlx"
)

type GapDetector struct {
    db     *sqlx.DB
    logger *slog.Logger
}

type IntegrityVerifier struct {
    db     *sqlx.DB
    logger *slog.Logger
}

func NewGapDetector(db *sqlx.DB, logger *slog.Logger) *GapDetector {
    return &GapDetector{db: db, logger: logger}
}

func NewIntegrityVerifier(db *sqlx.DB, logger *slog.Logger) *IntegrityVerifier {
    return &IntegrityVerifier{db: db, logger: logger}
}

func (gd *GapDetector) Run(ctx context.Context) {
    ticker := time.NewTicker(15 * time.Minute)
    defer ticker.Stop()
    gd.logger.Info("Starting gap detector", "interval", "15m")
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            gd.detectGaps(ctx)
        }
    }
}

func (gd *GapDetector) detectGaps(ctx context.Context) {
    var cameras []string
    err := gd.db.SelectContext(ctx, &cameras,
        "SELECT DISTINCT camera_id FROM recordings WHERE start_time > NOW() - INTERVAL '24 hours'")
    if err != nil {
        gd.logger.Error("Gap detector: failed to query cameras", "error", err)
        return
    }

    const maxGap = 65 * time.Second
    for _, camID := range cameras {
        type seg struct {
            StartTime time.Time `db:"start_time"`
            EndTime   time.Time `db:"end_time"`
        }
        var segments []seg
        err := gd.db.SelectContext(ctx, &segments,
            "SELECT start_time, end_time FROM recordings WHERE camera_id = $1 AND start_time > NOW() - INTERVAL '24 hours' ORDER BY start_time",
            camID)
        if err != nil {
            gd.logger.Error("Gap detector: failed to query segments", "camera_id", camID, "error", err)
            continue
        }

        for i := 1; i < len(segments); i++ {
            gap := segments[i].StartTime.Sub(segments[i-1].EndTime)
            if gap > maxGap {
                gd.logger.Error("Recording gap detected",
                    "camera_id", camID,
                    "expected_start", segments[i-1].EndTime,
                    "actual_start", segments[i].StartTime,
                    "gap_seconds", int(gap.Seconds()))
                common.RecordingGaps.WithLabelValues(camID).Inc()
            }
        }
    }
}

func computeSHA256(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil {
        return "", fmt.Errorf("failed to open file for checksum: %w", err)
    }
    defer f.Close()
    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return "", fmt.Errorf("failed to compute checksum: %w", err)
    }
    return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (iv *IntegrityVerifier) Run(ctx context.Context) {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    iv.logger.Info("Starting integrity verifier", "interval", "24h")
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            iv.verifyIntegrity(ctx)
        }
    }
}

func (iv *IntegrityVerifier) verifyIntegrity(ctx context.Context) {
    type recordingCheck struct {
        ID       string `db:"id"`
        FilePath string `db:"file_path"`
        SHA256   string `db:"sha256"`
    }
    var recordings []recordingCheck
    err := iv.db.SelectContext(ctx, &recordings,
        `SELECT id, file_path, sha256 FROM recordings
         WHERE sha256 IS NOT NULL
           AND (last_verified IS NULL OR last_verified < NOW() - INTERVAL '7 days')
         ORDER BY random()
         LIMIT GREATEST(1, (SELECT COUNT(*) * 0.05 FROM recordings WHERE sha256 IS NOT NULL))`)
    if err != nil {
        iv.logger.Error("Integrity verifier: failed to query recordings", "error", err)
        return
    }

    for _, rec := range recordings {
        actual, err := computeSHA256(rec.FilePath)
        if err != nil {
            iv.logger.Warn("Integrity verifier: failed to compute checksum", "id", rec.ID, "error", err)
            continue
        }
        if actual != rec.SHA256 {
            iv.logger.Error("Recording integrity mismatch",
                "id", rec.ID, "file_path", rec.FilePath,
                "expected", rec.SHA256, "actual", actual)
            common.RecordingIntegrityMismatch.WithLabelValues(rec.ID).Inc()
        }
        iv.db.ExecContext(ctx, "UPDATE recordings SET last_verified = NOW() WHERE id = $1", rec.ID)
    }
}
```

- [ ] **Step 4: Add SHA256 computation to processRecordingEvent**

In `processRecordingEvent` (`services/recorder/main.go`), after `fixupMoovAtom(event.Path, r.logger)` (line 391) and before the filename parsing (line 393), add:

```go
sha256Str, shaErr := computeSHA256(event.Path)
if shaErr != nil {
    r.logger.Warn("Failed to compute recording checksum", "path", event.Path, "error", shaErr)
}
```

Then change the `RecordingSegment` return to include SHA256. Modify the `RecordingSegment` struct to add a `SHA256` field. Find its definition and add:

```go
type RecordingSegment struct {
    CameraID  string
    StartTime time.Time
    EndTime   time.Time
    FilePath  string
    FileSize  int64
    SHA256    string  // add this
}
```

And in the return statement of `processRecordingEvent`:

```go
return RecordingSegment{
    CameraID:  event.CameraID,
    StartTime: startTime,
    EndTime:   endTime,
    FilePath:  event.Path,
    FileSize:  info.Size(),
    SHA256:    sha256Str,
}, nil
```

- [ ] **Step 5: Update IndexSegment to store SHA256**

Modify `IndexSegment` to store SHA256:

```go
func (r *Recorder) IndexSegment(ctx context.Context, seg RecordingSegment) error {
    start := time.Now()
    query := `INSERT INTO recordings (camera_id, start_time, end_time, file_path, file_size, sha256)
              VALUES (:camera_id, :start_time, :end_time, :file_path, :file_size, :sha256)`
    _, err := r.db.NamedExecContext(ctx, query, seg)
    // ... rest unchanged
}
```

- [ ] **Step 6: Start GapDetector and IntegrityVerifier in main**

In the `Listen` function or `main` setup, after the retention worker is started:

```go
gd := NewGapDetector(r.db, r.logger)
go gd.Run(ctx)

iv := NewIntegrityVerifier(r.db, r.logger)
go iv.Run(ctx)
```

- [ ] **Step 7: Add Prometheus metrics**

In `pkg/common/metrics.go` or wherever metrics are defined, add:

```go
var (
    RecordingGaps = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "recording_gaps_total",
        Help: "Total number of recording gaps detected",
    }, []string{"camera_id"})

    RecordingIntegrityMismatch = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "recording_integrity_mismatch_total",
        Help: "Total number of recording integrity mismatches detected",
    }, []string{"recording_id"})
)
```

If metrics are defined per-service, add them in `services/recorder/main.go`'s `init()` or var block.

- [ ] **Step 8: Verify build**

Run: `cd /home/ubuntu/EVMS && go build ./services/recorder/...`
Expected: Build succeeds

- [ ] **Step 9: Commit**

```bash
git add migrations/043_recording_integrity.up.sql migrations/043_recording_integrity.down.sql \
      services/recorder/integrity.go services/recorder/main.go pkg/common/metrics.go
git commit -m "feat(recorder): add recording integrity checks - SHA256 at ingest, gap detection, periodic verification"
```

---

### Task 6: Async Export Queue

**Files:**
- Create: `services/export/queue.go`
- Create: `migrations/044_export_jobs.up.sql`
- Create: `migrations/044_export_jobs.down.sql`
- Modify: `services/export/main.go`

- [ ] **Step 1: Create migration 044 up**

Write `migrations/044_export_jobs.up.sql`:

```sql
CREATE TABLE export_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  camera_id UUID NOT NULL REFERENCES cameras(id),
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ NOT NULL,
  watermark BOOLEAN NOT NULL DEFAULT false,
  status TEXT NOT NULL DEFAULT 'queued',
  file_path TEXT,
  sha256 TEXT,
  size_bytes BIGINT,
  error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_export_jobs_camera ON export_jobs(camera_id);
CREATE INDEX idx_export_jobs_status ON export_jobs(status);
```

- [ ] **Step 2: Create migration 044 down**

Write `migrations/044_export_jobs.down.sql`:

```sql
DROP TABLE IF EXISTS export_jobs;
```

- [ ] **Step 3: Create `services/export/queue.go`**

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
    "os"
    "os/exec"
    "path/filepath"
    "time"

    "github.com/dam-vms/dam/pkg/common"
    "github.com/google/uuid"
    "github.com/jmoiron/sqlx"
    "github.com/nats-io/nats.go"
)

type ExportJob struct {
    ID          string     `db:"id" json:"id"`
    CameraID    string     `db:"camera_id" json:"camera_id"`
    StartTime   time.Time  `db:"start_time" json:"start_time"`
    EndTime     time.Time  `db:"end_time" json:"end_time"`
    Watermark   bool       `db:"watermark" json:"watermark"`
    Status      string     `db:"status" json:"status"`
    FilePath    *string    `db:"file_path" json:"file_path,omitempty"`
    SHA256      *string    `db:"sha256" json:"sha256,omitempty"`
    SizeBytes   *int64     `db:"size_bytes" json:"size_bytes,omitempty"`
    Error       *string    `db:"error" json:"error,omitempty"`
    CreatedAt   time.Time  `db:"created_at" json:"created_at"`
    CompletedAt *time.Time `db:"completed_at" json:"completed_at,omitempty"`
}

type ExportJobProducer struct {
    db     *sqlx.DB
    nc     *nats.Conn
    logger *slog.Logger
}

type ExportJobConsumer struct {
    db     *sqlx.DB
    logger *slog.Logger
}

func NewExportJobProducer(db *sqlx.DB, nc *nats.Conn, logger *slog.Logger) *ExportJobProducer {
    return &ExportJobProducer{db: db, nc: nc, logger: logger}
}

func NewExportJobConsumer(db *sqlx.DB, logger *slog.Logger) *ExportJobConsumer {
    return &ExportJobConsumer{db: db, logger: logger}
}

func (p *ExportJobProducer) CreateJob(ctx context.Context, req ExportRequest) (*ExportJob, error) {
    id := uuid.New().String()
    now := time.Now()
    job := &ExportJob{
        ID:        id,
        CameraID:  req.CameraID,
        StartTime: req.StartTime,
        EndTime:   req.EndTime,
        Watermark: req.Watermark,
        Status:    "queued",
        CreatedAt: now,
    }

    _, err := p.db.ExecContext(ctx,
        `INSERT INTO export_jobs (id, camera_id, start_time, end_time, watermark, status, created_at)
         VALUES ($1, $2, $3, $4, $5, 'queued', $6)`,
        id, req.CameraID, req.StartTime, req.EndTime, req.Watermark, now)
    if err != nil {
        return nil, fmt.Errorf("failed to create export job: %w", err)
    }

    data, _ := json.Marshal(job)
    if err := p.nc.Publish("export.jobs", data); err != nil {
        p.logger.Error("Failed to publish export job", "job_id", id, "error", err)
    }

    return job, nil
}

func (c *ExportJobConsumer) ProcessJob(ctx context.Context, job *ExportJob) {
    c.logger.Info("Processing export job", "job_id", job.ID, "camera_id", job.CameraID)

    // Mark as processing
    c.db.ExecContext(ctx, "UPDATE export_jobs SET status = 'processing', updated_at = NOW() WHERE id = $1", job.ID)

    // Find segments
    segments, err := findSegments(job.CameraID, job.StartTime, job.EndTime)
    if err != nil {
        c.failJob(ctx, job.ID, fmt.Sprintf("failed to find segments: %v", err))
        return
    }
    if len(segments) == 0 {
        c.failJob(ctx, job.ID, "no recordings found")
        return
    }

    cameraID := sanitizeCameraID(job.CameraID)
    outputPath := filepath.Join("/exports", fmt.Sprintf("export_%s_%s.mp4", cameraID, job.ID[:8]))

    // Build ffmpeg args
    args := []string{"-y"}
    for _, seg := range segments {
        if err := common.ValidateRecordingPath(seg); err != nil {
            c.failJob(ctx, job.ID, fmt.Sprintf("invalid segment path: %s", seg))
            return
        }
        if err := common.ValidateFilePath(seg, "/recordings"); err != nil {
            c.failJob(ctx, job.ID, fmt.Sprintf("segment path outside allowed root: %s", seg))
            return
        }
        args = append(args, "-i", seg)
    }
    filter := fmt.Sprintf("concat=%d", len(segments))
    if job.Watermark {
        content := fmt.Sprintf("%%{localtime} | Camera: %s", cameraID)
        tmpFile, err := os.CreateTemp("", "watermark_*.txt")
        if err != nil {
            c.failJob(ctx, job.ID, "failed to create watermark file")
            return
        }
        watermarkPath := tmpFile.Name()
        if _, err := tmpFile.WriteString(content); err != nil {
            tmpFile.Close()
            os.Remove(watermarkPath)
            c.failJob(ctx, job.ID, "failed to write watermark file")
            return
        }
        tmpFile.Close()
        defer os.Remove(watermarkPath)
        filter += fmt.Sprintf(",drawtext=textfile='%s':fontsize=24:fontcolor=white:x=10:y=10", watermarkPath)
    }
    args = append(args, "-filter_complex", filter, "-c:v", "libx264", "-preset", "fast", outputPath)

    cmd := exec.CommandContext(ctx, "ffmpeg", args...)
    var stderr bytes.Buffer
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        c.failJob(ctx, job.ID, fmt.Sprintf("ffmpeg failed: %v - %s", err, stderr.String()))
        return
    }

    // Compute checksum
    f, err := os.Open(outputPath)
    if err != nil {
        c.failJob(ctx, job.ID, fmt.Sprintf("failed to read export: %v", err))
        return
    }
    defer f.Close()
    h := sha256.New()
    size, _ := io.Copy(h, f)
    checksum := fmt.Sprintf("%x", h.Sum(nil))
    sizeBytes := size

    now := time.Now()
    _, err = c.db.ExecContext(ctx,
        `UPDATE export_jobs SET status = 'completed', file_path = $1, sha256 = $2, size_bytes = $3, completed_at = $4, updated_at = $4 WHERE id = $5`,
        outputPath, checksum, sizeBytes, now, job.ID)
    if err != nil {
        c.logger.Error("Failed to update export job as completed", "job_id", job.ID, "error", err)
    }
    c.logger.Info("Export job completed", "job_id", job.ID, "file", outputPath, "sha256", checksum)
}

func (c *ExportJobConsumer) failJob(ctx context.Context, jobID, errMsg string) {
    c.logger.Error("Export job failed", "job_id", jobID, "error", errMsg)
    now := time.Now()
    c.db.ExecContext(ctx,
        `UPDATE export_jobs SET status = 'failed', error = $1, completed_at = $2, updated_at = $2 WHERE id = $3`,
        errMsg, now, jobID)
}
```

- [ ] **Step 4: Modify export HTTP handler to use queue**

Replace `handleExport` with:

```go
var (
    exportProducer *ExportJobProducer
)

func handleExport(w http.ResponseWriter, r *http.Request) {
    var req ExportRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        jsonError(w, "invalid request", http.StatusBadRequest)
        return
    }

    if exportProducer == nil {
        jsonError(w, "export queue not available", http.StatusServiceUnavailable)
        return
    }

    ctx := r.Context()
    job, err := exportProducer.CreateJob(ctx, req)
    if err != nil {
        slog.Error("Failed to create export job", "error", err)
        jsonError(w, "failed to create export job", http.StatusInternalServerError)
        return
    }

    writeJSON(w, http.StatusAccepted, job)
}

type jobStatusResponse struct {
    ID          string     `json:"id"`
    Status      string     `json:"status"`
    FilePath    *string    `json:"file_path,omitempty"`
    SHA256      *string    `json:"sha256,omitempty"`
    SizeBytes   *int64     `json:"size_bytes,omitempty"`
    Error       *string    `json:"error,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func handleExportStatus(db *sqlx.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        jobID := r.PathValue("id")
        if jobID == "" {
            jsonError(w, "job id required", http.StatusBadRequest)
            return
        }

        var job jobStatusResponse
        err := db.GetContext(r.Context(), &job,
            "SELECT id, status, file_path, sha256, size_bytes, error, created_at, completed_at FROM export_jobs WHERE id = $1",
            jobID)
        if err != nil {
            jsonError(w, "job not found", http.StatusNotFound)
            return
        }

        writeJSON(w, http.StatusOK, job)
    }
}

func handleExportDownload(db *sqlx.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        jobID := r.PathValue("id")
        if jobID == "" {
            jsonError(w, "job id required", http.StatusBadRequest)
            return
        }

        var job struct {
            Status   string  `db:"status"`
            FilePath *string `db:"file_path"`
        }
        err := db.GetContext(r.Context(), &job,
            "SELECT status, file_path FROM export_jobs WHERE id = $1", jobID)
        if err != nil {
            jsonError(w, "job not found", http.StatusNotFound)
            return
        }

        if job.Status != "completed" {
            writeJSON(w, http.StatusConflict, map[string]string{"error": "export not yet completed", "status": job.Status})
            return
        }

        if job.FilePath == nil {
            jsonError(w, "file path not available", http.StatusInternalServerError)
            return
        }

        http.ServeFile(w, r, *job.FilePath)
    }
}
```

- [ ] **Step 5: Update main.go for NATS, queue producer, and new routes**

Replace `main()` with:

```go
func main() {
    logger := common.NewLogger("export")
    slog.SetDefault(logger)

    common.CheckJWTSecret()

    if err := common.InitTelemetry("export"); err != nil {
        logger.Error("Failed to initialize telemetry", "error", err)
    }
    defer common.ShutdownTelemetry()

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))
    common.StartResourceMonitor(ctx)

    dbURL := os.Getenv("DB_URL")
    var db *sqlx.DB
    if dbURL != "" {
        cb := common.NewDBCircuitBreaker("export")
        dbCtx, dbCancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer dbCancel()
        var err error
        db, err = common.ConnectDBWithCircuitBreaker(dbCtx, "postgres", dbURL, cb)
        if err != nil {
            logger.Error("Failed to connect to database", "error", err)
        } else {
            logger.Info("Connected to database")
        }
    }

    // Connect to NATS
    natsURL := os.Getenv("NATS_URL")
    var nc *nats.Conn
    if natsURL != "" && db != nil {
        natsCB := common.NewNATSCircuitBreaker("export")
        var err error
        nc, err = common.ConnectNATSWithCircuitBreaker(natsURL, natsCB)
        if err != nil {
            logger.Error("Failed to connect to NATS", "error", err)
        } else {
            logger.Info("Connected to NATS")
            exportProducer = NewExportJobProducer(db, nc, logger)
            consumer := NewExportJobConsumer(db, logger)
            // Subscribe to export jobs
            nc.Subscribe("export.jobs", func(msg *nats.Msg) {
                var job ExportJob
                if err := json.Unmarshal(msg.Data, &job); err != nil {
                    logger.Error("Failed to unmarshal export job", "error", err)
                    return
                }
                go consumer.ProcessJob(context.Background(), &job)
            })
            logger.Info("Export worker subscribed to export.jobs")
        }
    }

    mux := http.NewServeMux()
    healthHandler := common.NewHealthHandler()
    if db != nil {
        healthHandler.AddDBChecker(db.DB, "postgres")
    }
    if nc != nil {
        healthHandler.AddNATSChecker(nc, "nats")
    }
    mux.HandleFunc("/health", healthHandler.Liveness)
    mux.HandleFunc("/ready", healthHandler.Readiness)

    // Export endpoints (now async)
    mux.Handle("/export", common.JWTAuthMiddleware(handleExport))
    mux.Handle("/export/status/{id}", common.JWTAuthMiddleware(handleExportStatus(db)))
    mux.Handle("/export/download/{id}", common.JWTAuthMiddleware(handleExportDownload(db)))

    if db != nil {
        mux.Handle("/api/evidence/cases", common.JWTAuthMiddleware(handleEvidenceCases(db, logger)))
        mux.Handle("/api/evidence/cases/", common.JWTAuthMiddleware(handleEvidenceCaseByID(db, logger)))
        mux.Handle("/api/evidence/lockers", common.JWTAuthMiddleware(handleEvidenceLockers(db, logger)))
        mux.Handle("/api/evidence/lockers/", common.JWTAuthMiddleware(handleEvidenceLockerByID(db, logger)))
        mux.Handle("/api/evidence/items", common.JWTAuthMiddleware(handleEvidenceItems(db, logger)))
        mux.Handle("/api/evidence/items/", common.JWTAuthMiddleware(handleEvidenceItemByID(db, logger)))
        mux.Handle("/api/evidence/share/", common.JWTAuthMiddleware(handleShareAccess(db, logger)))
    }

    server := &http.Server{
        Addr:         ":8094",
        Handler:      common.RecoveryMiddleware(mux),
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 60 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    go func() {
        logger.Info("Export service listening", "addr", ":8094")
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("server error", "error", err)
        }
    }()

    <-ctx.Done()
    shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    if nc != nil {
        nc.Drain()
    }
    server.Shutdown(shutdownCtx)
}
```

Add import for `"encoding/json"`, `"github.com/nats-io/nats.go"`.

- [ ] **Step 6: Verify build**

Run: `cd /home/ubuntu/EVMS && go build ./services/export/...`
Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
git add migrations/044_export_jobs.up.sql migrations/044_export_jobs.down.sql \
      services/export/queue.go services/export/main.go
git commit -m "feat(export): add async export queue with NATS JetStream"
```

---

## Post-Implementation

After all 6 tasks are complete:

- [ ] **Run all Go builds**: `cd /home/ubuntu/EVMS && go build ./...`
- [ ] **Run all tests**: `go test ./services/camera-mgmt/... ./services/camera-control/... ./services/recorder/... ./services/export/... -count=1`
- [ ] **Update release readiness report**: Update RELEASE_READINESS_REPORT.md Camera Operations from 70% to 90%, Recording from 85% to 92%, overall from 89%+ to 90%+
- [ ] **Commit final report update**: `git add RELEASE_READINESS_REPORT.md && git commit -m "docs: update readiness report after camera ops and recording hardening"`
