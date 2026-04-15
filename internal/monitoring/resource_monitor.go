package monitoring

import (
	"runtime"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// ResourceStats 资源使用统计
type ResourceStats struct {
	// CPU使用情况
	CPUUsage float64 `json:"cpu_usage"`

	// 内存使用情况
	MemoryUsage struct {
		Total   uint64  `json:"total"`
		Used    uint64  `json:"used"`
		Free    uint64  `json:"free"`
		Percent float64 `json:"percent"`
	} `json:"memory_usage"`

	// Goroutine数量
	GoroutineCount int `json:"goroutine_count"`

	// 文件描述符数量
	FileDescriptorCount int `json:"file_descriptor_count"`

	// 网络连接数量
	NetworkConnectionCount int `json:"network_connection_count"`

	// 响应时间统计
	ResponseTimeStats ResponseTimeStats `json:"response_time_stats"`

	// 自定义监控指标
	CustomMetrics []CustomMetric `json:"custom_metrics"`

	// 资源池统计
	PoolStats map[string]PoolStats `json:"pool_stats"`

	// 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// PoolStats 资源池统计
type PoolStats struct {
	Name           string `json:"name"`
	MaxSize        int    `json:"max_size"`
	MinSize        int    `json:"min_size"`
	Available      int    `json:"available"`
	InUse          int    `json:"in_use"`
	Total          int    `json:"total"`
	TotalCreated   int64  `json:"total_created"`
	TotalDestroyed int64  `json:"total_destroyed"`
	TotalAcquired  int64  `json:"total_acquired"`
	TotalReleased  int64  `json:"total_released"`
	TotalErrors    int64  `json:"total_errors"`
}

// ResponseTimeStats 响应时间统计
type ResponseTimeStats struct {
	// 请求计数
	TotalRequests      int64 `json:"total_requests"`
	SuccessfulRequests int64 `json:"successful_requests"`
	FailedRequests     int64 `json:"failed_requests"`

	// 响应时间统计（毫秒）
	MinResponseTime float64 `json:"min_response_time"`
	MaxResponseTime float64 `json:"max_response_time"`
	AvgResponseTime float64 `json:"avg_response_time"`
	P90ResponseTime float64 `json:"p90_response_time"`
	P95ResponseTime float64 `json:"p95_response_time"`
	P99ResponseTime float64 `json:"p99_response_time"`

	// 错误率
	ErrorRate float64 `json:"error_rate"`

	// 按类型统计
	TypeStats map[string]ResponseTimeTypeStats `json:"type_stats"`
}

// ResponseTimeTypeStats 按类型的响应时间统计
type ResponseTimeTypeStats struct {
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	AvgResponseTime    float64 `json:"avg_response_time"`
	ErrorRate          float64 `json:"error_rate"`
}

// CustomMetric 自定义监控指标
type CustomMetric struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // counter, gauge, histogram, summary
	Value       interface{}       `json:"value"`
	Labels      map[string]string `json:"labels"`
	Timestamp   time.Time         `json:"timestamp"`
	Description string            `json:"description,omitempty"`
}

// ResourceMonitor 资源监控器
type ResourceMonitor struct {
	mutex           sync.RWMutex
	poolStats       map[string]PoolStats
	statsHistory    []ResourceStats
	maxHistorySize  int
	monitorInterval time.Duration
	stopChan        chan struct{}

	// 响应时间统计
	responseTimeStats      ResponseTimeStats
	responseTimeHistory    []float64
	maxResponseTimeHistory int

	// 自定义监控指标
	customMetrics map[string]*CustomMetric
}

// NewResourceMonitor 创建资源监控器
func NewResourceMonitor(interval time.Duration) *ResourceMonitor {
	if interval <= 0 {
		interval = 10 * time.Second
	}

	return &ResourceMonitor{
		poolStats:       make(map[string]PoolStats),
		statsHistory:    make([]ResourceStats, 0),
		maxHistorySize:  100,
		monitorInterval: interval,
		stopChan:        make(chan struct{}),
		responseTimeStats: ResponseTimeStats{
			TypeStats: make(map[string]ResponseTimeTypeStats),
		},
		responseTimeHistory:    make([]float64, 0),
		maxResponseTimeHistory: 1000,
		customMetrics:          make(map[string]*CustomMetric),
	}
}

// Start 启动资源监控
func (m *ResourceMonitor) Start() {
	go func() {
		ticker := time.NewTicker(m.monitorInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.collectStats()
			case <-m.stopChan:
				return
			}
		}
	}()

	logger.Info("Resource monitor started")
}

