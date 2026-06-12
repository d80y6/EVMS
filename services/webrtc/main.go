package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dam-vms/dam/api/v1"
	"github.com/dam-vms/dam/pkg/common"
	"github.com/nats-io/nats.go"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// StreamSession represents an active WebRTC streaming session
type StreamSession struct {
	peerConnection *webrtc.PeerConnection
	videoTrack     *webrtc.TrackLocalStaticSample
	cameraID       string
	subscription   *nats.Subscription
}

// WebRTCService manages WebRTC streaming sessions
type WebRTCService struct {
	logger        *slog.Logger
	natsConn      *nats.Conn
	sessions      map[string]*StreamSession
	sessionsMu    sync.RWMutex
	config        WebRTCConfig
	healthHandler *common.HealthHandler
	webrtcAPI     *webrtc.API
	cameraCC      *grpc.ClientConn
	cameraSvc     damv1.CameraServiceClient
}

// WebRTCConfig holds configuration for the WebRTC service
type WebRTCConfig struct {
	HTTPAddr        string
	MetricsAddr     string
	NATSURL         string
	ICEServers      []string
	JWTSecretEnv    string
	ICEPort         int
	ICEHost         string
	CameraSvcAddr   string
}

func detectHostIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func iceHostOrDefault() string {
	if h := os.Getenv("WEBRTC_HOST_IP"); h != "" {
		return h
	}
	if h := detectHostIP(); h != "" {
		return h
	}
	return ""
}

// DefaultWebRTCConfig returns a configuration with sensible defaults
func DefaultWebRTCConfig() WebRTCConfig {
	iceServers := []string{}
	if turnURL := os.Getenv("TURN_URL"); turnURL != "" {
		if turnUsername := os.Getenv("TURN_USERNAME"); turnUsername != "" {
			if turnCredential := os.Getenv("TURN_CREDENTIAL"); turnCredential != "" {
				iceServers = append(iceServers, turnUsername+":"+turnCredential+"@"+turnURL)
			}
		}
	}
	icePort := 8083
	if p, err := strconv.Atoi(os.Getenv("WEBRTC_ICE_PORT")); err == nil && p > 0 {
		icePort = p
	}

	return WebRTCConfig{
		HTTPAddr:        common.GetEnv("HTTP_ADDR", ":8082"),
		MetricsAddr:     common.GetEnv("METRICS_ADDR", ":2112"),
		NATSURL:         common.GetEnv("NATS_URL", "nats://nats:4222"),
		ICEServers:      iceServers,
		JWTSecretEnv:   "JWT_SECRET",
		ICEPort:         icePort,
		ICEHost:         iceHostOrDefault(),
		CameraSvcAddr:   common.GetEnv("CAMERA_SERVICE_ADDR", "camera-mgmt:50051"),
	}
}

// tenantUnaryInterceptor forwards tenant_id from context into gRPC metadata.
func tenantUnaryInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if tenant := common.TenantFromContext(ctx); tenant != "" {
		md := metadata.Pairs("tenant_id", tenant)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	return invoker(ctx, method, req, reply, cc, opts...)
}

// NewWebRTCService creates a new WebRTC service instance
func NewWebRTCService(ctx context.Context, config WebRTCConfig, logger *slog.Logger) (*WebRTCService, error) {
	nc, err := nats.Connect(config.NATSURL, append(common.NATSTLSOptions(),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	creds, err := common.GRPCClientTLSCredentials("camera-mgmt")
	if err != nil {
		return nil, fmt.Errorf("failed to configure gRPC credentials: %w", err)
	}
	cameraCC, err := grpc.NewClient(config.CameraSvcAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype("json")),
		grpc.WithUnaryInterceptor(tenantUnaryInterceptor),
	)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to connect to camera service: %w", err)
	}
	cameraSvc := damv1.NewCameraServiceClient(cameraCC)

	svc := &WebRTCService{
		logger:        logger,
		natsConn:      nc,
		sessions:      make(map[string]*StreamSession),
		config:        config,
		healthHandler: common.NewHealthHandler(),
		cameraCC:      cameraCC,
		cameraSvc:     cameraSvc,
	}
	svc.healthHandler.AddNATSChecker(nc, "nats")

	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("failed to register default codecs: %w", err)
	}
	apiOpts := []func(*webrtc.API){webrtc.WithMediaEngine(m)}

	// Set up ICE UDP mux and NAT 1:1 mapping for Docker deployment
	if config.ICEPort > 0 || config.ICEHost != "" {
		var se webrtc.SettingEngine

		if config.ICEPort > 0 {
			udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: config.ICEPort})
			if err != nil {
				logger.Warn("Failed to bind ICE UDP port, using ephemeral", "port", config.ICEPort, "error", err)
			} else {
				mux := ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: udpConn})
				se.SetICEUDPMux(mux)
				logger.Info("ICE UDP mux bound", "port", config.ICEPort)
			}
		}

		if config.ICEHost != "" {
			se.SetNAT1To1IPs([]string{config.ICEHost}, webrtc.ICECandidateTypeHost)
			logger.Info("NAT 1:1 mapping configured", "host", config.ICEHost, "candidate_type", "host")
		}

		apiOpts = append(apiOpts, webrtc.WithSettingEngine(se))
	}

	svc.webrtcAPI = webrtc.NewAPI(apiOpts...)

	return svc, nil
}

