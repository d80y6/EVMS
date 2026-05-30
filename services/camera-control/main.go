package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/api/v1"
	"github.com/dam-vms/dam/pkg/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

type PTZConfig struct {
	Port           string
	CameraSvcAddr  string
	MetricsAddr    string
	RequestTimeout time.Duration
}

func DefaultPTZConfig() *PTZConfig {
	return &PTZConfig{
		Port:           common.GetEnv("CAMERA_CONTROL_PORT", ":8088"),
		CameraSvcAddr:  common.GetEnv("CAMERA_SERVICE_ADDR", "camera-mgmt:50051"),
		MetricsAddr:    common.GetEnv("METRICS_ADDR", ":2112"),
		RequestTimeout: 10 * time.Second,
	}
}

type PTZService struct {
	config    *PTZConfig
	logger    *slog.Logger
	cameraCC  *grpc.ClientConn
	cameraSvc damv1.CameraServiceClient
	httpCli   *http.Client
	server    *http.Server
}

func NewPTZService(config *PTZConfig, logger *slog.Logger) (*PTZService, error) {
	cameraCC, err := grpc.NewClient(config.CameraSvcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to camera service: %w", err)
	}
	cameraSvc := damv1.NewCameraServiceClient(cameraCC)

	return &PTZService{
		config:    config,
		logger:    logger,
		cameraCC:  cameraCC,
		cameraSvc: cameraSvc,
		httpCli:   &http.Client{Timeout: config.RequestTimeout},
	}, nil
}

func (s *PTZService) Close() error {
	if s.cameraCC != nil {
		return s.cameraCC.Close()
	}
	return nil
}

func (s *PTZService) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/cameras/", s.handlePTZRouter)
	mux.HandleFunc("/health", s.healthHandler)

	s.server = &http.Server{
		Addr:         s.config.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	s.logger.Info("Camera Control Service started", "address", s.config.Port)
	return s.server.ListenAndServe()
}

func (s *PTZService) Shutdown(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *PTZService) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *PTZService) handlePTZRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	s.logger.Debug("PTZ request", "path", path, "method", r.Method)

	parts := strings.Split(strings.TrimPrefix(path, "/cameras/"), "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] != "ptz" {
		jsonError(w, "invalid path: /cameras/{id}/ptz/{action}", http.StatusBadRequest)
		return
	}

	cameraID := parts[0]
	action := strings.Join(parts[2:], "/")

	ctx, cancel := context.WithTimeout(r.Context(), s.config.RequestTimeout)
	defer cancel()

	camera, err := s.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		s.logger.Error("Failed to get camera", "id", cameraID, "error", err)
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	switch action {
	case "move":
		s.handleMove(w, r, camera)
	case "zoom":
		s.handleZoom(w, r, camera)
	case "presets":
		if r.Method == http.MethodGet {
			s.handleListPresets(w, r, camera)
		} else if r.Method == http.MethodPost {
			s.handleSetPreset(w, r, camera)
		} else {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case "preset/goto":
		s.handleGotoPreset(w, r, camera)
	case "stop":
		s.handleStop(w, r, camera)
	default:
		jsonError(w, fmt.Sprintf("unknown PTZ action: %s", action), http.StatusBadRequest)
	}
}

type moveRequest struct {
	Direction string  `json:"direction"`
	Speed     float64 `json:"speed,omitempty"`
}