// Stop 停止资源监控
func (m *ResourceMonitor) Stop() {
	close(m.stopChan)
	logger.Info("Resource monitor stopped")
}

// UpdatePoolStats 更新资源池统计
func (m *ResourceMonitor) UpdatePoolStats(name string, stats PoolStats) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.poolStats[name] = stats
}

// GetCurrentStats 获取当前资源统计
func (m *ResourceMonitor) GetCurrentStats() ResourceStats {
	return m.collectCurrentStats()
}

// GetStatsHistory 获取统计历史
func (m *ResourceMonitor) GetStatsHistory() []ResourceStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// 返回副本，避免并发修改
	history := make([]ResourceStats, len(m.statsHistory))
	copy(history, m.statsHistory)
	return history
}

// collectStats 收集资源统计
func (m *ResourceMonitor) collectStats() {
	stats := m.collectCurrentStats()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.statsHistory = append(m.statsHistory, stats)

	// 限制历史记录数量
	if len(m.statsHistory) > m.maxHistorySize {
		m.statsHistory = m.statsHistory[len(m.statsHistory)-m.maxHistorySize:]
	}
}

// collectCurrentStats 收集当前资源统计
func (m *ResourceMonitor) collectCurrentStats() ResourceStats {
	var mstats runtime.MemStats
	runtime.ReadMemStats(&mstats)

	stats := ResourceStats{
		CPUUsage:       0, // 需要外部收集
		GoroutineCount: runtime.NumGoroutine(),
		Timestamp:      time.Now(),
		PoolStats:      make(map[string]PoolStats),
	}

	// 内存使用情况
	stats.MemoryUsage.Total = mstats.Sys
	stats.MemoryUsage.Used = mstats.Alloc
	stats.MemoryUsage.Free = mstats.Sys - mstats.Alloc
	if mstats.Sys > 0 {
		stats.MemoryUsage.Percent = float64(mstats.Alloc) / float64(mstats.Sys) * 100
	}

	// 复制资源池统计
	m.mutex.RLock()
	for name, poolStat := range m.poolStats {
		stats.PoolStats[name] = poolStat
	}

	// 复制响应时间统计
	stats.ResponseTimeStats = m.responseTimeStats

	// 复制自定义监控指标
	stats.CustomMetrics = make([]CustomMetric, 0, len(m.customMetrics))
	for _, metric := range m.customMetrics {
		stats.CustomMetrics = append(stats.CustomMetrics, *metric)
	}
	m.mutex.RUnlock()

	return stats
}

// GetHighWaterMark 获取资源使用高水位标记
func (m *ResourceMonitor) GetHighWaterMark() map[string]float64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	highWaterMark := make(map[string]float64)

	if len(m.statsHistory) == 0 {
		return highWaterMark
	}

	// 计算内存使用高水位
	var maxMemoryPercent float64
	for _, stats := range m.statsHistory {
		if stats.MemoryUsage.Percent > maxMemoryPercent {
			maxMemoryPercent = stats.MemoryUsage.Percent
		}
	}
	highWaterMark["memory"] = maxMemoryPercent

	// 计算Goroutine高水位
	var maxGoroutineCount int
	for _, stats := range m.statsHistory {
		if stats.GoroutineCount > maxGoroutineCount {
			maxGoroutineCount = stats.GoroutineCount
		}
	}
	highWaterMark["goroutine"] = float64(maxGoroutineCount)

	return highWaterMark
}

