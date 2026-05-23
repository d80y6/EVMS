package common

import (
	"net/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// StartMetricsServer starts a Prometheus metrics endpoint on the given port
func StartMetricsServer(addr string) {
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(addr, nil)
}
