package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	recordingsRoot := "/recordings"

	// Serve recorded mp4 files over HTTP
	http.HandleFunc("/playback/", func(w http.ResponseWriter, r *http.Request) {
		// Basic path sanitization for an enterprise foundation
		path := filepath.Clean(r.URL.Path[len("/playback/"):])
		fullPath := filepath.Join(recordingsRoot, path)

		logger.Info("Playback request", "path", fullPath)
		http.ServeFile(w, r, fullPath)
	})

	logger.Info("Playback Service listening", "address", ":8086")
	if err := http.ListenAndServe(":8086", nil); err != nil {
		logger.Error("Playback service failed", "error", err)
		os.Exit(1)
	}
}
