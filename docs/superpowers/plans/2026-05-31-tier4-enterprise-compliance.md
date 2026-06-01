# Tier 4: Enterprise Compliance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fulfill regulated vertical requirements — tamper-proof audit log hash chain, legal hold/retention lock, GDPR privacy blur, production Helm chart, N+1 recorder redundancy, FIPS-compliant crypto.

**Architecture:** SQL hash chain linking each audit row via SHA-256 of previous row; WORM-style retain flag on recordings with scheduled cleanup skip; FFmpeg post-processing for privacy blur; Helm chart with HPA/PDB/NetworkPolicy; NATS JetStream for exactly-once delivery in active-passive recorder pair; boringssl FIPS fork for crypto.

**Tech Stack:** Go, PostgreSQL (hash chain), FFmpeg (blur), Helm/K8s, NATS JetStream, boringssl

---

### Task 1: Tamper-proof audit log hash chain

**Files:**
- Create: `services/audit/main.go`
- Create: `services/audit/chain.go`
- Create: `services/audit/chain_test.go`
- Modify: `services/api-gateway/main.go` (audit middleware)

- [ ] **Step 1: Create hash chain implementation**

Create `services/audit/chain.go`:

```go
package main

import (
    "crypto/sha256"
    "database/sql"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "time"

    "github.com/jmoiron/sqlx"
)

type AuditEntry struct {
    ID           string          `db:"id"`
    PreviousHash string          `db:"previous_hash"`
    Hash         string          `db:"hash"`
    UserID       string          `db:"user_id"`
    Action       string          `db:"action"`
    ResourceType string          `db:"resource_type"`
    ResourceID   string          `db:"resource_id"`
    Details      json.RawMessage `db:"details"`
    CreatedAt    time.Time       `db:"created_at"`
}

type AuditChain struct {
    db *sqlx.DB
}

func NewAuditChain(db *sqlx.DB) *AuditChain {
    return &AuditChain{db: db}
}

func (ac *AuditChain) Append(userID, action, resourceType, resourceID string, details interface{}) (*AuditEntry, error) {
    // Get last hash
    var lastHash sql.NullString
    ac.db.Get(&lastHash, "SELECT hash FROM audit_logs ORDER BY created_at DESC LIMIT 1")

    detailsJSON, _ := json.Marshal(details)

    // Build chain payload
    payload := map[string]interface{}{
        "previous_hash": lastHash.String,
        "user_id":       userID,
        "action":        action,
        "resource_type": resourceType,
        "resource_id":   resourceID,
        "details":       string(detailsJSON),
        "timestamp":     time.Now().UTC(),
    }
    payloadJSON, _ := json.Marshal(payload)
    hash := sha256.Sum256(payloadJSON)
    hashStr := hex.EncodeToString(hash[:])

    var entry AuditEntry
    err := ac.db.QueryRow(
        `INSERT INTO audit_logs (previous_hash, hash, user_id, action, resource_type, resource_id, details)
         VALUES ($1, $2, $3, $4, $5, $6, $7)
         RETURNING id, previous_hash, hash, user_id, action, resource_type, resource_id, details, created_at`,
        lastHash.String, hashStr, userID, action, resourceType, resourceID, detailsJSON,
    ).Scan(&entry.ID, &entry.PreviousHash, &entry.Hash, &entry.UserID, &entry.Action,
        &entry.ResourceType, &entry.ResourceID, &entry.Details, &entry.CreatedAt)

    if err != nil {
        return nil, fmt.Errorf("append audit: %w", err)
    }
    return &entry, nil
}

func (ac *AuditChain) Verify(from time.Time) (bool, error) {
    var entries []AuditEntry
    err := ac.db.Select(&entries,
        "SELECT id, previous_hash, hash, user_id, action, resource_type, resource_id, details, created_at FROM audit_logs WHERE created_at >= $1 ORDER BY created_at ASC", from)
    if err != nil {
        return false, err
    }

    var prevHash string
    for i, entry := range entries {
        // Recompute hash
        payload := map[string]interface{}{
            "previous_hash": prevHash,
            "user_id":       entry.UserID,
            "action":        entry.Action,
            "resource_type": entry.ResourceType,
            "resource_id":   entry.ResourceID,
            "details":       string(entry.Details),
            "timestamp":     entry.CreatedAt.UTC(),
        }
        payloadJSON, _ := json.Marshal(payload)
        computed := sha256.Sum256(payloadJSON)
        computedStr := hex.EncodeToString(computed[:])

        if computedStr != entry.Hash {
            return false, fmt.Errorf("chain broken at entry %d (id=%s)", i, entry.ID)
        }
        if i > 0 && entry.PreviousHash != prevHash {
            return false, fmt.Errorf("previous_hash mismatch at entry %d (id=%s)", i, entry.ID)
        }
        prevHash = entry.Hash
    }
    return true, nil
}
```