// CheckResourceUsage 检查资源使用情况
func (m *ResourceMonitor) CheckResourceUsage(thresholds map[string]float64) map[string]bool {
	stats := m.GetCurrentStats()

	alerts := make(map[string]bool)

	// 检查内存使用
	if memoryThreshold, ok := thresholds["memory"]; ok {
		alerts["memory"] = stats.MemoryUsage.Percent > memoryThreshold
	}

	// 检查Goroutine数量
	if goroutineThreshold, ok := thresholds["goroutine"]; ok {
		alerts["goroutine"] = float64(stats.GoroutineCount) > goroutineThreshold
	}

	// 检查资源池使用
	for poolName, poolStat := range stats.PoolStats {
		if poolThreshold, ok := thresholds["pool_"+poolName]; ok {
			usagePercent := float64(poolStat.InUse) / float64(poolStat.MaxSize) * 100
			alerts["pool_"+poolName] = usagePercent > poolThreshold
		}
	}

	return alerts
}

// GetResourceReport 获取资源使用报告
func (m *ResourceMonitor) GetResourceReport() map[string]interface{} {
	stats := m.GetCurrentStats()
	highWaterMark := m.GetHighWaterMark()

	report := map[string]interface{}{
		"current": map[string]interface{}{
			"cpu_usage": stats.CPUUsage,
			"memory": map[string]interface{}{
				"used":    stats.MemoryUsage.Used,
				"total":   stats.MemoryUsage.Total,
				"percent": stats.MemoryUsage.Percent,
			},
			"goroutines": stats.GoroutineCount,
			"timestamp":  stats.Timestamp,
		},
		"high_water_mark": highWaterMark,
		"pool_stats":      stats.PoolStats,
		"history_length":  len(m.statsHistory),
	}

	return report
}

// RecordResponseTime 记录响应时间（毫秒）
func (m *ResourceMonitor) RecordResponseTime(responseTimeMs float64, requestType string, success bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 更新总请求计数
	m.responseTimeStats.TotalRequests++
	if success {
		m.responseTimeStats.SuccessfulRequests++
	} else {
		m.responseTimeStats.FailedRequests++
	}

	// 更新响应时间历史
	m.responseTimeHistory = append(m.responseTimeHistory, responseTimeMs)
	if len(m.responseTimeHistory) > m.maxResponseTimeHistory {
		m.responseTimeHistory = m.responseTimeHistory[len(m.responseTimeHistory)-m.maxResponseTimeHistory:]
	}

	// 更新统计数据
	m.updateResponseTimeStats(responseTimeMs)

	// 更新类型统计
	if requestType != "" {
		typeStats, exists := m.responseTimeStats.TypeStats[requestType]
		if !exists {
			typeStats = ResponseTimeTypeStats{}
		}

		typeStats.TotalRequests++
		if success {
			typeStats.SuccessfulRequests++
		} else {
			typeStats.FailedRequests++
		}

		// 更新类型平均响应时间
		totalTime := typeStats.AvgResponseTime * float64(typeStats.TotalRequests-1)
		typeStats.AvgResponseTime = (totalTime + responseTimeMs) / float64(typeStats.TotalRequests)

		// 更新错误率
		if typeStats.TotalRequests > 0 {
			typeStats.ErrorRate = float64(typeStats.FailedRequests) / float64(typeStats.TotalRequests)
		}

		m.responseTimeStats.TypeStats[requestType] = typeStats
	}
}