// Close gracefully shuts down the service
func (s *WebRTCService) Close() error {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()

	for _, session := range s.sessions {
		if session.subscription != nil {
			session.subscription.Unsubscribe()
		}
		if session.peerConnection != nil {
			session.peerConnection.Close()
		}
	}

	if s.natsConn != nil {
		s.natsConn.Close()
	}
	if s.cameraCC != nil {
		s.cameraCC.Close()
	}

	return nil
}

// createOfferHandler handles WebRTC offer requests
func (s *WebRTCService) createOfferHandler(w http.ResponseWriter, r *http.Request) {
	cameraID := r.URL.Query().Get("camera_id")
	if cameraID == "" {
		http.Error(w, "camera_id required", http.StatusBadRequest)
		return
	}

	username := common.UserFromContext(r.Context())
	if username == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	tenantID := common.TenantFromContext(ctx)

	if s.cameraSvc != nil {
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if _, err := s.cameraSvc.GetCamera(verifyCtx, &damv1.GetCameraRequest{Id: cameraID}); err != nil {
			s.logger.Warn("camera access denied", "camera_id", cameraID, "user", username, "tenant", tenantID, "error", err)
			http.Error(w, "camera not found or access denied", http.StatusNotFound)
			return
		}
	} else {
		s.logger.Warn("camera service not configured, skipping camera-level auth", "camera_id", cameraID)
	}

	s.logger.Info("WebRTC offer request received", "camera_id", cameraID, "remote_addr", r.RemoteAddr, "user", username)

	var offer webrtc.SessionDescription
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		s.logger.Error("Failed to decode offer", "camera_id", cameraID, "error", err)
		http.Error(w, "invalid offer", http.StatusBadRequest)
		return
	}
	s.logger.Info("Offer decoded successfully", "camera_id", cameraID, "offer_type", offer.Type)

	iceServers := []webrtc.ICEServer{}
	for _, url := range s.config.ICEServers {
		if strings.Contains(url, "@") {
			parts := strings.SplitN(url, "@", 2)
			creds := strings.SplitN(parts[0], ":", 2)
			if len(creds) == 2 {
				iceServers = append(iceServers, webrtc.ICEServer{
					URLs:           []string{parts[1]},
					Username:       creds[0],
					Credential:     creds[1],
					CredentialType: webrtc.ICECredentialTypePassword,
				})
				continue
			}
		}
		iceServers = append(iceServers, webrtc.ICEServer{URLs: []string{url}})
	}
	config := webrtc.Configuration{
		ICEServers: iceServers,
	}

	peerConnection, err := s.webrtcAPI.NewPeerConnection(config)
	if err != nil {
		s.logger.Error("Failed to create PeerConnection", "camera_id", cameraID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logger.Info("PeerConnection created", "camera_id", cameraID)

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"pion",
	)
	if err != nil {
		s.logger.Error("Failed to create video track", "camera_id", cameraID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logger.Info("Video track created", "camera_id", cameraID)

	if _, err = peerConnection.AddTrack(videoTrack); err != nil {
		s.logger.Error("Failed to add track", "camera_id", cameraID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logger.Info("Track added to PeerConnection", "camera_id", cameraID)

	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		s.logger.Error("Failed to set remote description", "camera_id", cameraID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logger.Info("Remote description set", "camera_id", cameraID)

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		s.logger.Error("Failed to create answer", "camera_id", cameraID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logger.Info("Answer created", "camera_id", cameraID)

	if err = peerConnection.SetLocalDescription(answer); err != nil {
		s.logger.Error("Failed to set local description", "camera_id", cameraID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logger.Info("Local description set", "camera_id", cameraID)

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	select {
	case <-gatherComplete:
		s.logger.Info("ICE gathering completed", "camera_id", cameraID)
	case <-time.After(5 * time.Second):
		s.logger.Warn("ICE gathering timed out, sending answer with partial candidates", "camera_id", cameraID)
	}

	lastFrameTime := time.Now()
	sub, err := s.natsConn.Subscribe(fmt.Sprintf("camera.%s.h264", cameraID), func(msg *nats.Msg) {
		now := time.Now()
		dur := now.Sub(lastFrameTime)
		if dur < 16*time.Millisecond {
			dur = 16 * time.Millisecond
		} else if dur > 250*time.Millisecond {
			dur = 250 * time.Millisecond
		}
		lastFrameTime = now

		if err := videoTrack.WriteSample(media.Sample{Duration: dur, Data: msg.Data}); err != nil {
			s.logger.Warn("Failed to write video sample", "error", err, "len", len(msg.Data))
		} else {
			s.logger.Debug("Wrote video sample", "len", len(msg.Data), "dur", dur)
		}
		common.FramesProcessed.WithLabelValues(cameraID, "webrtc").Inc()
	})
	if err != nil {
		s.logger.Error("Failed to subscribe to camera stream", "camera_id", cameraID, "error", err)
		peerConnection.Close()
		http.Error(w, "failed to subscribe to stream", http.StatusInternalServerError)
		return
	}
	s.logger.Info("NATS subscription created", "camera_id", cameraID)

	sub.SetPendingLimits(256, 32*1024*1024)

	session := &StreamSession{
		peerConnection: peerConnection,
		videoTrack:     videoTrack,
		cameraID:       cameraID,
		subscription:   sub,
	}

	s.sessionsMu.Lock()
	s.sessions[cameraID] = session
	common.WebRTCSessionsActive.Set(float64(len(s.sessions)))
	s.sessionsMu.Unlock()

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		s.logger.Info("PeerConnection state changed", "camera_id", cameraID, "state", state.String())
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			s.cleanupSession(cameraID)
		}
	})

	localDesc := peerConnection.LocalDescription()
	if localDesc == nil {
		s.logger.Error("Local description is nil after SetLocalDescription", "camera_id", cameraID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.logger.Info("Sending answer to client", "camera_id", cameraID)
	candidateCount := 0
	for _, line := range strings.Split(localDesc.SDP, "\n") {
		if strings.Contains(line, "a=candidate:") {
			candidateCount++
			s.logger.Info("ICE candidate in answer", "camera_id", cameraID, "candidate", strings.TrimSpace(line))
		}
	}
	s.logger.Info("ICE candidate summary", "camera_id", cameraID, "count", candidateCount)
	if err := json.NewEncoder(w).Encode(*localDesc); err != nil {
		s.logger.Error("Failed to encode answer", "camera_id", cameraID, "error", err)
	}
}

// cleanupSession removes and cleans up a session
func (s *WebRTCService) cleanupSession(cameraID string) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()

	session, exists := s.sessions[cameraID]
	if !exists {
		return
	}

	if session.subscription != nil {
		session.subscription.Unsubscribe()
	}
	if session.peerConnection != nil {
		session.peerConnection.Close()
	}

	delete(s.sessions, cameraID)
	common.WebRTCSessionsActive.Set(float64(len(s.sessions)))
	s.logger.Info("Cleaned up WebRTC session", "camera_id", cameraID)
}

// Start starts the HTTP server and blocks until ctx is cancelled
func (s *WebRTCService) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webrtc/offer", common.JWTAuthMiddleware(s.createOfferHandler))
	mux.HandleFunc("/health", s.healthHandler.Liveness)
	mux.HandleFunc("/ready", s.healthHandler.Readiness)

	server := &http.Server{
		Addr:         s.config.HTTPAddr,
		Handler:      common.RecoveryMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		s.logger.Info("WebRTC Relay Service listening", "address", s.config.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("WebRTC server error", "error", err)
		}
	}()

	<-ctx.Done()
	s.logger.Info("Shutting down WebRTC Service...")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func main() {
	logger := common.NewLogger("webrtc")
	slog.SetDefault(logger)

	common.CheckJWTSecret()

	if err := common.InitTelemetry("webrtc"); err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
	}
	defer common.ShutdownTelemetry()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultWebRTCConfig()

	common.StartMetricsServer(config.MetricsAddr)
	common.StartResourceMonitor(ctx)

	service, err := NewWebRTCService(ctx, config, logger)
	if err != nil {
		logger.Error("Failed to create WebRTC service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(ctx); err != nil {
		logger.Error("WebRTC service failed", "error", err)
		os.Exit(1)
	}
}
