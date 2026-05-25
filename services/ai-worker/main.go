package main

import (
	"net/http"
	"github.com/dam-vms/dam/services/ai-worker/internal/health"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health.Handler("ai-worker"))

	http.ListenAndServe(":8080", mux)
}
