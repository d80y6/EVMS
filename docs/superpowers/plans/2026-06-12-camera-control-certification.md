# Camera Control (PTZ) Domain Certification

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement comprehensive tests and certify the camera-control domain (AC-138: PTZ command operations, AC-139: PTZ preset management).

**Architecture:** The camera-control service (`services/camera-control/main.go`, 1866 lines) is a single-file package-main Go service with HTTP handlers that delegate to the ONVIF package for PTZ commands, VAPIX CGI for Axis cameras, and Hikvision ISAPI for Hikvision cameras. Tests use httptest for HTTP handler testing, standard Go testing + testify, and mock gRPC camera service clients.

**Tech Stack:** Go 1.24, testing (+ testify), httptest, google.golang.org/grpc (mocked)

---

### Task 1: Write pure function and utility tests

**Files:**
- Create: `services/camera-control/main_test.go` (initial 200 lines)

**Test scope:** All pure functions + router logic + handler validation

- [ ] **Step 1: Write the test file for pure functions and router/validation**

Write `services/camera-control/main_test.go` with:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	damv1 "github.com/dam-vms/dam/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// mockCameraClient implements damv1.CameraServiceClient for testing.
type mockCameraClient struct {
	cameras map[string]*damv1.Camera
}

func (m *mockCameraClient) GetCamera(_ context.Context, in *damv1.GetCameraRequest, _ ...grpc.CallOption) (*damv1.Camera, error) {
	cam, ok := m.cameras[in.Id]
	if !ok {
		return nil, io.EOF
	}
	return cam, nil
}

func (m *mockCameraClient) ListCameras(_ context.Context, _ *damv1.ListCamerasRequest, _ ...grpc.CallOption) (*damv1.ListCamerasResponse, error) {
	return nil, nil
}
func (m *mockCameraClient) CreateCamera(_ context.Context, _ *damv1.CreateCameraRequest, _ ...grpc.CallOption) (*damv1.Camera, error) {
	return nil, nil
}
func (m *mockCameraClient) UpdateCamera(_ context.Context, _ *damv1.UpdateCameraRequest, _ ...grpc.CallOption) (*damv1.Camera, error) {
	return nil, nil
}
func (m *mockCameraClient) DeleteCamera(_ context.Context, _ *damv1.DeleteCameraRequest, _ ...grpc.CallOption) (*damv1.DeleteCameraResponse, error) {
	return nil, nil
}
func (m *mockCameraClient) ListSites(_ context.Context, _ *damv1.ListSitesRequest, _ ...grpc.CallOption) (*damv1.ListSitesResponse, error) {
	return nil, nil
}
func (m *mockCameraClient) CreateSite(_ context.Context, _ *damv1.CreateSiteRequest, _ ...grpc.CallOption) (*damv1.Site, error) {
	return nil, nil
}
func (m *mockCameraClient) SmartSearch(_ context.Context, _ *damv1.SmartSearchRequest, _ ...grpc.CallOption) (*damv1.SmartSearchResponse, error) {
	return nil, nil
}

func newTestService(cameras map[string]*damv1.Camera) *PTZService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &PTZService{
		config:    DefaultPTZConfig(),
		logger:    logger,
		cameraSvc: &mockCameraClient{cameras: cameras},
		httpCli:   &http.Client{Timeout: 5 * time.Second},
	}
}

func newONVIFCamera(id, name, url string, onvifPort int) *damv1.Camera {
	config, _ := json.Marshal(map[string]interface{}{
		"onvif_port": onvifPort,
		"is_onvif":   true,
	})
	return &damv1.Camera{
		Id:            id,
		Name:          name,
		ConnectionUrl: url,
		Status:        "online",
		PtzProtocol:   "onvif",
		Config:        string(config),
		OnvifUsername: "admin",
		OnvifPassword: "password",
	}
}

// ========== Utility Function Tests ==========