func (s *PTZService) handleMove(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	speed := req.Speed
	if speed <= 0 {
		speed = 0.5
	}

	if err := s.sendPTZCommand(camera, "move", req.Direction, speed); err != nil {
		s.logger.Error("PTZ move failed", "camera", camera.Id, "direction", req.Direction, "error", err)
		jsonError(w, fmt.Sprintf("PTZ command failed: %v", err), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

type zoomRequest struct {
	Zoom float64 `json:"zoom"`
}

func (s *PTZService) handleZoom(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req zoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.sendPTZCommand(camera, "zoom", "", req.Zoom); err != nil {
		s.logger.Error("PTZ zoom failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("PTZ command failed: %v", err), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleStop(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.sendPTZCommand(camera, "stop", "", 0); err != nil {
		s.logger.Error("PTZ stop failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("PTZ command failed: %v", err), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

type setPresetRequest struct {
	PresetID   int    `json:"preset_id"`
	PresetName string `json:"preset_name,omitempty"`
}

type presetResponse struct {
	Presets []presetItem `json:"presets"`
}

type presetItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (s *PTZService) handleListPresets(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	presets, err := s.listPTZPresets(camera)
	if err != nil {
		s.logger.Error("Failed to list presets", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to list presets: %v", err), http.StatusInternalServerError)
		return
	}

	jsonOK(w, presetResponse{Presets: presets})
}

func (s *PTZService) handleSetPreset(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	var req setPresetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.sendPTZCommand(camera, "set_preset", strconv.Itoa(req.PresetID), 0); err != nil {
		s.logger.Error("Failed to set preset", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to set preset: %v", err), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

type gotoPresetRequest struct {
	PresetID int `json:"preset_id"`
}

func (s *PTZService) handleGotoPreset(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req gotoPresetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.sendPTZCommand(camera, "goto_preset", strconv.Itoa(req.PresetID), 0); err != nil {
		s.logger.Error("Failed to goto preset", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to goto preset: %v", err), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) sendPTZCommand(camera *damv1.Camera, command, param string, speed float64) error {
	protocol := resolvePTZProtocol(camera)
	baseURL := camera.ConnectionUrl

	s.logger.Info("Sending PTZ command", "camera", camera.Id, "protocol", protocol, "command", command)

	switch protocol {
	case "onvif":
		return s.onvifCommand(baseURL, command, param, speed)
	case "vapix":
		return s.vapixCommand(baseURL, command, param, speed)
	case "hikvision":
		return s.hikvisionCommand(baseURL, command, param, speed)
	default:
		return fmt.Errorf("unsupported PTZ protocol: %s", protocol)
	}
}

func (s *PTZService) listPTZPresets(camera *damv1.Camera) ([]presetItem, error) {
	protocol := resolvePTZProtocol(camera)
	baseURL := camera.ConnectionUrl

	switch protocol {
	case "onvif":
		return s.onvifListPresets(baseURL)
	case "vapix":
		return s.vapixListPresets(baseURL)
	case "hikvision":
		return s.hikvisionListPresets(baseURL)
	default:
		return nil, fmt.Errorf("unsupported PTZ protocol: %s", protocol)
	}
}

func resolvePTZProtocol(camera *damv1.Camera) string {
	return "onvif"
}

func (s *PTZService) onvifCommand(baseURL, command, param string, speed float64) error {
	var soapBody string
	switch command {
	case "move":
		var x, y float64
		switch param {
		case "up":
			y = speed
		case "down":
			y = -speed
		case "left":
			x = -speed
		case "right":
			x = speed
		case "up-left":
			x = -speed
			y = speed
		case "up-right":
			x = speed
			y = speed
		case "down-left":
			x = -speed
			y = -speed
		case "down-right":
			x = speed
			y = -speed
		default:
			y = speed
		}
		soapBody = fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
  <soap:Body>
    <ptz:ContinuousMove>
      <ptz:ProfileToken>profile_1</ptz:ProfileToken>
      <ptz:Velocity>
        <ptz:PanTilt x="%f" y="%f" space="http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocitySpace"/>
        <ptz:Zoom x="0.0" space="http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocitySpace"/>
      </ptz:Velocity>
    </ptz:ContinuousMove>
  </soap:Body>
</soap:Envelope>`, x, y)
	case "zoom":
		soapBody = fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
  <soap:Body>
    <ptz:ContinuousMove>
      <ptz:ProfileToken>profile_1</ptz:ProfileToken>
      <ptz:Velocity>
        <ptz:PanTilt x="0.0" y="0.0" space="http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocitySpace"/>
        <ptz:Zoom x="%f" space="http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocitySpace"/>
      </ptz:Velocity>
    </ptz:ContinuousMove>
  </soap:Body>
</soap:Envelope>`, speed)
	case "stop":
		soapBody = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
  <soap:Body>
    <ptz:Stop>
      <ptz:ProfileToken>profile_1</ptz:ProfileToken>
      <ptz:PanTilt>true</ptz:PanTilt>
      <ptz:Zoom>true</ptz:Zoom>
    </ptz:Stop>
  </soap:Body>
</soap:Envelope>`
	case "goto_preset":
		soapBody = fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
  <soap:Body>
    <ptz:GotoPreset>
      <ptz:ProfileToken>profile_1</ptz:ProfileToken>
      <ptz:PresetToken>%s</ptz:PresetToken>
    </ptz:GotoPreset>
  </soap:Body>
</soap:Envelope>`, param)
	case "set_preset":
		soapBody = fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
  <soap:Body>
    <ptz:SetPreset>
      <ptz:ProfileToken>profile_1</ptz:ProfileToken>
      <ptz:PresetName>Preset %s</ptz:PresetName>
    </ptz:SetPreset>
  </soap:Body>
</soap:Envelope>`, param)
	default:
		return fmt.Errorf("unknown ONVIF command: %s", command)
	}

	onvifURL := strings.TrimRight(baseURL, "/") + "/onvif/ptz_service"
	req, err := http.NewRequest("POST", onvifURL, bytes.NewBufferString(soapBody))
	if err != nil {
		return fmt.Errorf("failed to create ONVIF request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("ONVIF request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ONVIF request returned status %d", resp.StatusCode)
	}

	return nil
}

func (s *PTZService) onvifListPresets(baseURL string) ([]presetItem, error) {
	soapBody := `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
  <soap:Body>
    <ptz:GetPresets>
      <ptz:ProfileToken>profile_1</ptz:ProfileToken>
    </ptz:GetPresets>
  </soap:Body>
</soap:Envelope>`

	onvifURL := strings.TrimRight(baseURL, "/") + "/onvif/ptz_service"
	req, err := http.NewRequest("POST", onvifURL, bytes.NewBufferString(soapBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create ONVIF request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ONVIF request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ONVIF request returned status %d", resp.StatusCode)
	}

	return []presetItem{}, nil
}

func (s *PTZService) vapixCommand(baseURL, command, param string, speed float64) error {
	cgiURL := strings.TrimRight(baseURL, "/") + "/axis-cgi/com/ptz.cgi"

	var params string
	switch command {
	case "move":
		var direction string
		switch param {
		case "up":
			direction = "up"
		case "down":
			direction = "down"
		case "left":
			direction = "left"
		case "right":
			direction = "right"
		case "up-left":
			direction = "upleft"
		case "up-right":
			direction = "upright"
		case "down-left":
			direction = "downleft"
		case "down-right":
			direction = "downright"
		default:
			direction = "up"
		}
		speedVal := int(speed * 100)
		params = fmt.Sprintf("move=%s&speed=%d", direction, speedVal)
	case "zoom":
		var dir string
		if speed > 0 {
			dir = "wide"
		} else {
			dir = "tele"
		}
		params = fmt.Sprintf("move=%s", dir)
	case "stop":
		params = "continuouszoommove=0&continuouspantiltmove=0"
	case "goto_preset":
		params = fmt.Sprintf("gotoserverpresetnumber=%s", param)
	case "set_preset":
		params = fmt.Sprintf("setpresetname=%s", param)
	default:
		return fmt.Errorf("unknown VAPIX command: %s", command)
	}

	req, err := http.NewRequest("GET", cgiURL+"?"+params, nil)
	if err != nil {
		return fmt.Errorf("failed to create VAPIX request: %w", err)
	}

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("VAPIX request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("VAPIX request returned status %d", resp.StatusCode)
	}

	return nil
}

func (s *PTZService) vapixListPresets(baseURL string) ([]presetItem, error) {
	cgiURL := strings.TrimRight(baseURL, "/") + "/axis-cgi/com/ptz.cgi?gotoserverpresetname"
	req, err := http.NewRequest("GET", cgiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create VAPIX request: %w", err)
	}

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("VAPIX request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("VAPIX request returned status %d", resp.StatusCode)
	}

	return []presetItem{}, nil
}

func (s *PTZService) hikvisionCommand(baseURL, command, param string, speed float64) error {
	isapiURL := strings.TrimRight(baseURL, "/") + "/ISAPI/PTZCtrl/channels/1"

	switch command {
	case "move":
		var x, y int
		switch param {
		case "up":
			y = int(speed * 100)
		case "down":
			y = -int(speed * 100)
		case "left":
			x = -int(speed * 100)
		case "right":
			x = int(speed * 100)
		case "up-left":
			x = -int(speed * 100)
			y = int(speed * 100)
		case "up-right":
			x = int(speed * 100)
			y = int(speed * 100)
		case "down-left":
			x = -int(speed * 100)
			y = -int(speed * 100)
		case "down-right":
			x = int(speed * 100)
			y = -int(speed * 100)
		default:
			y = int(speed * 100)
		}
		xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<PTZData>
  <Pan>%d</Pan>
  <Tilt>%d</Tilt>
  <Zoom>0</Zoom>
</PTZData>`, x, y)
		req, err := http.NewRequest("PUT", isapiURL+"/continuous", bytes.NewBufferString(xmlBody))
		if err != nil {
			return fmt.Errorf("failed to create Hikvision request: %w", err)
		}
		req.Header.Set("Content-Type", "application/xml")
		resp, err := s.httpCli.Do(req)
		if err != nil {
			return fmt.Errorf("Hikvision request failed: %w", err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("Hikvision request returned status %d", resp.StatusCode)
		}
		return nil
	case "zoom":
		zoomVal := int(speed * 100)
		xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<PTZData>
  <Pan>0</Pan>
  <Tilt>0</Tilt>
  <Zoom>%d</Zoom>
</PTZData>`, zoomVal)
		req, err := http.NewRequest("PUT", isapiURL+"/continuous", bytes.NewBufferString(xmlBody))
		if err != nil {
			return fmt.Errorf("failed to create Hikvision request: %w", err)
		}
		req.Header.Set("Content-Type", "application/xml")
		resp, err := s.httpCli.Do(req)
		if err != nil {
			return fmt.Errorf("Hikvision request failed: %w", err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("Hikvision request returned status %d", resp.StatusCode)
		}
		return nil
	case "stop":
		xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<PTZData>
  <Pan>0</Pan>
  <Tilt>0</Tilt>
  <Zoom>0</Zoom>
</PTZData>`
		req, err := http.NewRequest("PUT", isapiURL+"/continuous", bytes.NewBufferString(xmlBody))
		if err != nil {
			return fmt.Errorf("failed to create Hikvision request: %w", err)
		}
		req.Header.Set("Content-Type", "application/xml")
		resp, err := s.httpCli.Do(req)
		if err != nil {
			return fmt.Errorf("Hikvision request failed: %w", err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("Hikvision request returned status %d", resp.StatusCode)
		}
		return nil
	case "goto_preset":
		xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<PTZData>
  <AbsoluteHigh>
    <presetID>%s</presetID>
  </AbsoluteHigh>
</PTZData>`, param)
		req, err := http.NewRequest("PUT", isapiURL+"/presets/"+param+"/goto", bytes.NewBufferString(xmlBody))
		if err != nil {
			return fmt.Errorf("failed to create Hikvision request: %w", err)
		}
		req.Header.Set("Content-Type", "application/xml")
		resp, err := s.httpCli.Do(req)
		if err != nil {
			return fmt.Errorf("Hikvision request failed: %w", err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("Hikvision request returned status %d", resp.StatusCode)
		}
		return nil
	case "set_preset":
		xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<PTZData>
  <AbsoluteHigh>
    <presetID>%s</presetID>
    <presetName>Preset %s</presetName>
  </AbsoluteHigh>
</PTZData>`, param, param)
		req, err := http.NewRequest("PUT", isapiURL+"/presets/"+param, bytes.NewBufferString(xmlBody))
		if err != nil {
			return fmt.Errorf("failed to create Hikvision request: %w", err)
		}
		req.Header.Set("Content-Type", "application/xml")
		resp, err := s.httpCli.Do(req)
		if err != nil {
			return fmt.Errorf("Hikvision request failed: %w", err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("Hikvision request returned status %d", resp.StatusCode)
		}
		return nil
	default:
		return fmt.Errorf("unknown Hikvision command: %s", command)
	}
}

func (s *PTZService) hikvisionListPresets(baseURL string) ([]presetItem, error) {
	isapiURL := strings.TrimRight(baseURL, "/") + "/ISAPI/PTZCtrl/channels/1/presets"
	req, err := http.NewRequest("GET", isapiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Hikvision request: %w", err)
	}

	resp, err := s.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Hikvision request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Hikvision request returned status %d", resp.StatusCode)
	}

	return []presetItem{}, nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultPTZConfig()
	common.StartMetricsServer(config.MetricsAddr)

	service, err := NewPTZService(config, logger)
	if err != nil {
		logger.Error("Failed to initialize PTZ service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	go func() {
		if err := service.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("Camera Control service failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down Camera Control Service...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := service.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}
}
