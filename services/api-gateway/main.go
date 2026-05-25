package main

import (
	"net/http"
	"github.com/dam-vms/dam/services/api-gateway/internal/health"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health.Handler("api-gateway"))

	http.ListenAndServe(":8080", mux)
}