func TestResolvePTZProtocol(t *testing.T) {
	svc := newTestService(nil)
	tests := []struct {
		name     string
		protocol string
		want     string
	}{
		{"onvif protocol", "onvif", "onvif"},
		{"vapix protocol", "vapix", "vapix"},
		{"hikvision protocol", "hikvision", "hikvision"},
		{"empty defaults to onvif", "", "onvif"},
		{"none defaults to onvif", "none", "onvif"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.resolvePTZProtocol(&damv1.Camera{PtzProtocol: tt.protocol})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetONVIFPort(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   int
	}{
		{"valid onvif port from config", `{"onvif_port":8080,"is_onvif":true}`, 8080},
		{"default port when valid onvif without port", `{"is_onvif":true}`, 8000},
		{"zero when is_onvif is false", `{"is_onvif":false}`, 0},
		{"zero when config is empty", "", 0},
		{"zero when config is invalid json", "not-json", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getONVIFPort(&damv1.Camera{Config: tt.config})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsONVIFDisabled(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		config   string
		want     bool
	}{
		{"disabled when onvif with no port", "onvif", "", true},
		{"not disabled when onvif with port", "onvif", `{"onvif_port":8080,"is_onvif":true}`, false},
		{"not disabled when non-onvif protocol", "vapix", "", false},
		{"not disabled when non-onvif with empty protocol", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isONVIFDisabled(&damv1.Camera{PtzProtocol: tt.protocol, Config: tt.config})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractParam(t *testing.T) {
	assert.Equal(t, "cam-1", extractParam("/cameras/cam-1/ptz/move", "/cameras/"))
	assert.Equal(t, "cam-2", extractParam("/cameras/cam-2", "/cameras/"))
	assert.Equal(t, "single", extractParam("/cameras/single", "/cameras/"))
}

func TestDefaultPTZConfig(t *testing.T) {
	os.Setenv("CAMERA_CONTROL_PORT", ":9090")
	defer os.Unsetenv("CAMERA_CONTROL_PORT")

	cfg := DefaultPTZConfig()
	assert.Equal(t, ":9090", cfg.Port)
	assert.Equal(t, "camera-mgmt:50051", cfg.CameraSvcAddr)
	assert.Equal(t, ":2112", cfg.MetricsAddr)
	assert.Equal(t, 10*time.Second, cfg.RequestTimeout)
}

// ========== Router Tests ==========

func TestPTZRouter_InvalidPath(t *testing.T) {
	svc := newTestService(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/invalid", nil)
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPTZRouter_TooFewParts(t *testing.T) {
	svc := newTestService(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/", nil)
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/cameras//ptz/move", nil)
	svc.handlePTZRouter(rr2, req2)
	assert.Equal(t, http.StatusBadRequest, rr2.Code)
}

func TestPTZRouter_UnknownAction(t *testing.T) {
	svc := newTestService(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/unknown", nil)
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ========== Handler Validation Tests ==========

func TestHandleMove_InvalidJSON(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/move",
		bytes.NewReader([]byte(`not-json`)))
	req.URL.Path = "/cameras/cam-1/ptz/move"
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Contains(t, body["error"], "invalid request")
}

func TestHandleMove_WrongMethod(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cameras/cam-1/ptz/move", nil)
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleZoom_InvalidJSON(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/zoom",
		bytes.NewReader([]byte(`not-json`)))
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleStop_WrongMethod(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cameras/cam-1/ptz/stop", nil)
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleGotoPreset_InvalidJSON(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/preset/goto",
		bytes.NewReader([]byte(`not-json`)))
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleAbsoluteMove_InvalidJSON(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/absolute-move",
		bytes.NewReader([]byte(`not-json`)))
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleRelativeMove_InvalidJSON(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/relative-move",
		bytes.NewReader([]byte(`not-json`)))
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleSetPreset_InvalidJSON(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/presets",
		bytes.NewReader([]byte(`not-json`)))
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleRemovePreset_MissingToken(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/cameras/cam-1/ptz/presets", nil)
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ========== Camera Not Found Tests ==========

func TestPTZRouter_CameraNotFound(t *testing.T) {
	svc := newTestService(nil) // no cameras
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/nonexistent/ptz/move",
		bytes.NewReader([]byte(`{"direction":"up"}`)))
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "camera not found", body["error"])
}

func TestCameraRouter_CameraNotFound(t *testing.T) {
	svc := newTestService(nil) // no cameras
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cameras/nonexistent/profiles", nil)
	svc.handleCameraRouter(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "camera not found", body["error"])
}
```

- [ ] **Step 2: Run the tests to verify they compile and pass**

Run: `cd /home/ubuntu/EVMS && go test ./services/camera-control/ -v -count=1 -run 'TestResolvePTZProtocol|TestGetONVIFPort|TestIsONVIFDisabled|TestExtractParam|TestDefaultPTZConfig|TestPTZRouter|TestHandleMove|TestHandleZoom|TestHandleStop|TestHandleGotoPreset|TestHandleAbsoluteMove|TestHandleRelativeMove|TestHandleSetPreset|TestHandleRemovePreset|TestCameraNotFound'`

Expected: ~20 tests pass, covering utility functions, routing, and handler validation.

- [ ] **Step 3: Commit**

```bash
git add services/camera-control/main_test.go
git commit -m "test(camera-control): add pure function, router, and validation tests"
```

---

### Task 2: Add VAPIX and Hikvision command tests

**Files:**
- Modify: `services/camera-control/main_test.go` (append ~150 lines)

- [ ] **Step 1: Write VAPIX CGI command tests with mock HTTP server**

Append to main_test.go:

```go
// ========== VAPIX Command Tests ==========

func TestVapixCommand_Move(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "GET", r.Method)
        assert.Contains(t, r.URL.String(), "move=up")
        assert.Contains(t, r.URL.String(), "speed=50")
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    svc := newTestService(nil)
    err := svc.vapixCommand(server.URL, "move", "up", 0.5)
    assert.NoError(t, err)
}

func TestVapixCommand_Stop(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Contains(t, r.URL.String(), "continuouszoommove=0")
        assert.Contains(t, r.URL.String(), "continuouspantiltmove=0")
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    svc := newTestService(nil)
    err := svc.vapixCommand(server.URL, "stop", "", 0)
    assert.NoError(t, err)
}

func TestVapixCommand_GotoPreset(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Contains(t, r.URL.String(), "gotoserverpresetnumber=1")
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    svc := newTestService(nil)
    err := svc.vapixCommand(server.URL, "goto_preset", "1", 0)
    assert.NoError(t, err)
}

func TestVapixCommand_NonOKStatus(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer server.Close()

    svc := newTestService(nil)
    err := svc.vapixCommand(server.URL, "move", "up", 0.5)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "returned status 500")
}

func TestVapixCommand_UnknownCommand(t *testing.T) {
    svc := newTestService(nil)
    err := svc.vapixCommand("http://localhost", "unknown_cmd", "", 0)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "unknown VAPIX command")
}
```

- [ ] **Step 2: Write Hikvision ISAPI command tests with mock HTTP server**

Append to main_test.go:

```go
// ========== Hikvision Command Tests ==========

func TestHikvisionCommand_Move(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "PUT", r.Method)
        assert.Contains(t, r.URL.Path, "/ISAPI/PTZCtrl/channels/1/continuous")
        body, _ := io.ReadAll(r.Body)
        assert.Contains(t, string(body), "<Pan>50</Pan>")
        assert.Contains(t, string(body), "<Tilt>50</Tilt>")
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    svc := newTestService(nil)
    err := svc.hikvisionCommand(server.URL, "move", "down-right", 0.5)
    assert.NoError(t, err)
}

func TestHikvisionCommand_Stop(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "PUT", r.Method)
        assert.Contains(t, r.URL.Path, "/continuous")
        body, _ := io.ReadAll(r.Body)
        assert.Contains(t, string(body), "<Pan>0</Pan>")
        assert.Contains(t, string(body), "<Tilt>0</Tilt>")
        assert.Contains(t, string(body), "<Zoom>0</Zoom>")
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    svc := newTestService(nil)
    err := svc.hikvisionCommand(server.URL, "stop", "", 0)
    assert.NoError(t, err)
}

func TestHikvisionCommand_GotoPreset(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Contains(t, r.URL.Path, "/presets/1/goto")
        assert.Equal(t, "PUT", r.Method)
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    svc := newTestService(nil)
    err := svc.hikvisionCommand(server.URL, "goto_preset", "1", 0)
    assert.NoError(t, err)
}

func TestHikvisionCommand_SetPreset(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Contains(t, r.URL.Path, "/presets/1")
        assert.Equal(t, "PUT", r.Method)
        body, _ := io.ReadAll(r.Body)
        assert.Contains(t, string(body), "<presetID>1</presetID>")
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    svc := newTestService(nil)
    err := svc.hikvisionCommand(server.URL, "set_preset", "1", 0)
    assert.NoError(t, err)
}

func TestHikvisionCommand_NonOKStatus(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusBadRequest)
    }))
    defer server.Close()

    svc := newTestService(nil)
    err := svc.hikvisionCommand(server.URL, "move", "up", 0.5)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "returned status 400")
}

func TestHikvisionCommand_UnknownCommand(t *testing.T) {
    svc := newTestService(nil)
    err := svc.hikvisionCommand("http://localhost", "unknown_cmd", "", 0)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "unknown Hikvision command")
}
```

- [ ] **Step 3: Write ONVIF command tests with mock SOAP server**

Append to main_test.go:

```go
// ========== ONVIF Command Tests ==========

func TestOnvifCommand_Move(t *testing.T) {
    // Set up a test server that handles GetProfiles and ContinuousMove SOAP calls
    callCount := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        callCount++
        body, _ := io.ReadAll(r.Body)
        w.Header().Set("Content-Type", "application/soap+xml")
        
        if strings.Contains(string(body), "GetProfiles") {
            w.Write([]byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetProfilesResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <Profiles token="profile_1"><Name>Main</Name></Profiles>
    </GetProfilesResponse>
  </s:Body>
</s:Envelope>`))
        } else if strings.Contains(string(body), "ContinuousMove") {
            assert.Contains(t, string(body), "PanTilt")
            w.Write([]byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <ContinuousMoveResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/>
  </s:Body>
</s:Envelope>`))
        } else {
            t.Logf("Unexpected SOAP call: %s", string(body))
            w.WriteHeader(http.StatusInternalServerError)
        }
    }))
    defer server.Close()

    // The camera's ConnectionUrl is the test server URL, but ONVIF port separate
    // We need to handle the URL resolution correctly:
    // getONVIFPort(camera) returns 0 when no valid config, which causes issues
    // So we set up the camera with the test server's port as the ONVIF port
    
    camera := newONVIFCamera("cam-onvif", "ONVIF Cam", server.URL, 0)
    // Override config to match the test server port
    u, _ := url.Parse(server.URL)
    port, _ := strconv.Atoi(u.Port())
    config, _ := json.Marshal(map[string]interface{}{
        "onvif_port": port,
        "is_onvif":   true,
    })
    camera.Config = string(config)
    
    svc := newTestService(map[string]*damv1.Camera{
        "cam-onvif": camera,
    })
    
    err := svc.onvifCommand(server.URL, "move", "up", 0.5, camera)
    assert.NoError(t, err)
    assert.GreaterOrEqual(t, callCount, 2) // GetProfiles + ContinuousMove
}

