package main

import (
	"net/http"
	"github.com/dam-vms/dam/services/webrtc-relay/internal/health"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health.Handler("webrtc-relay"))

	http.ListenAndServe(":8080", mux)
}
