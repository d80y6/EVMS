package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	damv1 "github.com/dam-vms/dam/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

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
		config:     DefaultPTZConfig(),
		logger:     logger,
		cameraSvc:  &mockCameraClient{cameras: cameras},
		httpCli:    &http.Client{Timeout: 5 * time.Second},
		ptzLimiter: newPTZRateLimiter(defaultPTZRateLimitConfig()),
	}
}

func newONVIFCamera(id, name, serverURL string, onvifPort int) *damv1.Camera {
	config, _ := json.Marshal(map[string]interface{}{
		"onvif_port": onvifPort,
		"is_onvif":   true,
	})
	return &damv1.Camera{
		Id:            id,
		Name:          name,
		ConnectionUrl: serverURL,
		Status:        "online",
		PtzProtocol:   "onvif",
		Config:        string(config),
		OnvifUsername: "admin",
		OnvifPassword: "password",
	}
}

func portFromURL(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	p, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return p
}

const profilesSOAPResponse = `<?xml version="1.0"?>
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

func onvifTestServer(t *testing.T, handlers map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.Body.Close()

		w.Header().Set("Content-Type", "application/soap+xml")
		for pattern, response := range handlers {
			if strings.Contains(string(body), pattern) {
				w.Write([]byte(response))
				return
			}
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

func TestResolvePTZProtocol(t *testing.T) {
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
			got := resolvePTZProtocol(&damv1.Camera{PtzProtocol: tt.protocol})
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
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/unknown", nil)
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleMove_InvalidJSON(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/move",
		bytes.NewReader([]byte(`not-json`)))
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

func TestHandleGotoPreset_FromPathParam(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/presets/1/goto", nil)
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
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
	svc := newTestService(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/cameras/cam-1/ptz", nil)
	svc.handleRemovePreset(rr, req, &damv1.Camera{Id: "cam-1", PtzProtocol: "onvif", Config: `{"onvif_port":8080,"is_onvif":true}`, ConnectionUrl: "http://localhost:8080"})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPTZRouter_CameraNotFound(t *testing.T) {
	svc := newTestService(nil)
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
	svc := newTestService(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cameras/nonexistent/profiles", nil)
	svc.handleCameraRouter(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "camera not found", body["error"])
}

func TestHandleMove_ONVIFDisabled(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: "{}", ConnectionUrl: "http://localhost"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/move",
		bytes.NewReader([]byte(`{"direction":"up"}`)))
	svc.handlePTZRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Contains(t, body["error"], "ONVIF disabled")
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

func TestHandleZoom_ONVIFDisabled(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: "{}", ConnectionUrl: "http://localhost"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/zoom",
		bytes.NewReader([]byte(`{"zoom":0.5}`)))
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

func TestHandleRelativeMove_ONVIFDisabled(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", PtzProtocol: "onvif", Config: "{}", ConnectionUrl: "http://localhost"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/relative-move",
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

func TestHandleMove_ValidRequest(t *testing.T) {
	onvifCalled := false
	server := onvifTestServer(t, map[string]string{
		"GetProfiles":     profilesSOAPResponse,
		"ContinuousMove":  `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><ContinuousMoveResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`,
	})
	defer server.Close()

	camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
	camera.Config = `{"onvif_port":` + strconv.Itoa(portFromURL(t, server.URL)) + `,"is_onvif":true}`
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/move",
		bytes.NewReader([]byte(`{"direction":"up","speed":0.5}`)))
	svc.handlePTZRouter(rr, req)

	handlerBody := rr.Body.String()
	assert.Equal(t, http.StatusOK, rr.Code, "response body: %s", handlerBody)
	assert.True(t, onvifCalled || strings.Contains(handlerBody, `"status":"ok"`), "move should have been executed")
}

func TestHandleZoom_ValidRequest(t *testing.T) {
	server := onvifTestServer(t, map[string]string{
		"GetProfiles":     profilesSOAPResponse,
		"ContinuousMove":  `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><ContinuousMoveResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`,
	})
	defer server.Close()

	camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/zoom",
		bytes.NewReader([]byte(`{"zoom":0.5}`)))
	svc.handlePTZRouter(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleStop_ValidRequest(t *testing.T) {
	server := onvifTestServer(t, map[string]string{
		"GetProfiles": profilesSOAPResponse,
		"Stop":        `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><StopResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`,
	})
	defer server.Close()

	camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/stop", nil)
	svc.handlePTZRouter(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleListPresets(t *testing.T) {
	server := onvifTestServer(t, map[string]string{
		"GetProfiles": profilesSOAPResponse,
		"GetPresets": `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetPresetsResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"><Preset token="1"><Name>Entrance</Name></Preset><Preset token="2"><Name>Parking</Name></Preset></GetPresetsResponse></s:Body></s:Envelope>`,
	})
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
	assert.Equal(t, "Parking", resp.Presets[1].Name)
}