func TestOnvifCommand_Stop(t *testing.T) {
    callCount := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        callCount++
        body, _ := io.ReadAll(r.Body)
        w.Header().Set("Content-Type", "application/soap+xml")
        
        if strings.Contains(string(body), "GetProfiles") {
            w.Write([]byte(profilesResponse))
        } else if strings.Contains(string(body), "Stop") {
            w.Write([]byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <StopResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/>
  </s:Body>
</s:Envelope>`))
        }
    }))
    defer server.Close()

    camera := newONVIFCamera("cam-1", "Cam", server.URL, portFromURL(t, server.URL))
    
    svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})
    err := svc.onvifCommand(server.URL, "stop", "", 0, camera)
    assert.NoError(t, err)
}

func TestOnvifCommand_UnknownCommand(t *testing.T) {
    svc := newTestService(nil)
    err := svc.onvifCommand("http://localhost", "unknown", "", 0, &damv1.Camera{})
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "unknown ONVIF command")
}
```

- [ ] **Step 4: Run all tests**

Run: `cd /home/ubuntu/EVMS && go test ./services/camera-control/ -v -count=1`

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/camera-control/main_test.go
git commit -m "test(camera-control): add VAPIX, Hikvision, ONVIF command tests with mock servers"
```

---

### Task 3: Add ONVIF handler and preset listing tests

**Files:**
- Modify: `services/camera-control/main_test.go` (append ~100 lines)

- [ ] **Step 1: Write handler tests that exercise the full PTZ service stack with mock ONVIF servers**

Append to main_test.go:

```go
// ========== Handler End-to-End Tests ==========

const profilesResponse = `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetProfilesResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <Profiles token="profile_1">
        <Name>Main</Name>
        <VideoSource token="vs_1"/>
      </Profiles>
    </GetProfilesResponse>
  </s:Body>
</s:Envelope>`

func portFromURL(t *testing.T, rawURL string) int {
    t.Helper()
    u, err := url.Parse(rawURL)
    require.NoError(t, err)
    p, err := strconv.Atoi(u.Port())
    require.NoError(t, err)
    return p
}

func TestHandleMove_ValidRequest(t *testing.T) {
    onvifCalled := false
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        w.Header().Set("Content-Type", "application/soap+xml")
        if strings.Contains(string(body), "GetProfiles") {
            w.Write([]byte(profilesResponse))
        } else if strings.Contains(string(body), "ContinuousMove") {
            onvifCalled = true
            assert.Contains(t, string(body), "PanTilt")
            w.Write([]byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <ContinuousMoveResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/>
  </s:Body>
</s:Envelope>`))
        }
    }))
    defer server.Close()

    camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
    svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/move",
        bytes.NewReader([]byte(`{"direction":"up","speed":0.5}`)))
    svc.handlePTZRouter(rr, req)

    assert.Equal(t, http.StatusOK, rr.Code)
    assert.True(t, onvifCalled, "ContinuousMove should have been called on the ONVIF server")
}

func TestHandleListPresets(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        w.Header().Set("Content-Type", "application/soap+xml")
        if strings.Contains(string(body), "GetProfiles") {
            w.Write([]byte(profilesResponse))
        } else if strings.Contains(string(body), "GetPresets") {
            w.Write([]byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetPresetsResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl">
      <Preset token="1"><Name>Entrance</Name></Preset>
      <Preset token="2"><Name>Parking</Name></Preset>
    </GetPresetsResponse>
  </s:Body>
</s:Envelope>`))
        }
    }))
    defer server.Close()

    camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
    svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/cameras/cam-1/ptz/presets", nil)
    svc.handlePTZRouter(rr, req)

    assert.Equal(t, http.StatusOK, rr.Code)
    var resp presetResponse
    err := json.Unmarshal(rr.Body.Bytes(), &resp)
    require.NoError(t, err)
    assert.Len(t, resp.Presets, 2)
    assert.Equal(t, "Entrance", resp.Presets[0].Name)
}

