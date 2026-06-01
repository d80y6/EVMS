package common

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	registry         = prometheus.NewRegistry()
	FramesProcessed  = newCounterVec("vms_frames_processed_total", "Total number of video frames processed", []string{"camera_id", "type"})
	InferenceDuration = newHistogramVec("vms_inference_duration_seconds", "Duration of AI inference in seconds", []string{"camera_id", "model"})
	StreamActive     = newGaugeVec("vms_stream_active", "Indicates if a camera stream is currently active (1) or inactive (0)", []string{"camera_id"})
	RecordingsIndexed = newCounterVec("vms_recordings_indexed_total", "Total number of recording segments indexed", []string{"camera_id"})
)

func newCounterVec(name, help string, labels []string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	registry.MustRegister(c)
	return c
}

func newHistogramVec(name, help string, labels []string) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: help}, labels)
	registry.MustRegister(h)
	return h
}

func newGaugeVec(name, help string, labels []string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
	registry.MustRegister(g)
	return g
}

// StartMetricsServer starts a Prometheus metrics and health endpoint on the given port
func StartMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Warn("Metrics server error", "error", err)
		}
	}()
}
