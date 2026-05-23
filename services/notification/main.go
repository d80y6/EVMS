package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
)

type Notification struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Type    string `json:"type"` // 'email', 'webhook', 'push'
}

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

	// Handle push notifications
	nc.Subscribe("notifications.push", func(msg *nats.Msg) {
		var n Notification
		if err := json.Unmarshal(msg.Data, &n); err != nil {
			return
		}

		logger.Info("Sending Notification", "title", n.Title, "message", n.Message)
		// In a real system, integrated with SendGrid, Twilio, or FCM here.
	})

	logger.Info("Notification Service started...")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}
