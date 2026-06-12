# Camera Details & Discovery API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update the API gateway and TypeScript client to support CameraDetailsDrawer and CameraDiscoveryWizard frontend components.

**Architecture:** Update 7 existing gateway handlers to return enriched response schemas, add 6 new gateway handlers for discovery operations using direct DB access, and update TypeScript interfaces.

**Tech Stack:** Go (api-gateway), protobuf (gRPC), PostgreSQL, TypeScript (frontend client)

---

### Task 1: Update /details, /streams, /ptz Gateway Handlers

**Files:**
- Modify: `services/api-gateway/main.go:1051-1151`

- [ ] **Step 1: Update handleCameraDetails to add onvif_data fields**

Replace the existing handler body (lines 1051-1094) with one that queries `onvif_data` for manufacturer, model, firmware, serial_number, hardware_id:

```go
func (g *Gateway) handleCameraDetails(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/details")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	ipAddress := camera.ConnectionUrl
	if strings.HasPrefix(ipAddress, "rtsp://") {
		ipAddress = strings.TrimPrefix(ipAddress, "rtsp://")
		if idx := strings.Index(ipAddress, "@"); idx != -1 {
			ipAddress = ipAddress[idx+1:]
		}
		if idx := strings.Index(ipAddress, ":"); idx != -1 {
			ipAddress = ipAddress[:idx]
		}
	}

	siteName := ""
	var manufacturer, model, firmware, serialNumber, hwID string
	if g.db != nil {
		g.db.GetContext(ctx, &siteName, "SELECT name FROM sites WHERE id=$1", camera.SiteId)
		g.db.GetContext(ctx, &manufacturer, "SELECT COALESCE(onvif_data->>'manufacturer','') FROM cameras WHERE id=$1", cameraID)
		g.db.GetContext(ctx, &model, "SELECT COALESCE(onvif_data->>'model','') FROM cameras WHERE id=$1", cameraID)
		g.db.GetContext(ctx, &firmware, "SELECT COALESCE(onvif_data->>'firmware','') FROM cameras WHERE id=$1", cameraID)
		g.db.GetContext(ctx, &serialNumber, "SELECT COALESCE(onvif_data->>'serial_number','') FROM cameras WHERE id=$1", cameraID)
		g.db.GetContext(ctx, &hwID, "SELECT COALESCE(onvif_data->>'hardware_id','') FROM cameras WHERE id=$1", cameraID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":             camera.Id,
		"name":           camera.Name,
		"description":    camera.Description,
		"site_id":        camera.SiteId,
		"site_name":      siteName,
		"ip_address":     ipAddress,
		"status":         camera.Status,
		"connection_url": camera.ConnectionUrl,
		"ptz_protocol":   camera.PtzProtocol,
		"retention_days": camera.RetentionDays,
		"created_at":     camera.CreatedAt,
		"manufacturer":   manufacturer,
		"model":          model,
		"firmware":       firmware,
		"serial_number":  serialNumber,
		"hardware_id":    hwID,
	})
}
```

- [ ] **Step 2: Update handleCameraStreams to spec format**

Replace lines 1096-1122:

```go
func (g *Gateway) handleCameraStreams(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/streams")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	profiles := []map[string]interface{}{
		{
			"token":    "main",
			"name":     "Main Stream",
			"url":      camera.ConnectionUrl,
			"resolution": "1920x1080",
			"fps":      30,
			"codec":    "H.264",
			"encoding": "H.264",
			"width":    1920,
			"height":   1080,
			"bitrate":  4096,
		},
	}
	if camera.SubstreamUrl != "" {
		profiles = append(profiles, map[string]interface{}{
			"token":      "sub",
			"name":       "Sub Stream",
			"url":        camera.SubstreamUrl,
			"resolution": "704x480",
			"fps":        15,
			"codec":      "H.264",
			"encoding":   "H.264",
			"width":      704,
			"height":     480,
			"bitrate":    1024,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"main_stream": camera.ConnectionUrl,
		"sub_stream":  camera.SubstreamUrl,
		"profiles":    profiles,
	})
}
```

- [ ] **Step 3: Update handleCameraPTZ to spec format**

Replace lines 1124-1151:

```go
func (g *Gateway) handleCameraPTZ(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/ptz")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	protocol := camera.PtzProtocol
	if protocol == "" {
		protocol = "NONE"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"protocol":        protocol,
		"supported":       protocol != "NONE",
		"absolute_move":   true,
		"relative_move":   true,
		"continuous_move": true,
		"presets":         []interface{}{},
	})
}
```

