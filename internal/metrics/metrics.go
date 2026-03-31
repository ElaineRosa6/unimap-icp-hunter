package metrics

import (
	"runtime"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// 自定义时间分位桶
var (
	// HTTP请求时间桶：100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
	httpDurationBuckets = []float64{0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}
	// 查询时间桶：500ms, 1s, 2s, 5s, 10s, 30s, 60s
	queryDurationBuckets = []float64{0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0}
	// 截图时间桶：1s, 2s, 5s, 10s, 20s, 30s, 60s
	screenshotDurationBuckets = []float64{1.0, 2.0, 5.0, 10.0, 20.0, 30.0, 60.0}
)

var (
	// HTTP 指标
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
			Buckets: httpDurationBuckets,
		},
		[]string{"path", "method", "status"},
	)

	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "unimap_http_requests_in_flight",
			Help: "Current number of in-flight HTTP requests.",
		},
	)

	// 限流指标
	rateLimitRejectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_rate_limit_rejected_total",
			Help: "Total number of requests rejected by rate limiting.",
		},
		[]string{"path"},
	)

	// 查询指标
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
			Buckets: queryDurationBuckets,
		},
		[]string{"status"},
	)

	// 按引擎细分的查询指标
	engineQueryTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_engine_query_total",
			Help: "Total queries per engine by status.",
		},
		[]string{"engine", "status"},
	)

	engineQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "unimap_engine_query_duration_seconds",
			Help:    "Query duration per engine.",
			Buckets: queryDurationBuckets,
		},
		[]string{"engine"},
	)

	// 缓存指标
	cacheLookups = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_cache_lookups_total",
			Help: "Cache lookup counts by backend and result.",
		},
		[]string{"backend", "result"},
	)

	cacheSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unimap_cache_size",
			Help: "Current cache size by backend.",
		},
		[]string{"backend"},
	)

	cacheHitRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unimap_cache_hit_rate",
			Help: "Cache hit rate by backend.",
		},
		[]string{"backend"},
	)

	// 引擎错误指标
	engineErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_engine_errors_total",
			Help: "Total number of engine-related errors by engine.",
		},
		[]string{"engine"},
	)

	// 篡改检测指标
	tamperChecksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_tamper_checks_total",
			Help: "Total number of tamper checks by status.",
		},
		[]string{"status"},
	)

	tamperCheckDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "unimap_tamper_check_duration_seconds",
			Help:    "Tamper check duration.",
			Buckets: httpDurationBuckets,
		},
		[]string{"status"},
	)

	// 截图指标
	screenshotRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_screenshot_requests_total",
			Help: "Total screenshot requests by type and status.",
		},
		[]string{"type", "status"},
	)

	screenshotDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "unimap_screenshot_duration_seconds",
			Help:    "Screenshot capture duration.",
			Buckets: screenshotDurationBuckets,
		},
		[]string{"type"},
	)

	screenshotBatchSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "unimap_screenshot_batch_size",
			Help:    "Screenshot batch size distribution.",
			Buckets: []float64{1, 5, 10, 20, 50, 100},
		},
	)

	// WebSocket 指标
	websocketConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "unimap_websocket_connections",
			Help: "Current number of active WebSocket connections.",
		},
	)

	websocketMessagesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_websocket_messages_total",
			Help: "Total WebSocket messages by direction.",
		},
		[]string{"direction"}, // inbound, outbound
	)

	// 资源使用指标
	goroutinesCount = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "unimap_goroutines_count",
			Help: "Current number of goroutines.",
		},
		func() float64 {
			return float64(runtime.NumGoroutine())
		},
	)

	memoryAllocMB = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "unimap_memory_alloc_mb",
			Help: "Current memory allocation in MB.",
		},
	)

	memorySysMB = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "unimap_memory_sys_mb",
			Help: "Total memory obtained from OS in MB.",
		},
	)

	// 批量操作指标
	batchOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_batch_operations_total",
			Help: "Total batch operations by type.",
		},
		[]string{"type"}, // screenshot, tamper, query
	)

	batchOperationSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "unimap_batch_operation_size",
			Help:    "Batch operation size distribution.",
			Buckets: []float64{1, 5, 10, 20, 50, 100},
		},
		[]string{"type"},
	)

	// Bridge 可观测指标
	bridgeRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_screenshot_bridge_requests_total",
			Help: "Total number of bridge screenshot requests by engine and status.",
		},
		[]string{"engine", "status"},
	)

	bridgeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "unimap_screenshot_bridge_duration_seconds",
			Help:    "Bridge screenshot request duration.",
			Buckets: screenshotDurationBuckets,
		},
		[]string{"engine"},
	)

	bridgeRetriesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "unimap_screenshot_bridge_retries_total",
			Help: "Total number of bridge retry attempts.",
		},
	)

	bridgeTimeoutsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "unimap_screenshot_bridge_timeouts_total",
			Help: "Total number of bridge timeout events.",
		},
	)

	bridgeFallbackTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unimap_screenshot_bridge_fallback_total",
			Help: "Total number of extension-to-cdp fallback events by reason.",
		},
		[]string{"reason"},
	)
)

