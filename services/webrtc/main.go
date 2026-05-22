package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

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

	// WebRTC configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	http.HandleFunc("/webrtc/offer", func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		if cameraID == "" {
			http.Error(w, "camera_id is required", http.StatusBadRequest)
			return
		}

		var offer webrtc.SessionDescription
		if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		peerConnection, err := webrtc.NewPeerConnection(config)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Create a video track
		videoTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = peerConnection.AddTrack(videoTrack)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Set the remote SessionDescription
		err = peerConnection.SetRemoteDescription(offer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Create an answer
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Sets the LocalDescription, and start our UDP listeners
		err = peerConnection.SetLocalDescription(answer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Store session
		sessionsMu.Lock()
		sessions[cameraID] = &StreamSession{
			peerConnection: peerConnection,
			videoTrack:     videoTrack,
		}
		sessionsMu.Unlock()

		// Subscribe to NATS frames for this camera
		// NOTE: This implementation assumes the ingest service provides H264 via NATS
		// In our current setup, ingest provides MJPEG. For WebRTC, we should ideally
		// have a dedicated H264 stream or transcode.
		nc.Subscribe(fmt.Sprintf("camera.%s.frames", cameraID), func(msg *nats.Msg) {
			videoTrack.WriteSample(media.Sample{Data: msg.Data})
		})

		json.NewEncoder(w).Encode(answer)
	})

	logger.Info("WebRTC Signaling Service listening", "address", ":8082")
	if err := http.ListenAndServe(":8082", nil); err != nil {
		logger.Error("WebRTC service failed", "error", err)
		os.Exit(1)
	}
}