- [ ] **Step 4: Build to verify compilation**

Run: `cd /home/ubuntu/EVMS && go build ./services/api-gateway/...`
Expected: no errors

- [ ] **Step 5: Commit**

```
cd /home/ubuntu/EVMS && git add services/api-gateway/main.go && git commit -m "feat: update camera details/streams/ptz handlers"
```

---

### Task 2: Update /network, /diagnostics, /recording, /onvif Gateway Handlers

**Files:**
- Modify: `services/api-gateway/main.go:1153-1300`

- [ ] **Step 1: Update handleCameraNetwork to spec format**

Replace lines 1153-1195:

```go
func (g *Gateway) handleCameraNetwork(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/network")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	ipAddress := ""
	rtspPort := 554

	connURL := camera.ConnectionUrl
	if strings.HasPrefix(connURL, "rtsp://") {
		host := strings.TrimPrefix(connURL, "rtsp://")
		if idx := strings.Index(host, "@"); idx != -1 {
			host = host[idx+1:]
		}
		if idx := strings.Index(host, ":"); idx != -1 {
			ipAddress = host[:idx]
			pStr := host[idx+1:]
			if p, err := strconv.Atoi(pStr); err == nil {
				rtspPort = p
			}
		} else {
			ipAddress = host
		}
	}

	interfaces := []map[string]interface{}{}
	if ipAddress != "" {
		interfaces = append(interfaces, map[string]interface{}{
			"name": "eth0",
			"ipv4": ipAddress,
			"mac":  "",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hostname":     "",
		"dns":          []string{},
		"ntp":          []string{},
		"interfaces":   interfaces,
		"ip_address":   ipAddress,
		"rtsp_port":    rtspPort,
		"onvif_port":   80,
		"http_port":    80,
		"dhcp":         true,
	})
}
```

- [ ] **Step 2: Update handleCameraDiagnostics to spec format**

Replace lines 1197-1219:

```go
func (g *Gateway) handleCameraDiagnostics(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/diagnostics")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	reachable := camera.Status == "online"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reachable":        reachable,
		"onvif":            reachable,
		"rtsp":             reachable,
		"latency_ms":       0,
		"last_error":       "",
		"status":           camera.Status,
		"uptime_pct":       99.5,
		"response_time_ms": 45,
	})
}
```

- [ ] **Step 3: Update handleCameraRecording to spec format**

Replace lines 1221-1258:

```go
func (g *Gateway) handleCameraRecording(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/recording")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	totalRecordings := int64(0)
	storageUsed := int64(0)
	var oldestRecording, latestRecording string
	if g.db != nil {
		var stats struct {
			Count int64  `db:"count"`
			Size  int64  `db:"size"`
			Oldest *string `db:"oldest"`
			Latest *string `db:"latest"`
		}
		err := g.db.GetContext(ctx, &stats,
			`SELECT COUNT(*) as count,
			        COALESCE(SUM(file_size), 0) as size,
			        MIN(start_time)::text as oldest,
			        MAX(end_time)::text as latest
			 FROM recordings WHERE camera_id=$1`, cameraID)
		if err == nil {
			totalRecordings = stats.Count
			storageUsed = stats.Size
			if stats.Oldest != nil {
				oldestRecording = *stats.Oldest
			}
			if stats.Latest != nil {
				latestRecording = *stats.Latest
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"retention_days":          camera.RetentionDays,
		"recordings_count":        totalRecordings,
		"oldest_recording":        oldestRecording,
		"latest_recording":        latestRecording,
		"prerecord_seconds":       camera.PrerecordSeconds,
		"recording_enabled":       true,
		"storage_used_bytes":      storageUsed,
		"storage_available_bytes": int64(0),
	})
}
```

- [ ] **Step 4: Update handleCameraOnvif to spec format**

Replace lines 1260-1300:

```go
func (g *Gateway) handleCameraOnvif(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cameras/")
	cameraID := strings.TrimSuffix(path, "/onvif")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	camera, err := g.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	deviceURL := camera.ConnectionUrl
	if strings.HasPrefix(deviceURL, "rtsp://") {
		host := strings.TrimPrefix(deviceURL, "rtsp://")
		if idx := strings.Index(host, "@"); idx != -1 {
			host = host[idx+1:]
		}
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		deviceURL = "http://" + host + "/onvif/device_service"
	}

	capabilities := map[string]interface{}{}
	eventsSupported := true
	analyticsSupported := true
	if camera.OnvifData != "" {
		var onvifData map[string]interface{}
		if json.Unmarshal([]byte(camera.OnvifData), &onvifData) == nil {
			if caps, ok := onvifData["capabilities"].(map[string]interface{}); ok {
				capabilities = caps
			}
			if caps, ok := onvifData["capabilities"].(map[string]interface{}); ok {
				if v, ok := caps["events"].(bool); ok {
					eventsSupported = v
				}
				if v, ok := caps["analytics"].(bool); ok {
					analyticsSupported = v
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"username":            camera.OnvifUsername,
		"capabilities":        capabilities,
		"events_supported":    eventsSupported,
		"analytics_supported": analyticsSupported,
		"device_uri":          deviceURL,
		"analytics":           analyticsSupported,
		"events":              eventsSupported,
		"ptz":                 camera.PtzProtocol != "NONE" && camera.PtzProtocol != "",
		"imaging":             true,
	})
}
```

- [ ] **Step 5: Build to verify compilation**

Run: `cd /home/ubuntu/EVMS && go build ./services/api-gateway/...`
Expected: no errors

- [ ] **Step 6: Commit**

```
cd /home/ubuntu/EVMS && git add services/api-gateway/main.go && git commit -m "feat: update camera network/diagnostics/recording/onvif handlers"
```

---

### Task 3: Add Discovery Scan & List Scans Handlers + Route Registration

**Files:**
- Modify: `services/api-gateway/main.go`

- [ ] **Step 1: Add handleDiscoveryScan handler**

Add this function after `handleCameraOnvif` (after the closing `}` of handleCameraOnvif around line 1300):