func TestHandlePTZStatus(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        w.Header().Set("Content-Type", "application/soap+xml")
        if strings.Contains(string(body), "GetProfiles") {
            w.Write([]byte(profilesResponse))
        } else if strings.Contains(string(body), "GetStatus") {
            w.Write([]byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetStatusResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl">
      <PTZStatus><MoveStatus><PanTilt>IDLE</PanTilt></MoveStatus></PTZStatus>
    </GetStatusResponse>
  </s:Body>
</s:Envelope>`))
        }
    }))
    defer server.Close()

    camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
    svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/cameras/cam-1/ptz/status", nil)
    svc.handlePTZRouter(rr, req)

    assert.Equal(t, http.StatusOK, rr.Code)
}
```

- [ ] **Step 2: Add ONVIF disabled tests for all PTZ handlers**

```go
// ========== ONVIF Disabled Handler Tests ==========

func TestHandleMove_ONVIFDisabled(t *testing.T) {
    svc := newTestService(map[string]*damv1.Camera{
        "cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: "{}", ConnectionUrl: "http://localhost"},
    })
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/move",
        bytes.NewReader([]byte(`{"direction":"up"}`)))
    svc.handlePTZRouter(rr, req)
    assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleStop_ONVIFDisabled(t *testing.T) {
    svc := newTestService(map[string]*damv1.Camera{
        "cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: "{}", ConnectionUrl: "http://localhost"},
    })
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/stop", nil)
    svc.handlePTZRouter(rr, req)
    assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleGotoPreset_ONVIFDisabled(t *testing.T) {
    svc := newTestService(map[string]*damv1.Camera{
        "cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: "{}", ConnectionUrl: "http://localhost"},
    })
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/preset/goto",
        bytes.NewReader([]byte(`{"preset_id":1}`)))
    svc.handlePTZRouter(rr, req)
    assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleAbsoluteMove_ONVIFDisabled(t *testing.T) {
    svc := newTestService(map[string]*damv1.Camera{
        "cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: "{}", ConnectionUrl: "http://localhost"},
    })
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/absolute-move",
        bytes.NewReader([]byte(`{"pan":0,"tilt":0}`)))
    svc.handlePTZRouter(rr, req)
    assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleGotoHome_ONVIFDisabled(t *testing.T) {
    svc := newTestService(map[string]*damv1.Camera{
        "cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: "{}", ConnectionUrl: "http://localhost"},
    })
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/home", nil)
    svc.handlePTZRouter(rr, req)
    assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleSetHome_ONVIFDisabled(t *testing.T) {
    svc := newTestService(map[string]*damv1.Camera{
        "cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: "{}", ConnectionUrl: "http://localhost"},
    })
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/set-home", nil)
    svc.handlePTZRouter(rr, req)
    assert.Equal(t, http.StatusBadRequest, rr.Code)
}
```

- [ ] **Step 3: Run all tests**

Run: `cd /home/ubuntu/EVMS && go test ./services/camera-control/ -v -count=1`

Expected: All tests pass (30+ tests).

- [ ] **Step 4: Commit**

```bash
git add services/camera-control/main_test.go
git commit -m "test(camera-control): add full ONVIF handler end-to-end and disabled tests"
```

---

### Task 4: Final verification and certification

**Files:**
- Modify: `DOMAIN_STATUS.md`

- [ ] **Step 1: Run the complete test suite**

Run: `cd /home/ubuntu/EVMS && go test ./pkg/... ./services/... -count=1`

Expected: All existing tests pass plus new camera-control tests.

- [ ] **Step 2: Run Makefile tests**

Run: `cd /home/ubuntu/EVMS && make test`

Expected: All tests pass.

- [ ] **Step 3: Update DOMAIN_STATUS.md to mark camera-control as certified**

Update camera-control row to:
```
| 6 | camera-control | ✓ | ✓ | ✓ | ✅ | All criteria met |
```

- [ ] **Step 4: Commit**

```bash
git add DOMAIN_STATUS.md
git commit -m "docs: certify camera-control domain with PTZ tests"
```
