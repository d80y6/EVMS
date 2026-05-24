package common

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	FramesProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "vms_frames_processed_total",
		Help: "Total number of video frames processed",
	}, []string{"camera_id", "type"})

	InferenceDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "vms_inference_duration_seconds",
		Help:    "Duration of AI inference in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"camera_id", "model"})

	StreamActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "vms_stream_active",
		Help: "Indicates if a camera stream is currently active (1) or inactive (0)",
	}, []string{"camera_id"})

	RecordingsIndexed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "vms_recordings_indexed_total",
		Help: "Total number of recording segments indexed",
	}, []string{"camera_id"})
)

// StartMetricsServer starts a Prometheus metrics endpoint on the given port
func StartMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(addr, mux)
}
