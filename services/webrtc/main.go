package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/nats-io/nats.go"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

type StreamSession struct {
	peerConnection *webrtc.PeerConnection
	videoTrack     *webrtc.TrackLocalStaticSample
}

var (
	sessions   = make(map[string]*StreamSession)
	sessionsMu sync.RWMutex
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Start metrics and health check
	common.StartMetricsServer(":2112")
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://nats:4222"
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		logger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	}

	http.HandleFunc("/webrtc/offer", func(w http.ResponseWriter, r *http.Request) {
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

		peerConnection, err := webrtc.NewPeerConnection(config)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Use H264 for WebRTC (matching ingest output 3)
		videoTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
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

		// Connect NATS H264 stream to WebRTC track
		nc.Subscribe(fmt.Sprintf("camera.%s.h264", cameraID), func(msg *nats.Msg) {
			videoTrack.WriteSample(media.Sample{Data: msg.Data})
		})

		json.NewEncoder(w).Encode(answer)
	})

	logger.Info("WebRTC Relay Service listening", "address", ":8082")
	if err := http.ListenAndServe(":8082", nil); err != nil {
		logger.Error("WebRTC service failed", "error", err)
		os.Exit(1)
	}
}
