package main

import (
	"fmt"
	"os"

	"github.com/dam-vms/dam/sdk/evms"
)

func main() {
	baseURL := os.Getenv("EVMS_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8090"
	}
	apiKey := os.Getenv("EVMS_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "EVMS_API_KEY environment variable required")
		os.Exit(1)
	}

	client := evms.NewClient(baseURL, apiKey)
	cameras, err := client.ListCameras()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list cameras: %v\n", err)
		os.Exit(1)
	}
	for _, cam := range cameras {
		fmt.Printf("Camera: %s (ID: %s, Status: %s)\n", cam["name"], cam["id"], cam["status"])
	}
}