// updateResponseTimeStats 更新响应时间统计数据
func (m *ResourceMonitor) updateResponseTimeStats(responseTimeMs float64) {
	// 更新最小响应时间
	if m.responseTimeStats.MinResponseTime == 0 || responseTimeMs < m.responseTimeStats.MinResponseTime {
		m.responseTimeStats.MinResponseTime = responseTimeMs
	}

	// 更新最大响应时间
	if responseTimeMs > m.responseTimeStats.MaxResponseTime {
		m.responseTimeStats.MaxResponseTime = responseTimeMs
	}

	// 更新平均响应时间
	totalTime := m.responseTimeStats.AvgResponseTime * float64(m.responseTimeStats.TotalRequests-1)
	m.responseTimeStats.AvgResponseTime = (totalTime + responseTimeMs) / float64(m.responseTimeStats.TotalRequests)

	// 更新错误率
	if m.responseTimeStats.TotalRequests > 0 {
		m.responseTimeStats.ErrorRate = float64(m.responseTimeStats.FailedRequests) / float64(m.responseTimeStats.TotalRequests)
	}

	// 更新百分位数
	m.updatePercentiles()
}

// updatePercentiles 更新响应时间百分位数
func (m *ResourceMonitor) updatePercentiles() {
	if len(m.responseTimeHistory) == 0 {
		return
	}

	// 创建副本并排序
	sortedTimes := make([]float64, len(m.responseTimeHistory))
	copy(sortedTimes, m.responseTimeHistory)
	for i := 0; i < len(sortedTimes); i++ {
		for j := i + 1; j < len(sortedTimes); j++ {
			if sortedTimes[i] > sortedTimes[j] {
				sortedTimes[i], sortedTimes[j] = sortedTimes[j], sortedTimes[i]
			}
		}
	}

	// 计算百分位数
	count := len(sortedTimes)
	m.responseTimeStats.P90ResponseTime = sortedTimes[int(float64(count)*0.9)]
	m.responseTimeStats.P95ResponseTime = sortedTimes[int(float64(count)*0.95)]
	m.responseTimeStats.P99ResponseTime = sortedTimes[int(float64(count)*0.99)]
}

// RegisterPool 注册资源池
func (m *ResourceMonitor) RegisterPool(name string, maxSize, minSize int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.poolStats[name] = PoolStats{
		Name:           name,
		MaxSize:        maxSize,
		MinSize:        minSize,
		Available:      minSize,
		InUse:          0,
		Total:          minSize,
		TotalCreated:   int64(minSize),
		TotalDestroyed: 0,
		TotalAcquired:  0,
		TotalReleased:  0,
		TotalErrors:    0,
	}
}

// UnregisterPool 注销资源池
func (m *ResourceMonitor) UnregisterPool(name string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.poolStats, name)
}

// RecordCustomMetric 记录自定义监控指标
func (m *ResourceMonitor) RecordCustomMetric(name, metricType string, value interface{}, labels map[string]string, description string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := name
	if labels != nil && len(labels) > 0 {
		for k, v := range labels {
			key += ":" + k + "=" + v
		}
	}

	metric, exists := m.customMetrics[key]
	if !exists {
		metric = &CustomMetric{
			Name:        name,
			Type:        metricType,
			Labels:      make(map[string]string),
			Description: description,
		}
	}

	// 更新标签
	if labels != nil {
		for k, v := range labels {
			metric.Labels[k] = v
		}
	}

	// 更新值和时间戳
	metric.Value = value
	metric.Timestamp = time.Now()

	m.customMetrics[key] = metric
}

// GetCustomMetric 获取自定义监控指标
func (m *ResourceMonitor) GetCustomMetric(name string, labels map[string]string) (*CustomMetric, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	key := name
	if labels != nil && len(labels) > 0 {
		for k, v := range labels {
			key += ":" + k + "=" + v
		}
	}

	metric, exists := m.customMetrics[key]
	return metric, exists
}

// ListCustomMetrics 列出所有自定义监控指标
func (m *ResourceMonitor) ListCustomMetrics() []*CustomMetric {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var metrics []*CustomMetric
	for _, metric := range m.customMetrics {
		metrics = append(metrics, metric)
	}

	return metrics
}

// DeleteCustomMetric 删除自定义监控指标
func (m *ResourceMonitor) DeleteCustomMetric(name string, labels map[string]string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := name
	if labels != nil && len(labels) > 0 {
		for k, v := range labels {
			key += ":" + k + "=" + v
		}
	}

	delete(m.customMetrics, key)
}
