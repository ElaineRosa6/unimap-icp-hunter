package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_http_requests_total",
			Help: "Total number of HTTP requests by path, method and status.",
		},
		[]string{"path", "method", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "unimap_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path", "method", "status"},
	)

	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "unimap_http_requests_in_flight",
			Help: "Current number of in-flight HTTP requests.",
		},
	)

	rateLimitRejectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_rate_limit_rejected_total",
			Help: "Total number of requests rejected by rate limiting.",
		},
		[]string{"path"},
	)

	queryRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_query_requests_total",
			Help: "Total query requests by final status.",
		},
		[]string{"status"},
	)

	queryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "unimap_query_duration_seconds",
			Help:    "Unified query execution duration.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)

	cacheLookups = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_cache_lookups_total",
			Help: "Cache lookup counts by backend and result.",
		},
		[]string{"backend", "result"},
	)

	engineErrorsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "unimap_engine_errors_total",
			Help: "Total number of engine-related errors.",
		},
	)

	tamperChecksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_tamper_checks_total",
			Help: "Total number of tamper checks by status.",
		},
		[]string{"status"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(httpRequestsInFlight)
	prometheus.MustRegister(rateLimitRejectedTotal)
	prometheus.MustRegister(queryRequestsTotal)
	prometheus.MustRegister(queryDuration)
	prometheus.MustRegister(cacheLookups)
	prometheus.MustRegister(engineErrorsTotal)
	prometheus.MustRegister(tamperChecksTotal)
}

func IncHTTPInFlight() {
	httpRequestsInFlight.Inc()
}

func DecHTTPInFlight() {
	httpRequestsInFlight.Dec()
}

func ObserveHTTPRequest(path, method string, statusCode int, duration time.Duration) {
	status := strconv.Itoa(statusCode)
	httpRequestsTotal.WithLabelValues(path, method, status).Inc()
	httpRequestDuration.WithLabelValues(path, method, status).Observe(duration.Seconds())
}

func IncRateLimitRejected(path string) {
	rateLimitRejectedTotal.WithLabelValues(path).Inc()
}

func IncQueryRequest(status string) {
	queryRequestsTotal.WithLabelValues(status).Inc()
}

func ObserveQueryDuration(status string, duration time.Duration) {
	queryDuration.WithLabelValues(status).Observe(duration.Seconds())
}

func ObserveCacheLookup(backend, result string) {
	if backend == "" {
		backend = "memory"
	}
	if result == "" {
		result = "miss"
	}
	cacheLookups.WithLabelValues(backend, result).Inc()
}

func IncEngineError() {
	engineErrorsTotal.Inc()
}

func IncTamperCheck(status string) {
	if status == "" {
		status = "unknown"
	}
	tamperChecksTotal.WithLabelValues(status).Inc()
}