func init() {
	// HTTP 指标
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(httpRequestsInFlight)

	// 限流指标
	prometheus.MustRegister(rateLimitRejectedTotal)

	// 查询指标
	prometheus.MustRegister(queryRequestsTotal)
	prometheus.MustRegister(queryDuration)
	prometheus.MustRegister(engineQueryTotal)
	prometheus.MustRegister(engineQueryDuration)

	// 缓存指标
	prometheus.MustRegister(cacheLookups)
	prometheus.MustRegister(cacheSize)
	prometheus.MustRegister(cacheHitRate)

	// 引擎错误指标
	prometheus.MustRegister(engineErrorsTotal)

	// 篡改检测指标
	prometheus.MustRegister(tamperChecksTotal)
	prometheus.MustRegister(tamperCheckDuration)

	// 截图指标
	prometheus.MustRegister(screenshotRequestsTotal)
	prometheus.MustRegister(screenshotDuration)
	prometheus.MustRegister(screenshotBatchSize)

	// WebSocket 指标
	prometheus.MustRegister(websocketConnections)
	prometheus.MustRegister(websocketMessagesTotal)

	// 资源使用指标
	prometheus.MustRegister(goroutinesCount)
	prometheus.MustRegister(memoryAllocMB)
	prometheus.MustRegister(memorySysMB)

	// 批量操作指标
	prometheus.MustRegister(batchOperationsTotal)
	prometheus.MustRegister(batchOperationSize)

	// Bridge 指标
	prometheus.MustRegister(bridgeRequestsTotal)
	prometheus.MustRegister(bridgeDuration)
	prometheus.MustRegister(bridgeRetriesTotal)
	prometheus.MustRegister(bridgeTimeoutsTotal)
	prometheus.MustRegister(bridgeFallbackTotal)
}

// HTTP 指标函数
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

// 限流指标函数
func IncRateLimitRejected(path string) {
	rateLimitRejectedTotal.WithLabelValues(path).Inc()
}

// 查询指标函数
func IncQueryRequest(status string) {
	queryRequestsTotal.WithLabelValues(status).Inc()
}

func ObserveQueryDuration(status string, duration time.Duration) {
	queryDuration.WithLabelValues(status).Observe(duration.Seconds())
}

// 引擎查询指标函数
func IncEngineQuery(engine, status string) {
	if engine == "" {
		engine = "unknown"
	}
	engineQueryTotal.WithLabelValues(engine, status).Inc()
}

func ObserveEngineQueryDuration(engine string, duration time.Duration) {
	if engine == "" {
		engine = "unknown"
	}
	engineQueryDuration.WithLabelValues(engine).Observe(duration.Seconds())
}

// 缓存指标函数
func ObserveCacheLookup(backend, result string) {
	if backend == "" {
		backend = "memory"
	}
	if result == "" {
		result = "miss"
	}
	cacheLookups.WithLabelValues(backend, result).Inc()
}

func UpdateCacheStats(backend string, size int, hitRate float64) {
	if backend == "" {
		backend = "memory"
	}
	cacheSize.WithLabelValues(backend).Set(float64(size))
	cacheHitRate.WithLabelValues(backend).Set(hitRate)
}

// 引擎错误指标函数
func IncEngineError() {
	engineErrorsTotal.WithLabelValues("unknown").Inc()
}

func IncEngineErrorByName(engine string) {
	if engine == "" {
		engine = "unknown"
	}
	engineErrorsTotal.WithLabelValues(engine).Inc()
}

// 篡改检测指标函数
func IncTamperCheck(status string) {
	if status == "" {
		status = "unknown"
	}
	tamperChecksTotal.WithLabelValues(status).Inc()
}

func ObserveTamperCheckDuration(status string, duration time.Duration) {
	if status == "" {
		status = "unknown"
	}
	tamperCheckDuration.WithLabelValues(status).Observe(duration.Seconds())
}

// 截图指标函数
func IncScreenshotRequest(screenshotType, status string) {
	if screenshotType == "" {
		screenshotType = "single"
	}
	screenshotRequestsTotal.WithLabelValues(screenshotType, status).Inc()
}

func ObserveScreenshotDuration(screenshotType string, duration time.Duration) {
	if screenshotType == "" {
		screenshotType = "single"
	}
	screenshotDuration.WithLabelValues(screenshotType).Observe(duration.Seconds())
}

func ObserveScreenshotBatchSize(size int) {
	screenshotBatchSize.Observe(float64(size))
}

// WebSocket 指标函数
func IncWebSocketConnection() {
	websocketConnections.Inc()
}

func DecWebSocketConnection() {
	websocketConnections.Dec()
}

func IncWebSocketMessage(direction string) {
	websocketMessagesTotal.WithLabelValues(direction).Inc()
}

// 资源使用指标函数
func UpdateMemoryStats() {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	memoryAllocMB.Set(float64(mem.Alloc) / 1024 / 1024)
	memorySysMB.Set(float64(mem.Sys) / 1024 / 1024)
}

// 批量操作指标函数
func IncBatchOperation(opType string) {
	batchOperationsTotal.WithLabelValues(opType).Inc()
}

func ObserveBatchOperationSize(opType string, size int) {
	batchOperationSize.WithLabelValues(opType).Observe(float64(size))
}

// Bridge 指标函数
func IncBridgeRequest(engine, status string) {
	if engine == "" {
		engine = "unknown"
	}
	if status == "" {
		status = "unknown"
	}
	bridgeRequestsTotal.WithLabelValues(engine, status).Inc()
}

func ObserveBridgeDuration(engine string, duration time.Duration) {
	if engine == "" {
		engine = "unknown"
	}
	bridgeDuration.WithLabelValues(engine).Observe(duration.Seconds())
}

func IncBridgeRetry() {
	bridgeRetriesTotal.Inc()
}

func IncBridgeTimeout() {
	bridgeTimeoutsTotal.Inc()
}

func IncBridgeFallback(reason string) {
	if reason == "" {
		reason = "unknown"
	}
	bridgeFallbackTotal.WithLabelValues(reason).Inc()
}
