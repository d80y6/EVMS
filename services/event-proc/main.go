package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dam-vms/dam/services/event-proc/internal/health"
	"github.com/nats-io/nats.go"
)

type Detection struct {
	Label      string    `json:"label"`
	Confidence float64   `json:"confidence"`
	BBox       []float64 `json:"bbox"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", health.Handler("event-proc"))
		http.ListenAndServe(":8080", mux)
	}()

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

	// Correlate AI detections with notifications
	nc.Subscribe("camera.*.events", func(msg *nats.Msg) {
		var detections []Detection
		if err := json.Unmarshal(msg.Data, &detections); err != nil {
			return
		}

		for _, d := range detections {
			if d.Label == "person" && d.Confidence > 0.8 {
				logger.Info("Person detected! Triggering notification.", "camera", msg.Subject)

				// Publish to notification service
				notification := map[string]string{
					"title":   "Security Alert",
					"message": fmt.Sprintf("Person detected on camera %s", msg.Subject),
				}
				data, _ := json.Marshal(notification)
				nc.Publish("notifications.push", data)
			}
		}
	})

	logger.Info("Event Processing Service started...")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}
