package common

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	registry = prometheus.NewRegistry()

	// Existing metrics
	FramesProcessed   = newCounterVec("vms_frames_processed_total", "Total number of video frames processed", []string{"camera_id", "type"})
	InferenceDuration = newHistogramVec("vms_inference_duration_seconds", "Duration of AI inference in seconds", []string{"camera_id", "model"})
	StreamActive      = newGaugeVec("vms_stream_active", "Indicates if a camera stream is currently active (1) or inactive (0)", []string{"camera_id"})
	RecordingsIndexed = newCounterVec("vms_recordings_indexed_total", "Total number of recording segments indexed", []string{"camera_id"})

	// Stream performance metrics
	StreamLatencyMs       = newGaugeVec("vms_stream_latency_ms", "Stream latency in milliseconds", []string{"camera_id"})
	FramesDroppedTotal    = newCounterVec("vms_frames_dropped_total", "Total number of video frames dropped", []string{"camera_id", "reason"})
	StreamBitrate         = newGaugeVec("vms_stream_bitrate_bps", "Current stream bitrate in bits per second", []string{"camera_id"})
	SegmentWriteDuration  = newHistogramVec("vms_segment_write_duration_seconds", "Duration of recording segment writes", []string{"camera_id"})

	// Infrastructure metrics
	DBQueryDuration       = newHistogramVec("vms_db_query_duration_seconds", "Duration of database queries", []string{"query_type"})
	DBConnectionsOpen     = newGaugeVec("vms_db_connections_open", "Number of open database connections", []string{"pool"})
	DBConnectionsInUse    = newGaugeVec("vms_db_connections_in_use", "Number of database connections in use", []string{"pool"})
	NATSMessageLatency    = newHistogramVec("vms_nats_message_latency_seconds", "NATS message processing latency", []string{"subject"})
	NATSConnectionsOpen   = newGauge("vms_nats_connections_open", "Number of open NATS connections")
	GRPCRequestDuration   = newHistogramVec("vms_grpc_request_duration_seconds", "gRPC request latency", []string{"method", "status"})
	HTTPRequestDuration   = newHistogramVec("vms_http_request_duration_seconds", "HTTP request latency", []string{"method", "path", "status"})
	ErrorCounter          = newCounterVec("vms_http_requests_errors_total", "Total number of HTTP request errors", []string{"method", "path"})

	// Resource metrics
	DiskUsageBytes        = newGaugeVec("vms_disk_usage_bytes", "Disk usage in bytes", []string{"path", "type"})
	DiskFreeBytes         = newGaugeVec("vms_disk_free_bytes", "Disk free space in bytes", []string{"path"})
	ActiveConnections     = newGaugeVec("vms_active_connections", "Active connection count by type", []string{"type"})
	WebRTCSessionsActive  = newGauge("vms_webrtc_sessions_active", "Number of active WebRTC sessions")
	GoRoutinesActive      = newGauge("vms_goroutines_active", "Number of active goroutines")
	MemoryUsageBytes      = newGauge("vms_memory_usage_bytes", "Process memory usage in bytes")

	// Recording integrity metrics
	RecordingGaps = newCounterVec("vms_recording_gaps_total", "Total number of recording gaps detected", []string{"camera_id"})

	// Operation metrics
	LeaderElected         = newGaugeVec("vms_leader_elected", "Leader election status (1=leader, 0=follower)", []string{"service", "shard"})
	HealthCheckStatus     = newGaugeVec("vms_health_check_status", "Health check status (1=healthy, 0=unhealthy)", []string{"service", "check"})
	CircuitBreakerState   = newGaugeVec("vms_circuit_breaker_state", "Circuit breaker state (0=closed, 1=open, 2=half-open)", []string{"name"})
	UpTime                = newGauge("vms_uptime_seconds", "Process uptime in seconds")
)

func init() {
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

func newCounterVec(name, help string, labels []string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	registry.MustRegister(c)
	return c
}

func newGauge(name string, help string) prometheus.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: help})
	registry.MustRegister(g)
	return g
}

func newGaugeVec(name, help string, labels []string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
	registry.MustRegister(g)
	return g
}

func newHistogramVec(name, help string, labels []string) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: help}, labels)
	registry.MustRegister(h)
	return h
}

func ObserveWithTrace(obs prometheus.Observer, val float64, traceID string) {
	if traceID == "" {
		obs.Observe(val)
		return
	}
	if eo, ok := obs.(prometheus.ExemplarObserver); ok {
		eo.ObserveWithExemplar(val, prometheus.Labels{"trace_id": traceID})
		return
	}
	obs.Observe(val)
}

func StartMetricsServer(addr string) {
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
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

func RecordResourceMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	MemoryUsageBytes.Set(float64(m.Alloc))
	GoRoutinesActive.Set(float64(runtime.NumGoroutine()))
}

func StartResourceMonitor(ctx context.Context) {
	UpTime.Set(0)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case <-ticker.C:
				RecordResourceMetrics()
				UpTime.Set(time.Since(start).Seconds())
			case <-ctx.Done():
				return
			}
		}
	}()
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func normalizePath(path string) string {
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	for i, p := range parts {
		if len(p) == 36 && strings.Count(p, "-") == 4 {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func HTTPREDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		duration := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		HTTPRequestDuration.WithLabelValues(r.Method, path, fmt.Sprintf("%d", sw.status/100*100)).Observe(duration)
		if sw.status >= 500 {
			ErrorCounter.WithLabelValues(r.Method, path).Inc()
		}
	})
}