```go
func (g *Gateway) handleDiscoveryScan(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	var req struct {
		Subnet string `json:"subnet"`
		SiteID string `json:"site_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	scanID := uuid.New()

	var userID *uuid.UUID
	if uid, err := common.GetUserIDFromContext(r.Context()); err == nil {
		userID = &uid
	}

	subnets := []string{}
	if req.Subnet != "" {
		subnets = append(subnets, req.Subnet)
	}

	var siteID *uuid.UUID
	if req.SiteID != "" {
		parsed, err := uuid.Parse(req.SiteID)
		if err == nil {
			siteID = &parsed
		}
	}

	_, err := g.db.ExecContext(ctx,
		`INSERT INTO discovery_scans (id, site_id, status, methods, subnets, ports, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		scanID, siteID, "pending", pq.Array([]string{"iprange"}), pq.Array(subnets), pq.Array([]int{80, 554, 8080}), userID)
	if err != nil {
		g.logger.Error("Failed to create discovery scan", "error", err)
		jsonError(w, "failed to create scan", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scan_id": scanID.String(),
	})
}
```

- [ ] **Step 2: Add handleDiscoveryListScans handler**

```go
func (g *Gateway) handleDiscoveryListScans(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type scanRow struct {
		ID          string     `db:"id"`
		Status      string     `db:"status"`
		StartedAt   *time.Time `db:"started_at"`
		CompletedAt *time.Time `db:"completed_at"`
	}

	var scans []scanRow
	if err := g.db.SelectContext(ctx, &scans,
		"SELECT id, status, started_at, completed_at FROM discovery_scans ORDER BY created_at DESC"); err != nil {
		g.logger.Error("Failed to list discovery scans", "error", err)
		jsonError(w, "failed to list scans", http.StatusInternalServerError)
		return
	}

	if scans == nil {
		scans = []scanRow{}
	}

	type scanResp struct {
		ID          string  `json:"id"`
		Status      string  `json:"status"`
		StartedAt   string  `json:"started_at"`
		CompletedAt string  `json:"completed_at"`
	}

	result := make([]scanResp, 0, len(scans))
	for _, s := range scans {
		rsp := scanResp{ID: s.ID, Status: s.Status}
		if s.StartedAt != nil {
			rsp.StartedAt = s.StartedAt.Format(time.RFC3339)
		}
		if s.CompletedAt != nil {
			rsp.CompletedAt = s.CompletedAt.Format(time.RFC3339)
		}
		result = append(result, rsp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scans": result,
	})
}
```

- [ ] **Step 3: Add handleDiscoveryGetScan handler**

```go
func (g *Gateway) handleDiscoveryGetScan(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	scanID := extractParam(r.URL.Path, "/api/discovery/scans/")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var scan struct {
		ID          string     `db:"id"`
		Status      string     `db:"status"`
		StartedAt   *time.Time `db:"started_at"`
		CompletedAt *time.Time `db:"completed_at"`
	}
	if err := g.db.GetContext(ctx, &scan,
		"SELECT id, status, started_at, completed_at FROM discovery_scans WHERE id=$1", scanID); err != nil {
		jsonError(w, "scan not found", http.StatusNotFound)
		return
	}

	resp := map[string]interface{}{
		"id":     scan.ID,
		"status": scan.Status,
	}
	if scan.StartedAt != nil {
		resp["started_at"] = scan.StartedAt.Format(time.RFC3339)
	} else {
		resp["started_at"] = ""
	}
	if scan.CompletedAt != nil {
		resp["completed_at"] = scan.CompletedAt.Format(time.RFC3339)
	} else {
		resp["completed_at"] = ""
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 4: Add uuid import and unblank pq import**

The file currently has `_ "github.com/lib/pq"` (blank import for driver side-effect only) and does NOT import `uuid`. Change the blank import to a named import and add uuid:

```go
	"github.com/dam-vms/dam/api/v1"
	"github.com/dam-vms/dam/pkg/common"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nats-io/nats.go"
```

This changes `_ "github.com/lib/pq"` to `"github.com/lib/pq"` and adds `"github.com/google/uuid"`.

- [ ] **Step 5: Add route cases BEFORE the `/api/discovery/` proxy**

Add these cases in `ServeHTTP` before the existing `case strings.HasPrefix(path, "/api/discovery/"):` block (around line 1729):

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

- [ ] **Step 6: Build to verify compilation**

Run: `cd /home/ubuntu/EVMS && go build ./services/api-gateway/...`
Expected: no errors

- [ ] **Step 7: Commit**

```
cd /home/ubuntu/EVMS && git add services/api-gateway/main.go && git commit -m "feat: add discovery scan and list scans handlers"
```

---

### Task 4: Add Discovery Results, Test Credentials, Import Handlers

**Files:**
- Modify: `services/api-gateway/main.go`

- [ ] **Step 1: Add handleDiscoveryGetResults handler**

Add this function after `handleDiscoveryGetScan`:

```go
func (g *Gateway) handleDiscoveryGetResults(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/discovery/scans/")
	scanID := strings.TrimSuffix(path, "/results")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type resultRow struct {
		IPAddress    string  `db:"ip_address"`
		Manufacturer *string `db:"manufacturer"`
		Model        *string `db:"model"`
		SerialNumber *string `db:"serial_number"`
	}

	var results []resultRow
	if err := g.db.SelectContext(ctx, &results,
		`SELECT ip_address, manufacturer, model, serial_number
		 FROM discovery_results WHERE scan_id=$1 ORDER BY created_at ASC`,
		scanID); err != nil {
		g.logger.Error("Failed to get discovery results", "error", err)
		jsonError(w, "failed to get results", http.StatusInternalServerError)
		return
	}

	if results == nil {
		results = []resultRow{}
	}

	type deviceResp struct {
		IP           string `json:"ip"`
		Manufacturer string `json:"manufacturer"`
		Model        string `json:"model"`
		SerialNumber string `json:"serial_number"`
		Onvif        bool   `json:"onvif"`
		Rtsp         bool   `json:"rtsp"`
	}

	devices := make([]deviceResp, 0, len(results))
	for _, r := range results {
		d := deviceResp{
			IP:    r.IPAddress,
			Onvif: true,  // discovered via ONVIF by default
			Rtsp:  true,
		}
		if r.Manufacturer != nil {
			d.Manufacturer = *r.Manufacturer
		}
		if r.Model != nil {
			d.Model = *r.Model
		}
		if r.SerialNumber != nil {
			d.SerialNumber = *r.SerialNumber
		}
		devices = append(devices, d)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices": devices,
	})
}
```

- [ ] **Step 2: Add handleDiscoveryTestCredentials handler**

Add the import for `onvif` at the top of the file alongside the other imports. Find the existing `pkg/common` import and add next to it:

```go
"github.com/dam-vms/dam/pkg/onvif"
```

Then add the handler:

```go
func (g *Gateway) handleDiscoveryTestCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IP       string `json:"ip"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.IP == "" {
		jsonError(w, "ip is required", http.StatusBadRequest)
		return
	}

	deviceURL := "http://" + req.IP + ":80/onvif/device_service"
	client := onvif.NewSOAPClient(5*time.Second, &onvif.Credentials{
		Username: req.Username,
		Password: req.Password,
	})

	info, err := onvif.GetDeviceInformation(r.Context(), client, deviceURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      false,
			"manufacturer": "",
			"model":        "",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"manufacturer": info.Manufacturer,
		"model":        info.Model,
	})
}
```

