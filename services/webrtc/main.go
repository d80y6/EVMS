package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

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
	logger     *slog.Logger
	natsConn   *nats.Conn
	sessions   map[string]*StreamSession
	sessionsMu sync.RWMutex
	config     WebRTCConfig
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
	iceServers := []string{"stun:stun.l.google.com:19302"}
	if turnURL := os.Getenv("TURN_URL"); turnURL != "" {
		if turnUsername := os.Getenv("TURN_USERNAME"); turnUsername != "" {
			if turnCredential := os.Getenv("TURN_CREDENTIAL"); turnCredential != "" {
				iceServers = append(iceServers, turnUsername+":"+turnCredential+"@"+turnURL)
			}
		}
	}
	return WebRTCConfig{
		HTTPAddr:     common.GetEnv("HTTP_ADDR", ":8082"),
		MetricsAddr:  common.GetEnv("METRICS_ADDR", ":2112"),
		NATSURL:      common.GetEnv("NATS_URL", "nats://nats:4222"),
		ICEServers:   iceServers,
		JWTSecretEnv: "JWT_SECRET",
	}
}

// NewWebRTCService creates a new WebRTC service instance
func NewWebRTCService(ctx context.Context, config WebRTCConfig, logger *slog.Logger) (*WebRTCService, error) {
	nc, err := nats.Connect(config.NATSURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
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
					CredentialType: 		webrtc.ICECredentialTypePassword,
				})
				continue
			}
		}
		iceServers = append(iceServers, webrtc.ICEServer{URLs: []string{url}})
	}
	config := webrtc.Configuration{
		ICEServers: iceServers,
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

	sub.SetPendingLimits(256, 32*1024*1024)

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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// Start starts the HTTP server and blocks until ctx is cancelled
func (s *WebRTCService) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webrtc/offer", common.JWTAuthMiddleware(s.createOfferHandler))
	mux.HandleFunc("/health", s.healthHandler)

	server := &http.Server{
		Addr:         s.config.HTTPAddr,
		Handler:      mux,
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config := DefaultWebRTCConfig()

	common.StartMetricsServer(config.MetricsAddr)

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