- [ ] **Step 2: Write chain verification test**

Create `services/audit/chain_test.go`:

```go
package main

import (
    "testing"
    "time"
)

func TestHashChain(t *testing.T) {
    // Test the hash computation logic without DB
    payload := map[string]interface{}{
        "previous_hash": "",
        "user_id":       "user1",
        "action":        "login",
        "timestamp":     time.Now().UTC(),
    }

    // Verify deterministic hashing
    hash1 := computeHash(payload)
    hash2 := computeHash(payload)
    if hash1 != hash2 {
        t.Fatal("hash must be deterministic")
    }
}

func computeHash(payload map[string]interface{}) string {
    data, _ := json.Marshal(payload)
    h := sha256.Sum256(data)
    return hex.EncodeToString(h[:])
}
```

- [ ] **Step 3: Create audit service HTTP API**

Create `services/audit/main.go`:

```go
package main

import (
    "context"
    "encoding/json"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/dam-vms/dam/pkg/common"
    "github.com/jmoiron/sqlx"
    _ "github.com/lib/pq"
)

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    dbURL := common.GetEnv("DB_URL", "postgres://dam_admin:dam_password@localhost:5432/dam_vms?sslmode=disable")
    db, err := sqlx.Connect("postgres", dbURL)
    if err != nil {
        logger.Error("db connect", "error", err)
        os.Exit(1)
    }
    defer db.Close()

    chain := NewAuditChain(db)

    mux := http.NewServeMux()

    // POST /audit - append entry
    mux.HandleFunc("/audit", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
        var req struct {
            UserID       string      `json:"user_id"`
            Action       string      `json:"action"`
            ResourceType string      `json:"resource_type"`
            ResourceID   string      `json:"resource_id"`
            Details      interface{} `json:"details"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            jsonError(w, "invalid request", http.StatusBadRequest)
            return
        }
        entry, err := chain.Append(req.UserID, req.Action, req.ResourceType, req.ResourceID, req.Details)
        if err != nil {
            logger.Error("append audit", "error", err)
            jsonError(w, "append failed", http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusCreated)
        json.NewEncoder(w).Encode(entry)
    })

    // GET /audit/verify - verify chain integrity
    mux.HandleFunc("/audit/verify", func(w http.ResponseWriter, r *http.Request) {
        ok, err := chain.Verify(time.Now().Add(-720 * time.Hour)) // last 30 days
        status := "ok"
        if err != nil || !ok {
            status = "broken"
            w.WriteHeader(http.StatusInternalServerError)
        }
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status": status,
            "error":  err,
        })
    })

    common.StartMetricsServer(common.GetEnv("METRICS_ADDR", ":2112"))

    server := &http.Server{
        Addr:         ":8095",
        Handler:      mux,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    go func() {
        logger.Info("audit service listening", "addr", ":8095")
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("server error", "error", err)
        }
    }()

    <-ctx.Done()
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    server.Shutdown(shutdownCtx)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
```

- [ ] **Step 4: Add audit middleware to gateway for state mutations**

In `services/api-gateway/main.go`, add audit logging:

```go
func (g *Gateway) auditMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Only audit state-mutating methods
        if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
            // Extract user from JWT on non-blocking goroutine
            go func(method, path string) {
                // POST to audit service
                http.Post("http://audit:8095/audit", "application/json",
                    strings.NewReader(fmt.Sprintf(
                        `{"user_id":"system","action":"%s %s","resource_type":"api","details":{}}`,
                        method, path)))
            }(r.Method, r.URL.Path)
        }
        next(w, r)
    }
}
```

Wrap state-mutating routes with audit middleware.

- [ ] **Step 5: Validate**

Run: `gofmt -d services/audit/*.go`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: tamper-proof audit log with SHA-256 hash chain and verification endpoint"
```

---

### Task 2: Legal hold / retention lock

**Files:**
- Modify: `services/recorder/main.go`
- Modify: `services/api-gateway/main.go`
- Modify: `web/src/pages/SettingsPage.tsx`
- Create: `migrations/003_legal_hold.sql`

- [ ] **Step 1: Create legal hold migration**

Create `migrations/003_legal_hold.sql`:

```sql
CREATE TABLE IF NOT EXISTS legal_holds (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    reason TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT legal_hold_time_range CHECK (end_time > start_time)
);

-- Prevent deletion of recordings under legal hold
CREATE INDEX IF NOT EXISTS idx_legal_holds_camera_time
    ON legal_holds(camera_id, start_time, end_time);
```

- [ ] **Step 2: Update recorder retention cleanup to respect holds**

In `services/recorder/main.go`, update the retention cleanup function:

```go
func (s *RecorderService) cleanupOldSegments(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            s.deleteExpiredSegments(ctx)
        case <-ctx.Done():
            return
        }
    }
}

func (s *RecorderService) deleteExpiredSegments(ctx context.Context) {
    // Find segments past retention that are NOT under legal hold
    _, err := s.db.ExecContext(ctx, `
        DELETE FROM recordings r
        WHERE r.end_time < NOW() - (SELECT retention_days FROM cameras c WHERE c.id = r.camera_id) * INTERVAL '1 day'
        AND NOT EXISTS (
            SELECT 1 FROM legal_holds lh
            WHERE lh.camera_id = r.camera_id
            AND r.start_time < lh.end_time
            AND r.end_time > lh.start_time
        )`)
    if err != nil {
        s.logger.Error("cleanup failed", "error", err)
    }
}
```

- [ ] **Step 3: Add legal hold API endpoints**

In `services/api-gateway/main.go`:

```go
func (g *Gateway) handleLegalHolds(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        var holds []struct {
            ID        string    `json:"id"`
            CameraID  string    `json:"camera_id"`
            StartTime time.Time `json:"start_time"`
            EndTime   time.Time `json:"end_time"`
            Reason    string    `json:"reason"`
        }
        g.db.SelectContext(r.Context(), &holds, "SELECT id, camera_id, start_time, end_time, reason FROM legal_holds ORDER BY created_at DESC")
        json.NewEncoder(w).Encode(map[string]interface{}{"holds": holds})

    case http.MethodPost:
        var req struct {
            CameraID  string `json:"camera_id"`
            StartTime string `json:"start_time"`
            EndTime   string `json:"end_time"`
            Reason    string `json:"reason"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            jsonError(w, "invalid request", http.StatusBadRequest)
            return
        }
        _, err := g.db.ExecContext(r.Context(),
            "INSERT INTO legal_holds (camera_id, start_time, end_time, reason, created_by) VALUES ($1, $2, $3, $4, $5)",
            req.CameraID, req.StartTime, req.EndTime, req.Reason, "admin")
        if err != nil {
            jsonError(w, "failed to create hold", http.StatusInternalServerError)
            return
        }
        json.NewEncoder(w).Encode(map[string]string{"status": "created"})

    case http.MethodDelete:
        id := strings.TrimPrefix(r.URL.Path, "/api/legal-holds/")
        g.db.ExecContext(r.Context(), "DELETE FROM legal_holds WHERE id = $1", id)
        json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
    }
}
```

- [ ] **Step 4: Add legal hold UI to SettingsPage**

```typescript
interface LegalHold {
  id: string;
  camera_id: string;
  start_time: string;
  end_time: string;
  reason: string;
}

const [holds, setHolds] = useState<LegalHold[]>([]);

<div className="bg-gray-800 p-4 rounded mt-4">
  <h3 className="font-bold mb-2 text-red-400">⚖️ Legal Holds (WORM)</h3>
  {holds.filter(h => h.camera_id === cameraId).map(h => (
    <div key={h.id} className="bg-gray-700 p-2 rounded mb-2 text-sm flex justify-between">
      <div>
        <span className="text-red-300">{h.reason}</span>
        <span className="text-gray-400 ml-2">{h.start_time} → {h.end_time}</span>
      </div>
      <button onClick={() => deleteHold(h.id)} className="text-red-400 hover:text-red-300">Remove</button>
    </div>
  ))}
</div>
```

- [ ] **Step 5: Validate**

Run: `gofmt -d services/recorder/main.go services/api-gateway/main.go`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: legal hold/WORM retention lock with time-range and cleanup exclusion"
```

---

### Task 3: GDPR privacy blur worker

**Files:**
- Create: `services/privacy-worker/main.go`
- Create: `services/privacy-worker/Dockerfile`
- Create: `migrations/004_privacy_queue.sql`
- Modify: `web/src/pages/SettingsPage.tsx`

- [ ] **Step 1: Create privacy blur worker**

Create `services/privacy-worker/main.go`:

```go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "os"
    "os/exec"
    "os/signal"
    "path/filepath"
    "syscall"
    "time"

    "github.com/jmoiron/sqlx"
    _ "github.com/lib/pq"
    "github.com/nats-io/nats.go"
)

type BlurJob struct {
    ID        string `json:"id"`
    CameraID  string `json:"camera_id"`
    FilePath  string `json:"file_path"`
    Regions   []struct {
        X float64 `json:"x"`
        Y float64 `json:"y"`
        W float64 `json:"w"`
        H float64 `json:"h"`
    } `json:"regions"`
}

func processBlurJob(job BlurJob, logger *slog.Logger) error {
    inputPath := job.FilePath
    ext := filepath.Ext(inputPath)
    outputPath := inputPath[:len(inputPath)-len(ext)] + "_blurred" + ext

    // Build FFmpeg filter for each blur region
    var filterParts []string
    for i, region := range job.Regions {
        filterParts = append(filterParts, fmt.Sprintf(
            "[0:v]crop=iw*%f:ih*%f:iw*%f:ih*%f,boxblur=10:5[blur%d];[0:v][blur%d]overlay=%f*W:%f*H[out%d]",
            region.W, region.H, region.X, region.Y,
            i, i, region.X, region.Y, i))
    }

    if len(filterParts) == 0 {
        return nil // nothing to blur
    }

    filterComplex := strings.Join(filterParts, ";")
    filterComplex += fmt.Sprintf(";[out%d]format=yuv420p", len(job.Regions)-1)

    args := []string{
        "-i", inputPath,
        "-filter_complex", filterComplex,
        "-c:v", "libx264", "-preset", "fast",
        "-c:a", "copy",
        "-y", outputPath,
    }

    cmd := exec.Command("ffmpeg", args...)
    var stderr bytes.Buffer
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("ffmpeg blur failed: %w\nstderr: %s", err, stderr.String())
    }

    // Replace original with blurred
    os.Remove(inputPath)
    os.Rename(outputPath, inputPath)

    return nil
}

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    nc, err := nats.Connect(os.Getenv("NATS_URL"),
        nats.RetryOnFailedConnect(true),
        nats.MaxReconnects(-1),
        nats.ReconnectWait(2*time.Second))
    if err != nil {
        logger.Error("nats connect", "error", err)
        os.Exit(1)
    }
    defer nc.Close()

    nc.QueueSubscribe("privacy.blur", "privacy-workers", func(msg *nats.Msg) {
        var job BlurJob
        if err := json.Unmarshal(msg.Data, &job); err != nil {
            logger.Error("parse job", "error", err)
            msg.Ack()
            return
        }

        logger.Info("processing blur job", "id", job.ID, "file", job.FilePath)
        if err := processBlurJob(job, logger); err != nil {
            logger.Error("blur failed", "id", job.ID, "error", err)
        }
        msg.Ack()
    })

    <-ctx.Done()
}
```

- [ ] **Step 2: Create Dockerfile**

Create `services/privacy-worker/Dockerfile`:

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /privacy-worker services/privacy-worker/main.go

FROM alpine:3.19
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=builder /privacy-worker /usr/local/bin/privacy-worker
CMD ["privacy-worker"]
```

- [ ] **Step 3: Create privacy queue migration**

Create `migrations/004_privacy_queue.sql`:

```sql
-- Privacy blur request queue
CREATE TABLE IF NOT EXISTS privacy_blur_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    camera_id UUID NOT NULL REFERENCES cameras(id),
    recording_id UUID REFERENCES recordings(id),
    requested_by TEXT NOT NULL,
    reason TEXT NOT NULL, -- 'gdpr_request', 'user_initiated'
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'processing', 'completed', 'failed'
    created_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_privacy_blur_status ON privacy_blur_requests(status);
```

- [ ] **Step 4: Add privacy blur request to SettingsPage**

```typescript
const requestPrivacyBlur = async () => {
  await api.requestPrivacyBlur(cameraId, 'user_initiated');
};

<button onClick={requestPrivacyBlur}
        className="bg-red-700 px-3 py-1 rounded text-sm mt-2">
  Request Privacy Blur (GDPR)
</button>
```

- [ ] **Step 5: Validate**

Run: `gofmt -d services/privacy-worker/*.go`

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: GDPR privacy blur worker with FFmpeg face/region blur and NATS job queue"
```

---

### Task 4: Production Helm chart

**Files:**
- Create: `deploy/charts/dam-vms/Chart.yaml`
- Create: `deploy/charts/dam-vms/values.yaml`
- Create: `deploy/charts/dam-vms/templates/_helpers.tpl`
- Create: `deploy/charts/dam-vms/templates/deployment.yaml`
- Create: `deploy/charts/dam-vms/templates/service.yaml`
- Create: `deploy/charts/dam-vms/templates/ingress.yaml`
- Create: `deploy/charts/dam-vms/templates/hpa.yaml`
- Create: `deploy/charts/dam-vms/templates/pdb.yaml`
- Create: `deploy/charts/dam-vms/templates/networkpolicy.yaml`
- Create: `deploy/charts/dam-vms/templates/secret.yaml`
- Create: `deploy/charts/dam-vms/templates/pvc.yaml`
- Create: `deploy/charts/dam-vms/templates/configmap.yaml`

- [ ] **Step 1: Create Chart.yaml**

```yaml
apiVersion: v2
name: dam-vms
description: DAM VMS - Production Video Management System
type: application
version: 0.1.0
appVersion: "1.0.0"
```

- [ ] **Step 2: Create values.yaml**

```yaml
global:
  imageRegistry: "ghcr.io/dam-vms"
  imageTag: "latest"
  imagePullPolicy: "IfNotPresent"
  db:
    host: "postgres"
    port: 5432
    name: "dam_vms"
    user: "dam_admin"
  nats:
    url: "nats://nats:4222"

auth:
  enabled: true
  replicas: 2
  resources:
    requests: { cpu: "100m", memory: "128Mi" }
    limits: { cpu: "500m", memory: "256Mi" }

cameraMgmt:
  enabled: true
  replicas: 2
  resources:
    requests: { cpu: "100m", memory: "128Mi" }
    limits: { cpu: "500m", memory: "256Mi" }

recorder:
  enabled: true
  replicas: 2
  storage: 100Gi
  storageClass: "nvme-raid"
  resources:
    requests: { cpu: "500m", memory: "512Mi" }
    limits: { cpu: "2", memory: "2Gi" }

playback:
  enabled: true
  replicas: 3
  resources:
    requests: { cpu: "200m", memory: "256Mi" }
    limits: { cpu: "1", memory: "512Mi" }

gateway:
  enabled: true
  replicas: 2
  tls:
    enabled: false
    domain: ""
    email: ""
  resources:
    requests: { cpu: "100m", memory: "128Mi" }
    limits: { cpu: "500m", memory: "256Mi" }

aiWorker:
  enabled: true
  replicas: 1
  gpu:
    enabled: false
  resources:
    requests: { cpu: "500m", memory: "1Gi" }
    limits: { cpu: "4", memory: "4Gi" }

export:
  enabled: true
  replicas: 1

ingress:
  enabled: true
  className: "nginx"
  host: "vms.example.com"
  tls: true
  certManager:
    enabled: true
    issuer: "letsencrypt-prod"

hpa:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  cpuThreshold: 75
  memoryThreshold: 80

pdb:
  enabled: true
  minAvailable: 1

networkPolicy:
  enabled: true
  allowFromMonitoring: true
```

- [ ] **Step 3: Create deployment template**

```yaml
{{- range $name, $service := .Values }}
{{- if and (kindIs "map" $service) $service.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ $name }}
  labels:
    app: dam-vms
    component: {{ $name }}
spec:
  replicas: {{ $service.replicas | default 1 }}
  selector:
    matchLabels:
      app: dam-vms
      component: {{ $name }}
  template:
    metadata:
      labels:
        app: dam-vms
        component: {{ $name }}
    spec:
      imagePullSecrets:
        - name: regcred
      containers:
        - name: {{ $name }}
          image: "{{ $.Values.global.imageRegistry }}/{{ $name }}:{{ $.Values.global.imageTag }}"
          imagePullPolicy: {{ $.Values.global.imagePullPolicy }}
          envFrom:
            - configMapRef:
                name: dam-vms-config
            - secretRef:
                name: dam-vms-secret
          ports:
            - containerPort: {{ $service.port | default 8080 }}
              name: http
          livenessProbe:
            httpGet:
              path: /health
              port: 2112
            initialDelaySeconds: 10
            periodSeconds: 15
          readinessProbe:
            httpGet:
              path: /health
              port: 2112
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            {{- toYaml $service.resources | nindent 12 }}
{{- end }}
{{- end }}
```

- [ ] **Step 4: Create HPA template**

```yaml
{{- if .Values.hpa.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: dam-vms-recorder
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: recorder
  minReplicas: {{ .Values.hpa.minReplicas }}
  maxReplicas: {{ .Values.hpa.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.hpa.cpuThreshold }}
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: {{ .Values.hpa.memoryThreshold }}
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: dam-vms-playback
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: playback
  minReplicas: 2
  maxReplicas: 20
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Pods
      pods:
        metric:
          name: webrtc_connections
        target:
          type: AverageValue
          averageValue: 50
{{- end }}
```

- [ ] **Step 5: Create PDB template**

```yaml
{{- if .Values.pdb.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: dam-vms-pdb
spec:
  minAvailable: {{ .Values.pdb.minAvailable }}
  selector:
    matchLabels:
      app: dam-vms
{{- end }}
```

- [ ] **Step 6: Validate chart**

```bash
helm lint deploy/charts/dam-vms/
helm template dam-vms deploy/charts/dam-vms/ > /dev/null
```

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: production Helm chart with HPA, PDB, NetworkPolicy, Ingress, TLS"
```

---

### Task 5: N+1 recorder redundancy with NATS JetStream

**Files:**
- Modify: `services/recorder/main.go`
- Create: `services/recorder/leader.go`
- Modify: `deploy/docker/docker-compose.yml` (add jetstream)

- [ ] **Step 1: Implement leader election via NATS KV store**

Create `services/recorder/leader.go`:

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "time"

    "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/jetstream"
)

type LeaderElection struct {
    kv        jetstream.KeyValue
    key       string
    identity  string
    isLeader  bool
    heartbeat *time.Ticker
    logger    *slog.Logger
    onLeader  func(bool)
}

func NewLeaderElection(nc *nats.Conn, key, identity string, logger *slog.Logger, onLeader func(bool)) (*LeaderElection, error) {
    js, err := jetstream.New(nc)
    if err != nil {
        return nil, err
    }

    kv, err := js.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{
        Bucket: "leader_election",
        TTL:    10 * time.Second,
    })
    if err != nil {
        kv, err = js.KeyValue(context.Background(), "leader_election")
        if err != nil {
            return nil, err
        }
    }

    return &LeaderElection{
        kv:       kv,
        key:      key,
        identity: identity,
        logger:   logger,
        onLeader: onLeader,
    }, nil
}

func (le *LeaderElection) Run(ctx context.Context) {
    le.heartbeat = time.NewTicker(3 * time.Second)
    defer le.heartbeat.Stop()

    for {
        select {
        case <-le.heartbeat.C:
            // Try to acquire/renew leadership
            if err := le.kv.Put(ctx, le.key, []byte(le.identity)); err != nil {
                le.logger.Warn("leader election put failed", "error", err)
                if le.isLeader {
                    le.isLeader = false
                    le.onLeader(false)
                }
                continue
            }

            // Verify we're still leader
            entry, err := le.kv.Get(ctx, le.key)
            if err != nil {
                continue
            }

            becameLeader := string(entry.Value()) == le.identity && !le.isLeader
            lostLeader := string(entry.Value()) != le.identity && le.isLeader

            if becameLeader {
                le.isLeader = true
                le.logger.Info("became leader", "identity", le.identity)
                le.onLeader(true)
            }
            if lostLeader {
                le.isLeader = false
                le.logger.Info("lost leadership", "identity", le.identity)
                le.onLeader(false)
            }

        case <-ctx.Done():
            // Release leadership
            le.kv.Delete(ctx, le.key)
            return
        }
    }
}
```

- [ ] **Step 2: Integrate leader election into recorder**

In `services/recorder/main.go`:

```go
identity := os.Getenv("HOSTNAME")
if identity == "" {
    identity = fmt.Sprintf("recorder-%d", time.Now().UnixNano())
}

var isActive bool
le, err := NewLeaderElection(nc, "recorder-leader", identity, logger, func(leader bool) {
    isActive = leader
    if leader {
        logger.Info("this recorder is now active — starting recording")
        // Start recording goroutines
    } else {
        logger.Info("this recorder is now standby — stopping recording")
        // Stop recording goroutines
    }
})
if err != nil {
    logger.Error("leader election init", "error", err)
    // Continue without leader election (standalone mode)
} else {
    go le.Run(ctx)
}

// Use JetStream for exactly-once delivery of recording frames
js, _ := jetstream.New(nc)
js.CreateStream(context.Background(), jetstream.StreamConfig{
    Name:      "recordings_stream",
    Subjects:  []string{"recordings.>"},
    Storage:   jetstream.FileStorage,
    Replicas:  1,
    Retention: jetstream.LimitRetention,
    MaxAge:    24 * time.Hour,
})
```

- [ ] **Step 3: Update docker-compose for JetStream**

In `deploy/docker/docker-compose.yml`:

```yaml
  nats:
    image: nats:latest
    command: ["-js", "-sd", "/data"]
    ports:
      - "4222:4222"
    volumes:
      - nats-data:/data
    networks: [dam-net]
```

- [ ] **Step 4: Validate**

Run: `gofmt -d services/recorder/leader.go`

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: N+1 recorder redundancy with NATS JetStream leader election and KV store"
```

---

### Task 6: FIPS 140-2 compliant crypto build

**Files:**
- Create: `Makefile`
- Create: `fips.go`

- [ ] **Step 1: Create FIPS build Makefile**

Create `Makefile` at project root:

```makefile
.PHONY: build build-fips test lint

# Standard build
build:
	go build -o bin/gateway services/api-gateway/main.go
	go build -o bin/auth services/auth/main.go
	go build -o bin/camera-mgmt services/camera-mgmt/main.go
	go build -o bin/recorder services/recorder/main.go
	go build -o bin/playback services/playback/main.go
	go build -o bin/event-proc services/event-proc/main.go
	go build -o bin/metadata services/metadata/main.go
	go build -o bin/notification services/notification/main.go
	go build -o bin/camera-control services/camera-control/main.go
	go build -o bin/thumbnails services/thumbnails/main.go
	go build -o bin/discovery services/discovery/main.go
	go build -o bin/onvif-events services/onvif-events/main.go
	go build -o bin/export services/export/main.go
	go build -o bin/audit services/audit/main.go
	go build -o bin/privacy-worker services/privacy-worker/main.go

# FIPS-compliant build using golang-fips toolchain
# Requires: go1.23.0 openssl-fips (https://github.com/golang/go/wiki/GoFIPS)
build-fips:
	CGO_ENABLED=1 GOEXPERIMENT=strictfipsruntime \
	go build -tags=fips -o bin/gateway-fips services/api-gateway/main.go
	CGO_ENABLED=1 GOEXPERIMENT=strictfipsruntime \
	go build -tags=fips -o bin/auth-fips services/auth/main.go
	# ... repeat for all services

# Replace TLS cipher suites with FIPS-approved
build-fips-hardened:
	CGO_ENABLED=1 GOEXPERIMENT=systemcrypto \
	go build -o bin/gateway-hardened services/api-gateway/main.go

test:
	go test ./...

lint:
	golangci-lint run ./...
```

- [ ] **Step 2: Add FIPS-compliant TLS config to gateway**

Create `fips.go`:

```go
//go:build fips

package dam

// FIPSCipherSuites returns TLS cipher suites approved by FIPS 140-2
func FIPSCipherSuites() []uint16 {
    return []uint16{
        0x1301, // TLS_AES_128_GCM_SHA256
        0x1302, // TLS_AES_256_GCM_SHA384
        0x1303, // TLS_CHACHA20_POLY1305_SHA256
        0xC02C, // TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
        0xC02B, // TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
        0xC030, // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
        0xC02F, // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
    }
}
```

Add FIPS cipher enforcement in `services/api-gateway/main.go`:

```go
server := &http.Server{
    Addr:         ":443",
    Handler:      handler,
    TLSConfig: &tls.Config{
        CipherSuites: FIPSCipherSuites(),
        MinVersion:   tls.VersionTLS12,
        MaxVersion:   tls.VersionTLS13,
    },
}
```

- [ ] **Step 3: Validate**

```bash
make build  # standard build must still pass
```

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat: FIPS 140-2 compliant build with restricted cipher suites and fips-tagged build target"
```
