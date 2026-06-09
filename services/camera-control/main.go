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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dam-vms/dam/api/v1"
	"github.com/dam-vms/dam/pkg/common"
	"github.com/dam-vms/dam/pkg/onvif"
	"google.golang.org/grpc"
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
	config        *PTZConfig
	logger        *slog.Logger
	cameraCC      *grpc.ClientConn
	cameraSvc     damv1.CameraServiceClient
	httpCli       *http.Client
	server        *http.Server
	healthHandler *common.HealthHandler
}

func NewPTZService(config *PTZConfig, logger *slog.Logger) (*PTZService, error) {
	creds, err := common.GRPCClientTLSCredentials("camera-mgmt")
	if err != nil {
		return nil, fmt.Errorf("failed to configure gRPC credentials: %w", err)
	}
	cameraCC, err := grpc.NewClient(config.CameraSvcAddr,
		grpc.WithTransportCredentials(creds))
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
	mux.HandleFunc("/cameras/", s.handleRouter)
	mux.HandleFunc("/diagnostics", s.handleServiceDebug)
	handler := common.NewHealthHandler()
	s.healthHandler = handler
	mux.HandleFunc("/health", handler.Liveness)
	mux.HandleFunc("/ready", handler.Readiness)

	s.server = &http.Server{
		Addr:         s.config.Port,
		Handler:      common.RecoveryMiddleware(mux),
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

	switch {
	case action == "move":
		s.handleMove(w, r, camera)
	case action == "zoom":
		s.handleZoom(w, r, camera)
	case action == "presets":
		if r.Method == http.MethodGet {
			s.handleListPresets(w, r, camera)
		} else if r.Method == http.MethodPost {
			s.handleSetPreset(w, r, camera)
		} else {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case strings.HasPrefix(action, "presets/") && strings.HasSuffix(action, "/goto"):
		s.handleGotoPreset(w, r, camera)
	case strings.HasPrefix(action, "presets/") && r.Method == http.MethodDelete:
		s.handleRemovePreset(w, r, camera)
	case action == "preset/goto":
		s.handleGotoPreset(w, r, camera)
	case action == "stop":
		s.handleStop(w, r, camera)
	case action == "home" && r.Method == http.MethodPost:
		s.handleGotoHome(w, r, camera)
	case action == "set-home" && r.Method == http.MethodPost:
		s.handleSetHome(w, r, camera)
	case action == "absolute-move" && r.Method == http.MethodPost:
		s.handleAbsoluteMove(w, r, camera)
	case action == "relative-move" && r.Method == http.MethodPost:
		s.handleRelativeMove(w, r, camera)
	case action == "status" && r.Method == http.MethodGet:
		s.handlePTZStatus(w, r, camera)
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

func (s *PTZService) handleIO(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cameraID := extractParam(r.URL.Path, "/cameras/")
	cameraID = strings.TrimSuffix(cameraID, "/io")

	ctx, cancel := context.WithTimeout(r.Context(), s.config.RequestTimeout)
	defer cancel()

	camera, err := s.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	var req struct {
		RelayID string `json:"relay_id"`
		State   string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	if camera.PtzProtocol != "onvif" {
		jsonError(w, "ONVIF required for IO control", http.StatusBadRequest)
		return
	}

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

	ioURL := strings.TrimRight(camera.ConnectionUrl, "/") + "/onvif/deviceio_service"
	resp, err := http.Post(ioURL, "application/soap+xml", strings.NewReader(soapBody))
	if err != nil {
		jsonError(w, "IO control failed", http.StatusInternalServerError)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	jsonOK(w, map[string]string{"status": "relay " + req.State})
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

	presetToken := ""

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/cameras/"), "/")
	if len(parts) >= 5 && parts[2] == "presets" && parts[len(parts)-1] == "goto" {
		presetToken = parts[len(parts)-2]
	}

	if presetToken == "" {
		var req gotoPresetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		presetToken = strconv.Itoa(req.PresetID)
	}

	if err := s.sendPTZCommand(camera, "goto_preset", presetToken, 0); err != nil {
		s.logger.Error("Failed to goto preset", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to goto preset: %v", err), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleRemovePreset(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/cameras/"), "/")
	if len(parts) < 5 {
		jsonError(w, "preset token required", http.StatusBadRequest)
		return
	}
	presetToken := parts[3]
	if err := s.sendPTZCommand(camera, "remove_preset", presetToken, 0); err != nil {
		s.logger.Error("Failed to remove preset", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to remove preset: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleGotoHome(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if err := s.sendPTZCommand(camera, "goto_home", "", 0); err != nil {
		s.logger.Error("Failed to goto home", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to goto home: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleSetHome(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if err := s.sendPTZCommand(camera, "set_home", "", 0); err != nil {
		s.logger.Error("Failed to set home", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to set home: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

type absoluteMoveRequest struct {
	Pan float64 `json:"pan"`
	Tilt float64 `json:"tilt"`
	Zoom float64 `json:"zoom,omitempty"`
	Speed float64 `json:"speed,omitempty"`
}

func (s *PTZService) handleAbsoluteMove(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	var req absoluteMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	ptzURL := onvif.BuildPTZURL(camera.ConnectionUrl)
	profileToken := s.getONVIFProfileToken(r.Context(), client, camera.ConnectionUrl)

	position := &onvif.PTZPosition{
		PanTilt: &onvif.Vector2D{X: req.Pan, Y: req.Tilt},
	}
	if req.Zoom != 0 {
		position.Zoom = &onvif.Vector1D{X: req.Zoom}
	}

	var speed *onvif.PTZSpeed
	if req.Speed > 0 {
		speed = &onvif.PTZSpeed{
			PanTilt: &onvif.Vector2D{X: req.Speed, Y: req.Speed},
		}
	}

	if err := onvif.AbsoluteMove(r.Context(), client, ptzURL, profileToken, position, speed); err != nil {
		s.logger.Error("AbsoluteMove failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("absolute move failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

type relativeMoveRequest struct {
	Pan   float64 `json:"pan"`
	Tilt  float64 `json:"tilt"`
	Zoom  float64 `json:"zoom,omitempty"`
	Speed float64 `json:"speed,omitempty"`
}

func (s *PTZService) handleRelativeMove(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	var req relativeMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	ptzURL := onvif.BuildPTZURL(camera.ConnectionUrl)
	profileToken := s.getONVIFProfileToken(r.Context(), client, camera.ConnectionUrl)

	translation := &onvif.Vector2D{X: req.Pan, Y: req.Tilt}
	var zoom *onvif.Vector1D
	if req.Zoom != 0 {
		zoom = &onvif.Vector1D{X: req.Zoom}
	}

	var speed *onvif.PTZSpeed
	if req.Speed > 0 {
		speed = &onvif.PTZSpeed{
			PanTilt: &onvif.Vector2D{X: req.Speed, Y: req.Speed},
		}
	}

	if err := onvif.RelativeMove(r.Context(), client, ptzURL, profileToken, translation, zoom, speed); err != nil {
		s.logger.Error("RelativeMove failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("relative move failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handlePTZStatus(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if camera.PtzProtocol != "" && camera.PtzProtocol != "none" && camera.PtzProtocol != "onvif" {
		jsonError(w, "PTZ status only supported for ONVIF", http.StatusBadRequest)
		return
	}
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	ptzURL := onvif.BuildPTZURL(camera.ConnectionUrl)
	profileToken := s.getONVIFProfileToken(r.Context(), client, camera.ConnectionUrl)
	status, err := onvif.GetPTZStatus(r.Context(), client, ptzURL, profileToken)
	if err != nil {
		s.logger.Error("GetPTZStatus failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("PTZ status failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, status)
}

func (s *PTZService) sendPTZCommand(camera *damv1.Camera, command, param string, speed float64) error {
	protocol := resolvePTZProtocol(camera)
	baseURL := camera.ConnectionUrl

	s.logger.Info("Sending PTZ command", "camera", camera.Id, "protocol", protocol, "command", command)

	switch protocol {
	case "onvif":
		return s.onvifCommand(baseURL, command, param, speed, camera)
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
		return s.onvifListPresets(baseURL, camera)
	case "vapix":
		return s.vapixListPresets(baseURL)
	case "hikvision":
		return s.hikvisionListPresets(baseURL)
	default:
		return nil, fmt.Errorf("unsupported PTZ protocol: %s", protocol)
	}
}

func resolvePTZProtocol(camera *damv1.Camera) string {
	if camera.PtzProtocol != "" && camera.PtzProtocol != "none" {
		return camera.PtzProtocol
	}
	return "onvif"
}

func (s *PTZService) getONVIFClient(baseURL string, camera *damv1.Camera) *onvif.SOAPClient {
	creds := &onvif.Credentials{}
	if camera.OnvifUsername != "" {
		creds.Username = camera.OnvifUsername
		creds.Password = camera.OnvifPassword
	}
	return onvif.NewSOAPClient(s.config.RequestTimeout, creds)
}

func (s *PTZService) getONVIFProfileToken(ctx context.Context, client *onvif.SOAPClient, baseURL string) string {
	mediaURL := onvif.BuildMediaURL(baseURL)
	profiles, err := onvif.GetProfiles(ctx, client, mediaURL)
	if err != nil || len(profiles) == 0 {
		return "profile_1"
	}
	mainProfile := onvif.FindMainProfile(profiles)
	if mainProfile != nil {
		return mainProfile.Token
	}
	return profiles[0].Token
}

func (s *PTZService) onvifCommand(baseURL, command, param string, speed float64, camera *damv1.Camera) error {
	client := s.getONVIFClient(baseURL, camera)
	ptzURL := onvif.BuildPTZURL(baseURL)
	profileToken := s.getONVIFProfileToken(context.Background(), client, baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout-1*time.Second)
	defer cancel()

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
		return onvif.ContinuousMove(ctx, client, ptzURL, profileToken,
			&onvif.Vector2D{X: x, Y: y}, nil)
	case "zoom":
		return onvif.ContinuousMove(ctx, client, ptzURL, profileToken,
			nil, &onvif.Vector1D{X: speed})
	case "stop":
		return onvif.Stop(ctx, client, ptzURL, profileToken, true, true)
	case "goto_preset":
		return onvif.GotoPreset(ctx, client, ptzURL, profileToken, param, nil)
	case "set_preset":
		_, err := onvif.SetPreset(ctx, client, ptzURL, profileToken, param)
		return err
	case "goto_home":
		return onvif.GotoHomePosition(ctx, client, ptzURL, profileToken, nil)
	case "set_home":
		return onvif.SetHomePosition(ctx, client, ptzURL, profileToken)
	case "remove_preset":
		return onvif.RemovePreset(ctx, client, ptzURL, profileToken, param)
	default:
		return fmt.Errorf("unknown ONVIF command: %s", command)
	}
}

func (s *PTZService) onvifListPresets(baseURL string, camera *damv1.Camera) ([]presetItem, error) {
	client := s.getONVIFClient(baseURL, camera)
	ptzURL := onvif.BuildPTZURL(baseURL)
	profileToken := s.getONVIFProfileToken(context.Background(), client, baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout-1*time.Second)
	defer cancel()

	presets, err := onvif.GetPresets(ctx, client, ptzURL, profileToken)
	if err != nil {
		return nil, fmt.Errorf("GetPresets failed: %w", err)
	}

	items := make([]presetItem, 0, len(presets))
	for _, p := range presets {
		id, _ := strconv.Atoi(p.Token)
		if id == 0 {
			id = len(items) + 1
		}
		items = append(items, presetItem{ID: id, Name: p.Name})
	}

	return items, nil
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

// ========== General Router ==========

func (s *PTZService) handleRouter(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/io") {
		s.handleIO(w, r)
		return
	}
	s.handleCameraRouter(w, r)
}

func (s *PTZService) handleCameraRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/cameras/"), "/")

	if len(parts) < 2 || parts[0] == "" {
		jsonError(w, "invalid path", http.StatusBadRequest)
		return
	}

	if parts[1] == "ptz" {
		s.handlePTZRouter(w, r)
		return
	}

	cameraID := parts[0]

	ctx, cancel := context.WithTimeout(r.Context(), s.config.RequestTimeout)
	defer cancel()

	camera, err := s.cameraSvc.GetCamera(ctx, &damv1.GetCameraRequest{Id: cameraID})
	if err != nil {
		s.logger.Error("Failed to get camera", "id", cameraID, "error", err)
		jsonError(w, "camera not found", http.StatusNotFound)
		return
	}

	switch parts[1] {
	case "profiles":
		s.handleGetProfiles(w, r, camera)
	case "snapshot":
		s.handleGetSnapshotURI(w, r, camera)
	case "stream-uri":
		s.handleGetStreamURI(w, r, camera)
	case "video-sources":
		s.handleGetVideoSources(w, r, camera)
	case "audio-sources":
		s.handleGetAudioSources(w, r, camera)
	case "imaging":
		s.handleImagingRouter(w, r, camera, parts[2:])
	case "device":
		s.handleDeviceRouter(w, r, camera, parts[2:])
	case "network":
		s.handleNetworkRouter(w, r, camera, parts[2:])
	case "recording":
		s.handleRecordingRouter(w, r, camera, parts[2:])
	case "analytics":
		s.handleAnalyticsRouter(w, r, camera, parts[2:])
	case "diagnostics":
		s.handleDeviceDiagnostics(w, r, camera)
	default:
		jsonError(w, fmt.Sprintf("unknown endpoint: %s", parts[1]), http.StatusBadRequest)
	}
}

// ========== Media Handlers ==========

func (s *PTZService) handleGetProfiles(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	mediaURL := onvif.BuildMediaURL(camera.ConnectionUrl)
	profiles, err := onvif.GetProfiles(r.Context(), client, mediaURL)
	if err != nil {
		s.logger.Error("GetProfiles failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to get profiles: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"profiles": profiles})
}

func (s *PTZService) handleGetSnapshotURI(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	mediaURL := onvif.BuildMediaURL(camera.ConnectionUrl)
	profileToken := r.URL.Query().Get("profile")
	if profileToken == "" {
		profileToken = s.getONVIFProfileToken(r.Context(), client, camera.ConnectionUrl)
	}
	uri, err := onvif.GetSnapshotURI(r.Context(), client, mediaURL, profileToken)
	if err != nil {
		s.logger.Error("GetSnapshotURI failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to get snapshot URI: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"snapshot_uri": uri})
}

func (s *PTZService) handleGetStreamURI(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	mediaURL := onvif.BuildMediaURL(camera.ConnectionUrl)
	profileToken := r.URL.Query().Get("profile")
	if profileToken == "" {
		profileToken = s.getONVIFProfileToken(r.Context(), client, camera.ConnectionUrl)
	}
	protocol := r.URL.Query().Get("protocol")
	uri, err := onvif.GetStreamURI(r.Context(), client, mediaURL, profileToken, protocol)
	if err != nil {
		s.logger.Error("GetStreamURI failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to get stream URI: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"uri": uri})
}

func (s *PTZService) handleGetVideoSources(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	mediaURL := onvif.BuildMediaURL(camera.ConnectionUrl)
	sources, err := onvif.GetVideoSources(r.Context(), client, mediaURL)
	if err != nil {
		s.logger.Error("GetVideoSources failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to get video sources: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"video_sources": sources})
}

func (s *PTZService) handleGetAudioSources(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	mediaURL := onvif.BuildMediaURL(camera.ConnectionUrl)
	sources, err := onvif.GetAudioSources(r.Context(), client, mediaURL)
	if err != nil {
		s.logger.Error("GetAudioSources failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to get audio sources: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"audio_sources": sources})
}

// ========== Imaging Handlers ==========

func (s *PTZService) handleImagingRouter(w http.ResponseWriter, r *http.Request, camera *damv1.Camera, parts []string) {
	if len(parts) == 0 {
		jsonError(w, "invalid imaging path", http.StatusBadRequest)
		return
	}

	switch {
	case len(parts) == 1 && parts[0] == "settings" && r.Method == http.MethodGet:
		s.handleGetImagingSettings(w, r, camera)
	case len(parts) == 1 && parts[0] == "settings" && (r.Method == http.MethodPut || r.Method == http.MethodPost):
		s.handleSetImagingSettings(w, r, camera)
	case len(parts) == 2 && parts[0] == "focus" && parts[1] == "move":
		s.handleMoveFocus(w, r, camera)
	case len(parts) == 2 && parts[0] == "focus" && parts[1] == "stop":
		s.handleStopFocus(w, r, camera)
	case len(parts) == 1 && parts[0] == "status" && r.Method == http.MethodGet:
		s.handleGetImagingStatus(w, r, camera)
	default:
		jsonError(w, "unknown imaging action", http.StatusBadRequest)
	}
}

func (s *PTZService) handleGetImagingSettings(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	imagingURL := onvif.BuildImagingURL(camera.ConnectionUrl)

	videoSourceToken := r.URL.Query().Get("profile")
	if videoSourceToken == "" {
		videoSourceToken = s.getVideoSourceToken(r.Context(), client, camera.ConnectionUrl)
	}

	settings, err := onvif.GetImagingSettings(r.Context(), client, imagingURL, videoSourceToken)
	if err != nil {
		s.logger.Error("GetImagingSettings failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("failed to get imaging settings: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"settings": settings})
}

type setImagingSettingsReq struct {
	ProfileToken string                 `json:"profile_token"`
	Settings     map[string]interface{} `json:"settings"`
}

func (s *PTZService) handleSetImagingSettings(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	var req setImagingSettingsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	imagingURL := onvif.BuildImagingURL(camera.ConnectionUrl)

	videoSourceToken := req.ProfileToken
	if videoSourceToken == "" {
		videoSourceToken = s.getVideoSourceToken(r.Context(), client, camera.ConnectionUrl)
	}

	var settings onvif.ImagingSettings
	if v, ok := req.Settings["brightness"].(float64); ok {
		settings.Brightness = &v
	}
	if v, ok := req.Settings["color_saturation"].(float64); ok {
		settings.ColorSaturation = &v
	}
	if v, ok := req.Settings["contrast"].(float64); ok {
		settings.Contrast = &v
	}
	if v, ok := req.Settings["sharpness"].(float64); ok {
		settings.Sharpness = &v
	}
	if v, ok := req.Settings["ir_cut_filter"].(string); ok {
		settings.IrCutFilter = v
	}

	if err := onvif.SetImagingSettings(r.Context(), client, imagingURL, videoSourceToken, &settings); err != nil {
		s.logger.Error("SetImagingSettings failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("set imaging settings failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleMoveFocus(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Speed float64 `json:"speed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	imagingURL := onvif.BuildImagingURL(camera.ConnectionUrl)
	videoSourceToken := s.getVideoSourceToken(r.Context(), client, camera.ConnectionUrl)

	if err := onvif.MoveFocus(r.Context(), client, imagingURL, videoSourceToken, req.Speed); err != nil {
		s.logger.Error("MoveFocus failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("move focus failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleStopFocus(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	imagingURL := onvif.BuildImagingURL(camera.ConnectionUrl)
	videoSourceToken := s.getVideoSourceToken(r.Context(), client, camera.ConnectionUrl)

	if err := onvif.StopFocus(r.Context(), client, imagingURL, videoSourceToken); err != nil {
		s.logger.Error("StopFocus failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("stop focus failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleGetImagingStatus(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	imagingURL := onvif.BuildImagingURL(camera.ConnectionUrl)

	videoSourceToken := r.URL.Query().Get("profile")
	if videoSourceToken == "" {
		videoSourceToken = s.getVideoSourceToken(r.Context(), client, camera.ConnectionUrl)
	}

	status, err := onvif.GetImagingStatus(r.Context(), client, imagingURL, videoSourceToken)
	if err != nil {
		s.logger.Error("GetImagingStatus failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get imaging status failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"status": status})
}

// ========== Device Handlers ==========

func (s *PTZService) handleDeviceRouter(w http.ResponseWriter, r *http.Request, camera *damv1.Camera, parts []string) {
	if len(parts) == 0 {
		jsonError(w, "invalid device path", http.StatusBadRequest)
		return
	}

	switch {
	case len(parts) == 1 && parts[0] == "info" && r.Method == http.MethodGet:
		s.handleGetDeviceInfo(w, r, camera)
	case len(parts) == 1 && parts[0] == "capabilities" && r.Method == http.MethodGet:
		s.handleGetCapabilities(w, r, camera)
	case len(parts) == 1 && parts[0] == "services" && r.Method == http.MethodGet:
		s.handleGetServices(w, r, camera)
	case len(parts) == 1 && parts[0] == "date" && r.Method == http.MethodGet:
		s.handleGetSystemDate(w, r, camera)
	case len(parts) == 1 && parts[0] == "reboot" && r.Method == http.MethodPost:
		s.handleRebootDevice(w, r, camera)
	default:
		jsonError(w, "unknown device action", http.StatusBadRequest)
	}
}

func (s *PTZService) handleGetDeviceInfo(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	info, err := onvif.GetDeviceInformation(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetDeviceInformation failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get device info failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, info)
}

func (s *PTZService) handleGetCapabilities(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	caps, err := onvif.GetCapabilities(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetCapabilities failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get capabilities failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, caps)
}

func (s *PTZService) handleGetServices(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	services, err := onvif.GetServices(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetServices failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get services failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"services": services})
}

func (s *PTZService) handleGetSystemDate(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	dateTime, err := onvif.GetSystemDateAndTime(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetSystemDateAndTime failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get system date failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, dateTime)
}

func (s *PTZService) handleRebootDevice(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	if err := onvif.Reboot(r.Context(), client, camera.ConnectionUrl); err != nil {
		s.logger.Error("Reboot failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("reboot failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// ========== Network Handlers ==========

func (s *PTZService) handleNetworkRouter(w http.ResponseWriter, r *http.Request, camera *damv1.Camera, parts []string) {
	if len(parts) == 0 {
		jsonError(w, "invalid network path", http.StatusBadRequest)
		return
	}

	switch {
	case len(parts) == 1 && parts[0] == "interfaces" && r.Method == http.MethodGet:
		s.handleGetNetworkInterfaces(w, r, camera)
	case len(parts) == 1 && parts[0] == "dns" && r.Method == http.MethodGet:
		s.handleGetDNS(w, r, camera)
	case len(parts) == 1 && parts[0] == "dns" && (r.Method == http.MethodPut || r.Method == http.MethodPost):
		s.handleSetDNS(w, r, camera)
	case len(parts) == 1 && parts[0] == "ntp" && r.Method == http.MethodGet:
		s.handleGetNTP(w, r, camera)
	case len(parts) == 1 && parts[0] == "hostname" && r.Method == http.MethodGet:
		s.handleGetHostname(w, r, camera)
	case len(parts) == 1 && parts[0] == "hostname" && (r.Method == http.MethodPut || r.Method == http.MethodPost):
		s.handleSetHostname(w, r, camera)
	default:
		jsonError(w, "unknown network action", http.StatusBadRequest)
	}
}

func (s *PTZService) handleGetNetworkInterfaces(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	ifaces, err := onvif.GetNetworkInterfaces(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetNetworkInterfaces failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get network interfaces failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"interfaces": ifaces})
}

func (s *PTZService) handleGetDNS(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	dns, err := onvif.GetDNS(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetDNS failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get DNS failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, dns)
}

type setDNSReq struct {
	FromDHCP   bool     `json:"from_dhcp"`
	DNSServers []string `json:"dns_servers"`
}

func (s *PTZService) handleSetDNS(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	var req setDNSReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	if err := onvif.SetDNS(r.Context(), client, camera.ConnectionUrl, req.FromDHCP, req.DNSServers); err != nil {
		s.logger.Error("SetDNS failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("set DNS failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleGetNTP(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	ntp, err := onvif.GetNTP(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetNTP failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get NTP failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, ntp)
}

func (s *PTZService) handleGetHostname(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	hostname, err := onvif.GetHostname(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetHostname failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get hostname failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"hostname": hostname})
}

type setHostnameReq struct {
	Hostname string `json:"hostname"`
}

func (s *PTZService) handleSetHostname(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	var req setHostnameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	if err := onvif.SetHostname(r.Context(), client, camera.ConnectionUrl, req.Hostname); err != nil {
		s.logger.Error("SetHostname failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("set hostname failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// ========== Recording Handlers ==========

func (s *PTZService) handleRecordingRouter(w http.ResponseWriter, r *http.Request, camera *damv1.Camera, parts []string) {
	if len(parts) < 2 {
		jsonError(w, "invalid recording path", http.StatusBadRequest)
		return
	}

	switch {
	case parts[0] == "recordings":
		switch {
		case len(parts) == 1 && r.Method == http.MethodGet:
			s.handleListRecordings(w, r, camera)
		case len(parts) == 1 && r.Method == http.MethodPost:
			s.handleCreateRecording(w, r, camera)
		case len(parts) == 2 && r.Method == http.MethodDelete:
			s.handleDeleteRecording(w, r, camera)
		case len(parts) == 3 && parts[2] == "tracks" && r.Method == http.MethodGet:
			s.handleGetRecordingTracks(w, r, camera)
		default:
			jsonError(w, "unknown recording action", http.StatusBadRequest)
		}
	case parts[0] == "replay" && r.Method == http.MethodGet:
		s.handleGetReplayURI(w, r, camera)
	case parts[0] == "jobs" && r.Method == http.MethodPost:
		s.handleCreateRecordingJob(w, r, camera)
	default:
		jsonError(w, "unknown recording action", http.StatusBadRequest)
	}
}

func (s *PTZService) handleListRecordings(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	recordingURL := onvif.BuildRecordingURL(camera.ConnectionUrl)
	recordings, err := onvif.GetRecordings(r.Context(), client, recordingURL)
	if err != nil {
		s.logger.Error("GetRecordings failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("list recordings failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"recordings": recordings})
}

func (s *PTZService) handleCreateRecording(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	var req struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	recordingURL := onvif.BuildRecordingURL(camera.ConnectionUrl)

	config := &onvif.RecordingConfiguration{
		Source:  req.Source,
		Content: req.Name,
	}

	token, err := onvif.CreateRecording(r.Context(), client, recordingURL, config)
	if err != nil {
		s.logger.Error("CreateRecording failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("create recording failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"recording_token": token, "status": "ok"})
}

func (s *PTZService) handleDeleteRecording(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/cameras/"), "/")
	if len(parts) < 4 {
		jsonError(w, "recording token required", http.StatusBadRequest)
		return
	}
	recordingToken := parts[3]

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	recordingURL := onvif.BuildRecordingURL(camera.ConnectionUrl)

	if err := onvif.DeleteRecording(r.Context(), client, recordingURL, recordingToken); err != nil {
		s.logger.Error("DeleteRecording failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("delete recording failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleGetRecordingTracks(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/cameras/"), "/")
	if len(parts) < 5 {
		jsonError(w, "recording token required", http.StatusBadRequest)
		return
	}
	recordingToken := parts[3]

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	recordingURL := onvif.BuildRecordingURL(camera.ConnectionUrl)

	tracks, err := onvif.GetRecordingTracks(r.Context(), client, recordingURL, recordingToken)
	if err != nil {
		s.logger.Error("GetRecordingTracks failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get recording tracks failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"tracks": tracks})
}

func (s *PTZService) handleGetReplayURI(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	recordingToken := r.URL.Query().Get("recording")
	if recordingToken == "" {
		jsonError(w, "recording query parameter required", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	replayURL := onvif.BuildReplayURL(camera.ConnectionUrl)
	profileToken := s.getONVIFProfileToken(r.Context(), client, camera.ConnectionUrl)

	uri, err := onvif.GetReplayURI(r.Context(), client, replayURL, recordingToken, profileToken)
	if err != nil {
		s.logger.Error("GetReplayURI failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get replay URI failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"uri": uri})
}

func (s *PTZService) handleCreateRecordingJob(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	var req struct {
		RecordingToken string `json:"recording_token"`
		ProfileToken   string `json:"profile_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	recordingURL := onvif.BuildRecordingURL(camera.ConnectionUrl)

	profileToken := req.ProfileToken
	if profileToken == "" {
		profileToken = s.getONVIFProfileToken(r.Context(), client, camera.ConnectionUrl)
	}

	jobToken, err := onvif.CreateRecordingJob(r.Context(), client, recordingURL, req.RecordingToken, profileToken)
	if err != nil {
		s.logger.Error("CreateRecordingJob failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("create recording job failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"job_token": jobToken, "status": "ok"})
}

// ========== Analytics Handlers ==========

func (s *PTZService) handleAnalyticsRouter(w http.ResponseWriter, r *http.Request, camera *damv1.Camera, parts []string) {
	if len(parts) == 0 {
		jsonError(w, "invalid analytics path", http.StatusBadRequest)
		return
	}

	switch {
	case parts[0] == "modules" && r.Method == http.MethodGet:
		s.handleGetAnalyticsModules(w, r, camera)
	case parts[0] == "rules":
		switch {
		case len(parts) == 1 && r.Method == http.MethodGet:
			s.handleGetAnalyticsRules(w, r, camera)
		case len(parts) == 1 && r.Method == http.MethodPost:
			s.handleCreateAnalyticsRule(w, r, camera)
		case len(parts) == 2 && r.Method == http.MethodDelete:
			s.handleDeleteAnalyticsRule(w, r, camera)
		default:
			jsonError(w, "unknown analytics rule action", http.StatusBadRequest)
		}
	case parts[0] == "state" && r.Method == http.MethodGet:
		s.handleGetAnalyticsState(w, r, camera)
	default:
		jsonError(w, "unknown analytics action", http.StatusBadRequest)
	}
}

func (s *PTZService) handleGetAnalyticsModules(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	analyticsURL := onvif.BuildAnalyticsURL(camera.ConnectionUrl)
	modules, err := onvif.GetAnalyticsModules(r.Context(), client, analyticsURL)
	if err != nil {
		s.logger.Error("GetAnalyticsModules failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get analytics modules failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"modules": modules})
}

func (s *PTZService) handleGetAnalyticsRules(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	analyticsURL := onvif.BuildAnalyticsURL(camera.ConnectionUrl)
	rules, err := onvif.GetSupportedAnalyticsRules(r.Context(), client, analyticsURL)
	if err != nil {
		s.logger.Error("GetSupportedAnalyticsRules failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get analytics rules failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"rules": rules})
}

func (s *PTZService) handleCreateAnalyticsRule(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	var req struct {
		RuleToken  string            `json:"rule_token"`
		RuleType   string            `json:"rule_type"`
		Parameters map[string]string `json:"parameters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	analyticsURL := onvif.BuildAnalyticsURL(camera.ConnectionUrl)

	if err := onvif.CreateAnalyticsRule(r.Context(), client, analyticsURL, req.RuleToken, req.RuleType, req.Parameters); err != nil {
		s.logger.Error("CreateAnalyticsRule failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("create analytics rule failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleDeleteAnalyticsRule(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/cameras/"), "/")
	if len(parts) < 4 {
		jsonError(w, "rule token required", http.StatusBadRequest)
		return
	}
	ruleToken := parts[3]

	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	analyticsURL := onvif.BuildAnalyticsURL(camera.ConnectionUrl)

	if err := onvif.DeleteAnalyticsRule(r.Context(), client, analyticsURL, ruleToken); err != nil {
		s.logger.Error("DeleteAnalyticsRule failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("delete analytics rule failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *PTZService) handleGetAnalyticsState(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)
	analyticsURL := onvif.BuildAnalyticsURL(camera.ConnectionUrl)
	state, err := onvif.GetAnalyticsState(r.Context(), client, analyticsURL)
	if err != nil {
		s.logger.Error("GetAnalyticsState failed", "camera", camera.Id, "error", err)
		jsonError(w, fmt.Sprintf("get analytics state failed: %v", err), http.StatusInternalServerError)
		return
	}
	jsonOK(w, state)
}

// ========== Diagnostics Handlers ==========

func (s *PTZService) handleDeviceDiagnostics(w http.ResponseWriter, r *http.Request, camera *damv1.Camera) {
	client := s.getONVIFClient(camera.ConnectionUrl, camera)

	info, err := onvif.GetDeviceInformation(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetDeviceInformation failed for diagnostics", "camera", camera.Id, "error", err)
	}

	caps, err := onvif.GetCapabilities(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetCapabilities failed for diagnostics", "camera", camera.Id, "error", err)
	}

	dateTime, err := onvif.GetSystemDateAndTime(r.Context(), client, camera.ConnectionUrl)
	if err != nil {
		s.logger.Error("GetSystemDateAndTime failed for diagnostics", "camera", camera.Id, "error", err)
	}

	jsonOK(w, map[string]interface{}{
		"device_info":  info,
		"capabilities": caps,
		"date_time":    dateTime,
	})
}

func (s *PTZService) handleServiceDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	jsonOK(w, map[string]interface{}{
		"goroutines": runtime.NumGoroutine(),
		"memory": map[string]interface{}{
			"alloc_mb":       memStats.Alloc / 1024 / 1024,
			"total_alloc_mb": memStats.TotalAlloc / 1024 / 1024,
			"sys_mb":         memStats.Sys / 1024 / 1024,
			"num_gc":         memStats.NumGC,
		},
		"config": s.config,
	})
}

// ========== Helpers ==========

func (s *PTZService) getVideoSourceToken(ctx context.Context, client *onvif.SOAPClient, baseURL string) string {
	mediaURL := onvif.BuildMediaURL(baseURL)
	profiles, err := onvif.GetProfiles(ctx, client, mediaURL)
	if err != nil || len(profiles) == 0 {
		return ""
	}
	mainProfile := onvif.FindMainProfile(profiles)
	if mainProfile != nil && mainProfile.VideoSource != nil {
		return mainProfile.VideoSource.Token
	}
	if profiles[0].VideoSource != nil {
		return profiles[0].VideoSource.Token
	}
	return ""
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	common.CheckJWTSecret()

	if err := common.InitTelemetry("camera-control"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultPTZConfig()
	common.StartMetricsServer(config.MetricsAddr)
	common.StartResourceMonitor(ctx)

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

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := service.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}
}

func extractParam(path, prefix string) string {
	trimmed := strings.TrimPrefix(path, prefix)
	idx := strings.IndexByte(trimmed, '/')
	if idx == -1 {
		return trimmed
	}
	return trimmed[:idx]
}
