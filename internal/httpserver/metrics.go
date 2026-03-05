package httpserver

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type serverMetrics struct {
	requestTotal    *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
}

func newServerMetrics(registry *prometheus.Registry) *serverMetrics {
	requestTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "flow_http_requests_total",
			Help: "Total number of HTTP requests handled by Flow.",
		},
		[]string{"method", "route", "status"},
	)
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "flow_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds for Flow.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)

	registry.MustRegister(requestTotal, requestDuration)

	return &serverMetrics{
		requestTotal:    requestTotal,
		requestDuration: requestDuration,
	}
}

func (m *serverMetrics) observe(method string, route string, status int, duration time.Duration) {
	statusLabel := strconv.Itoa(status)
	m.requestTotal.WithLabelValues(method, route, statusLabel).Inc()
	m.requestDuration.WithLabelValues(method, route, statusLabel).Observe(duration.Seconds())
}
