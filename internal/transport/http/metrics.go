package http

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	totalRequests *prometheus.CounterVec
	totalDuration *prometheus.HistogramVec
}

func (m *metrics) observeRequest(method, path, code string, duration float64) {
	m.totalRequests.WithLabelValues(method, path, code).Inc()
	m.totalDuration.WithLabelValues(method, path, code).Observe(duration)
}

func setupMetrics(reg prometheus.Registerer) *metrics {
	metricLabels := []string{"method", "path", "code"}

	m := new(metrics)
	m.totalRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		metricLabels,
	)
	m.totalDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Request duration",
			Buckets: prometheus.DefBuckets,
		},
		metricLabels,
	)

	reg.MustRegister(m.totalRequests)
	reg.MustRegister(m.totalDuration)

	return m
}
