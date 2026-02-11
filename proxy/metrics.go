package main

import "github.com/prometheus/client_golang/prometheus"

// Metrics defines our Prometheus metrics
type Metrics struct {
	requestCount        *prometheus.CounterVec
	requestDuration     *prometheus.HistogramVec
	backendUpGauge      *prometheus.GaugeVec
	activeConnections   *prometheus.GaugeVec
	backendResponseTime *prometheus.HistogramVec
	backendErrors       *prometheus.CounterVec
}
