package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/nats-io/nats.go"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
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
	logger    *slog.Logger
	natsConn  *nats.Conn
	sessions  map[string]*StreamSession
	sessionsMu sync.RWMutex
	config    WebRTCConfig
}

// WebRTCConfig holds configuration for the WebRTC service
type WebRTCConfig struct {
	HTTPAddr     string
	MetricsAddr  string
	NATSURL      string
	ICEServers   []string
	JWTSecretEnv string
}

// DefaultWebRTCConfig returns a configuration with sensible defaults
func DefaultWebRTCConfig() WebRTCConfig {
	return WebRTCConfig{
		HTTPAddr:     ":8082",
		MetricsAddr:  ":2112",
		NATSURL:      "nats://nats:4222",
		ICEServers:   []string{"stun:stun.l.google.com:19302"},
		JWTSecretEnv: "JWT_SECRET",
	}
}

// NewWebRTCService creates a new WebRTC service instance
func NewWebRTCService(ctx context.Context, config WebRTCConfig, logger *slog.Logger) (*WebRTCService, error) {
	nc, err := nats.Connect(config.NATSURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &WebRTCService{
		logger:   logger,
		natsConn: nc,
		sessions: make(map[string]*StreamSession),
		config:   config,
	}, nil
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

	return nil
}

// createOfferHandler handles WebRTC offer requests
func (s *WebRTCService) createOfferHandler(w http.ResponseWriter, r *http.Request) {
	cameraID := r.URL.Query().Get("camera_id")
	if cameraID == "" {
		http.Error(w, "camera_id required", http.StatusBadRequest)
		return
	}

	var offer webrtc.SessionDescription
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		http.Error(w, "invalid offer", http.StatusBadRequest)
		return
	}

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: s.config.ICEServers}},
	}

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"pion",
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err = peerConnection.AddTrack(videoTrack); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err = peerConnection.SetLocalDescription(answer); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sub, err := s.natsConn.Subscribe(fmt.Sprintf("camera.%s.h264", cameraID), func(msg *nats.Msg) {
		if err := videoTrack.WriteSample(media.Sample{Data: msg.Data}); err != nil {
			s.logger.Warn("Failed to write video sample", "error", err)
		}
		common.FramesProcessed.WithLabelValues(cameraID, "webrtc").Inc()
	})
	if err != nil {
		s.logger.Error("Failed to subscribe to camera stream", "camera_id", cameraID, "error", err)
		peerConnection.Close()
		http.Error(w, "failed to subscribe to stream", http.StatusInternalServerError)
		return
	}

	session := &StreamSession{
		peerConnection: peerConnection,
		videoTrack:     videoTrack,
		cameraID:       cameraID,
		subscription:   sub,
	}

	s.sessionsMu.Lock()
	s.sessions[cameraID] = session
	s.sessionsMu.Unlock()

	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			s.cleanupSession(cameraID)
		}
	})

	if err := json.NewEncoder(w).Encode(answer); err != nil {
		s.logger.Error("Failed to encode answer", "error", err)
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
	s.logger.Info("Cleaned up WebRTC session", "camera_id", cameraID)
}

// healthHandler handles health check requests
func (s *WebRTCService) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// Start starts the HTTP server
func (s *WebRTCService) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webrtc/offer", common.JWTAuthMiddleware(s.createOfferHandler))
	mux.HandleFunc("/health", s.healthHandler)

	s.logger.Info("WebRTC Relay Service listening", "address", s.config.HTTPAddr)
	return http.ListenAndServe(s.config.HTTPAddr, mux)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultWebRTCConfig()
	if addr := os.Getenv("HTTP_ADDR"); addr != "" {
		config.HTTPAddr = addr
	}
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		config.NATSURL = natsURL
	}

	common.StartMetricsServer(config.MetricsAddr)

	service, err := NewWebRTCService(ctx, config, logger)
	if err != nil {
		logger.Error("Failed to create WebRTC service", "error", err)
		os.Exit(1)
	}
	defer service.Close()

	if err := service.Start(); err != nil {
		logger.Error("WebRTC service failed", "error", err)
		os.Exit(1)
	}
}