func TestHandleSetPreset_ValidRequest(t *testing.T) {
	server := onvifTestServer(t, map[string]string{
		"GetProfiles": profilesSOAPResponse,
		"SetPreset":   `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><SetPresetResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"><PresetToken>1</PresetToken></SetPresetResponse></s:Body></s:Envelope>`,
	})
	defer server.Close()

	camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/presets",
		bytes.NewReader([]byte(`{"preset_id":1,"preset_name":"Test"}`)))
	svc.handlePTZRouter(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandlePTZStatus(t *testing.T) {
	server := onvifTestServer(t, map[string]string{
		"GetProfiles": profilesSOAPResponse,
		"GetStatus":   `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetStatusResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"><PTZStatus><MoveStatus><PanTilt>IDLE</PanTilt></MoveStatus></PTZStatus></GetStatusResponse></s:Body></s:Envelope>`,
	})
	defer server.Close()

	camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cameras/cam-1/ptz/status", nil)
	svc.handlePTZRouter(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleGotoHome_ValidRequest(t *testing.T) {
	server := onvifTestServer(t, map[string]string{
		"GetProfiles":    profilesSOAPResponse,
		"GotoHomePosition": `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GotoHomePositionResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`,
	})
	defer server.Close()

	camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/home", nil)
	svc.handlePTZRouter(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleSetHome_ValidRequest(t *testing.T) {
	server := onvifTestServer(t, map[string]string{
		"GetProfiles":     profilesSOAPResponse,
		"SetHomePosition": `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><SetHomePositionResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`,
	})
	defer server.Close()

	camera := newONVIFCamera("cam-1", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cameras/cam-1/ptz/set-home", nil)
	svc.handlePTZRouter(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

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

func TestHikvisionCommand_Move(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "PUT", r.Method)
		assert.Contains(t, r.URL.Path, "/ISAPI/PTZCtrl/channels/1/continuous")
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "<Pan>50</Pan>")
		assert.Contains(t, string(body), "<Tilt>-50</Tilt>")
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

func TestOnvifCommand_Move(t *testing.T) {
	handlers := map[string]string{
		"GetProfiles":    profilesSOAPResponse,
		"ContinuousMove": `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><ContinuousMoveResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`,
	}
	server := onvifTestServer(t, handlers)
	defer server.Close()

	camera := newONVIFCamera("cam-onvif", "ONVIF Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-onvif": camera})

	err := svc.onvifCommand(server.URL, "move", "up", 0.5, camera)
	assert.NoError(t, err)
}

func TestOnvifCommand_Stop(t *testing.T) {
	handlers := map[string]string{
		"GetProfiles": profilesSOAPResponse,
		"Stop":        `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><StopResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`,
	}
	server := onvifTestServer(t, handlers)
	defer server.Close()

	camera := newONVIFCamera("cam-1", "Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	err := svc.onvifCommand(server.URL, "stop", "", 0, camera)
	assert.NoError(t, err)
}

func TestOnvifCommand_GotoPreset(t *testing.T) {
	handlers := map[string]string{
		"GetProfiles": profilesSOAPResponse,
		"GotoPreset":  `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GotoPresetResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`,
	}
	server := onvifTestServer(t, handlers)
	defer server.Close()

	camera := newONVIFCamera("cam-1", "Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	err := svc.onvifCommand(server.URL, "goto_preset", "1", 0, camera)
	assert.NoError(t, err)
}

func TestOnvifCommand_UnknownCommand(t *testing.T) {
	svc := newTestService(nil)
	err := svc.onvifCommand("http://localhost", "unknown", "", 0, &damv1.Camera{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown ONVIF command")
}

func TestOnvifCommand_SetPreset(t *testing.T) {
	handlers := map[string]string{
		"GetProfiles": profilesSOAPResponse,
		"SetPreset":   `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><SetPresetResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"><PresetToken>1</PresetToken></SetPresetResponse></s:Body></s:Envelope>`,
	}
	server := onvifTestServer(t, handlers)
	defer server.Close()

	camera := newONVIFCamera("cam-1", "Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	err := svc.onvifCommand(server.URL, "set_preset", "1", 0, camera)
	assert.NoError(t, err)
}

func TestOnvifListPresets(t *testing.T) {
	handlers := map[string]string{
		"GetProfiles": profilesSOAPResponse,
		"GetPresets": `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetPresetsResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"><Preset token="1"><Name>Entrance</Name></Preset><Preset token="2"><Name>Parking</Name></Preset></GetPresetsResponse></s:Body></s:Envelope>`,
	}
	server := onvifTestServer(t, handlers)
	defer server.Close()

	camera := newONVIFCamera("cam-1", "Cam", server.URL, portFromURL(t, server.URL))
	svc := newTestService(map[string]*damv1.Camera{"cam-1": camera})

	presets, err := svc.onvifListPresets(server.URL, camera)
	require.NoError(t, err)
	assert.Len(t, presets, 2)
	assert.Equal(t, "Entrance", presets[0].Name)
	assert.Equal(t, 1, presets[0].ID)
}

func TestCameraRouter_InvalidPath(t *testing.T) {
	svc := newTestService(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cameras", nil)
	svc.handleCameraRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCameraRouter_EmptyCameraID(t *testing.T) {
	svc := newTestService(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cameras//profiles", nil)
	svc.handleCameraRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCameraRouter_UnknownEndpoint(t *testing.T) {
	svc := newTestService(map[string]*damv1.Camera{
		"cam-1": {Id: "cam-1", ConnectionUrl: "http://localhost"},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cameras/cam-1/unknown-endpoint", nil)
	svc.handleCameraRouter(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleServiceDebug(t *testing.T) {
	svc := newTestService(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/diagnostics", nil)
	svc.handleServiceDebug(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var body map[string]interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Contains(t, body, "goroutines")
	assert.Contains(t, body, "memory")
}

func TestHandleServiceDebug_WrongMethod(t *testing.T) {
	svc := newTestService(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/diagnostics", nil)
	svc.handleServiceDebug(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestJSONError(t *testing.T) {
	rr := httptest.NewRecorder()
	jsonError(rr, "test error", http.StatusBadRequest)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "test error", body["error"])
}

func TestJSONOK(t *testing.T) {
	rr := httptest.NewRecorder()
	jsonOK(rr, map[string]string{"status": "ok"})
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	assert.Equal(t, "ok", body["status"])
}

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
