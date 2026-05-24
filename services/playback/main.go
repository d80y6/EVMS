package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dam-vms/dam/pkg/common"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	recordingsRoot := "/recordings"
	// Ensure recordingsRoot is absolute and cleaned
	absRoot, err := filepath.Abs(recordingsRoot)
	if err != nil {
		logger.Error("Failed to get absolute path for recordings root", "error", err)
		os.Exit(1)
	}

	// Serve recorded mp4 files over HTTP with security hardening
	playbackHandler := func(w http.ResponseWriter, r *http.Request) {
		// 1. Path Sanitization & Traversal Prevention
		relPath := r.URL.Path[len("/playback/"):]
		fullPath := filepath.Join(absRoot, relPath)

		// Ensure the resulting path is still within recordingsRoot
		finalPath, err := filepath.Abs(fullPath)
		if err != nil || !strings.HasPrefix(finalPath, absRoot) {
			logger.Warn("Blocked traversal attempt", "path", relPath, "remote_addr", r.RemoteAddr)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// 2. Check if file exists and is not a directory
		info, err := os.Stat(finalPath)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if info.IsDir() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		logger.Info("Playback request", "path", finalPath)
		http.ServeFile(w, r, finalPath)
	}

	// Wrap with JWT Authentication
	http.HandleFunc("/playback/", common.JWTAuthMiddleware(playbackHandler))

	logger.Info("Hardened Playback Service listening", "address", ":8086", "root", absRoot)
	if err := http.ListenAndServe(":8086", nil); err != nil {
		logger.Error("Playback service failed", "error", err)
		os.Exit(1)
	}
}