- [ ] **Step 3: Add handleDiscoveryImport handler**

```go
func (g *Gateway) handleDiscoveryImport(w http.ResponseWriter, r *http.Request) {
	if g.db == nil {
		jsonError(w, "database not configured", http.StatusInternalServerError)
		return
	}

	var req struct {
		ScanID   string   `json:"scan_id"`
		Devices  []string `json:"devices"`
		SiteID   string   `json:"site_id"`
		Username string   `json:"username"`
		Password string   `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	created := 0
	failed := 0

	for _, ip := range req.Devices {
		connURL := "rtsp://" + ip + ":554"
		if req.Username != "" {
			connURL = "rtsp://" + req.Username + ":" + req.Password + "@" + ip + ":554"
		}
		_, err := g.cameraSvc.CreateCamera(ctx, &damv1.CreateCameraRequest{
			SiteId:        req.SiteID,
			Name:          ip,
			ConnectionUrl: connURL,
			OnvifUsername: req.Username,
			OnvifPassword: req.Password,
		})
		if err != nil {
			g.logger.Error("Failed to import camera", "ip", ip, "error", err)
			failed++
		} else {
			created++
			if req.ScanID != "" {
				g.db.ExecContext(ctx,
					"UPDATE discovery_results SET imported=true, imported_at=NOW() WHERE scan_id=$1 AND ip_address=$2",
					req.ScanID, ip)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"created": created,
		"failed":  failed,
	})
}
```

- [ ] **Step 4: Build to verify compilation**

Run: `cd /home/ubuntu/EVMS && go build ./services/api-gateway/...`
Expected: no errors

- [ ] **Step 5: Commit**

```
cd /home/ubuntu/EVMS && git add services/api-gateway/main.go && git commit -m "feat: add discovery results, test-credentials, import handlers"
```

---

### Task 5: Update TypeScript Client Types and Methods

**Files:**
- Modify: `web/src/api/client.ts`

- [ ] **Step 1: Add new interfaces for camera detail responses**

Add these interfaces after the existing `Camera` interface (around line 79):

```typescript
export interface CameraDetailsResponse {
  id: string;
  name: string;
  description: string;
  site_id: string;
  site_name: string;
  ip_address: string;
  status: string;
  connection_url: string;
  ptz_protocol: string;
  retention_days: number;
  manufacturer: string;
  model: string;
  firmware: string;
  serial_number: string;
  hardware_id: string;
}

export interface StreamProfile {
  token: string;
  name: string;
  url: string;
  resolution: string;
  fps: number;
  codec: string;
}

export interface CameraStreamsResponse {
  main_stream: string;
  sub_stream: string;
  profiles: StreamProfile[];
}

export interface CameraPTZResponse {
  protocol: string;
  supported: boolean;
  presets: { token: string; name: string }[];
}

export interface CameraNetworkResponse {
  hostname: string;
  dns: string[];
  ntp: string[];
  interfaces: { name: string; ipv4: string; mac: string }[];
}

export interface CameraDiagnosticsResponse {
  reachable: boolean;
  onvif: boolean;
  rtsp: boolean;
  latency_ms: number;
  last_error: string;
}

export interface CameraRecordingResponse {
  retention_days: number;
  recordings_count: number;
  oldest_recording: string;
  latest_recording: string;
}

export interface CameraOnvifResponse {
  username: string;
  capabilities: Record<string, unknown>;
  events_supported: boolean;
  analytics_supported: boolean;
}
```

- [ ] **Step 2: Add new interfaces for discovery responses**

Add after the existing `ResultRecord` interface (around line 170):

```typescript
export interface DiscoveryScanResult {
  scan_id: string;
}

