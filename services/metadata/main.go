package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	dbURL := os.Getenv("DB_URL")
	natsURL := os.Getenv("NATS_URL")

	db, err := sqlx.Connect("postgres", dbURL)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	nc, err := nats.Connect(natsURL)
	if err != nil {
		logger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	// Metadata service supports vector embeddings
	_, err = nc.Subscribe("camera.*.events", func(msg *nats.Msg) {
		var detections []struct {
			Label      string    `json:"label"`
			Confidence float64   `json:"confidence"`
			BBox       []float64 `json:"bbox"`
			Embedding  []float32 `json:"embedding,omitempty"`
		}

		if err := json.Unmarshal(msg.Data, &detections); err != nil {
			logger.Error("Failed to unmarshal AI event", "error", err)
			return
		}

		// Subject format: camera.<id>.events
		parts := strings.Split(msg.Subject, ".")
		if len(parts) < 2 {
			logger.Warn("Invalid NATS subject for AI event", "subject", msg.Subject)
			return
		}
		cameraID := parts[1]

		for _, d := range detections {
			bbox, _ := json.Marshal(d.BBox)

			if len(d.Embedding) > 0 {
				embeddingJSON, _ := json.Marshal(d.Embedding)
				_, err = db.Exec(`INSERT INTO ai_events (camera_id, object_type, confidence, bounding_box, embedding, event_time)
                                  VALUES ($1, $2, $3, $4, $5, NOW())`,
					cameraID, d.Label, d.Confidence, string(bbox), string(embeddingJSON))
			} else {
				_, err = db.Exec(`INSERT INTO ai_events (camera_id, object_type, confidence, bounding_box, event_time)
                                  VALUES ($1, $2, $3, $4, NOW())`,
					cameraID, d.Label, d.Confidence, string(bbox))
			}

			if err != nil {
				logger.Error("Failed to store AI event", "error", err)
			}
		}
	})

	logger.Info("AI Metadata Service (Vector-Enabled) listening...")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}