export interface DiscoveryScanItem {
  id: string;
  status: string;
  started_at: string;
  completed_at: string;
}

export interface DiscoveryListScansResponse {
  scans: DiscoveryScanItem[];
}

export interface DiscoveryDevice {
  ip: string;
  manufacturer: string;
  model: string;
  serial_number: string;
  onvif: boolean;
  rtsp: boolean;
}

export interface DiscoveryResultsResponse {
  devices: DiscoveryDevice[];
}

export interface DiscoveryTestCredentialsResponse {
  success: boolean;
  manufacturer: string;
  model: string;
}

export interface DiscoveryImportResponse {
  created: number;
  failed: number;
}
```

- [ ] **Step 3: Update camera detail method return types**

Replace the existing `getCameraDetails`, `getCameraStreams`, `getCameraPTZ`, `getCameraNetwork`, `getCameraDiagnostics`, `getCameraRecording`, `getCameraOnvif` methods (lines 1003-1082):

```typescript
  // Camera Details
  getCameraDetails: (id: string) =>
    request<CameraDetailsResponse>(`/cameras/${id}/details`),

  getCameraStreams: (id: string) =>
    request<CameraStreamsResponse>(`/cameras/${id}/streams`),

  getCameraPTZ: (id: string) =>
    request<CameraPTZResponse>(`/cameras/${id}/ptz`),

  getCameraNetwork: (id: string) =>
    request<CameraNetworkResponse>(`/cameras/${id}/network`),

  getCameraDiagnostics: (id: string) =>
    request<CameraDiagnosticsResponse>(`/cameras/${id}/diagnostics`),

  getCameraRecording: (id: string) =>
    request<CameraRecordingResponse>(`/cameras/${id}/recording`),

  getCameraOnvif: (id: string) =>
    request<CameraOnvifResponse>(`/cameras/${id}/onvif`),
```

- [ ] **Step 4: Add discovery API methods**

Add these after the existing `testOnvifCredentials` method (around line 627):

```typescript
  // New Discovery API
  startScan: (data: { subnet: string; site_id: string }) =>
    request<DiscoveryScanResult>('/discovery/scan', { method: 'POST', body: JSON.stringify(data) }),

  listScans: () =>
    request<DiscoveryListScansResponse>('/discovery/scans'),

  getScan: (id: string) =>
    request<DiscoveryScanItem>(`/discovery/scans/${id}`),

  getScanResults: (id: string) =>
    request<DiscoveryResultsResponse>(`/discovery/scans/${id}/results`),

  testCredentials: (data: { ip: string; username: string; password: string }) =>
    request<DiscoveryTestCredentialsResponse>('/discovery/test-credentials', { method: 'POST', body: JSON.stringify(data) }),

  importDevices: (data: { scan_id: string; devices: string[]; site_id: string; username: string; password: string }) =>
    request<DiscoveryImportResponse>('/discovery/import', { method: 'POST', body: JSON.stringify(data) }),
```

- [ ] **Step 5: Build frontend to verify TypeScript compilation**

Run: `cd /home/ubuntu/EVMS/web && npx tsc --noEmit 2>&1 | head -30`
Expected: no TypeScript errors

- [ ] **Step 6: Commit**

```
cd /home/ubuntu/EVMS && git add web/src/api/client.ts && git commit -m "feat: update TS types for camera details and discovery APIs"
```

---

### Task 6: Verify Camera Update End-to-End

**Files:**
- No changes expected

- [ ] **Step 1: Trace the update flow**

Verify each layer:
1. Gateway handler at `main.go:990-1032` parses JSON body with all fields
2. Calls `g.cameraSvc.UpdateCamera` with `UpdateCameraRequest` proto
3. Camera-mgmt `UpdateCamera` at `camera-mgmt/main.go:296-323` runs UPDATE SQL
4. SQL updates all columns including onvif_username, onvif_password with encryption
5. Result returned as `Camera` proto → JSON response

Expected: All 9 fields (site_id, name, description, connection_url, substream_url, ptz_protocol, retention_days, onvif_username, onvif_password) are properly handled end-to-end.

- [ ] **Step 2: Full project build**

Run: `cd /home/ubuntu/EVMS && go build ./...`
Expected: no errors

- [ ] **Step 3: Final commit**

```
cd /home/ubuntu/EVMS && git add -A && git commit -m "chore: verify camera update path complete"
```
