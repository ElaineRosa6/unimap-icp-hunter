package performance

import (
	"sync"
	"time"
)

// PerformanceOptimizer 性能优化器
type PerformanceOptimizer struct {
	cache         *CacheManager
	concurrency   *ConcurrencyManager
	metrics       *PerformanceMetrics
	mu            sync.RWMutex
}

// NewPerformanceOptimizer 创建性能优化器
func NewPerformanceOptimizer() *PerformanceOptimizer {
	return &PerformanceOptimizer{
		cache:       NewCacheManager(),
		concurrency: NewConcurrencyManager(),
		metrics:     NewPerformanceMetrics(),
	}
}

// Optimize 执行性能优化
func (po *PerformanceOptimizer) Optimize(siteURL string, content string) (string, error) {
	start := time.Now()
	
	// 检查缓存
	cached, found := po.cache.Get(siteURL)
	if found {
		po.metrics.RecordCacheHit(siteURL, time.Since(start))
		return cached, nil
	}
	
	// 设置并发限制
	po.concurrency.Acquire()
	defer po.concurrency.Release()
	
	// 这里可以添加实际的优化逻辑
	
	duration := time.Since(start)
	po.metrics.RecordOptimization(siteURL, duration)
	
	// 设置缓存
	po.cache.Set(siteURL, content, time.Minute*5)
	
	return content, nil
}

// GetCacheManager 获取缓存管理器
func (po *PerformanceOptimizer) GetCacheManager() *CacheManager {
	return po.cache
}

// GetConcurrencyManager 获取并发管理器
func (po *PerformanceOptimizer) GetConcurrencyManager() *ConcurrencyManager {
	return po.concurrency
}

// GetPerformanceMetrics 获取性能指标
func (po *PerformanceOptimizer) GetPerformanceMetrics() *PerformanceMetrics {
	return po.metrics
}

// CacheManager 缓存管理器
type CacheManager struct {
	cache      map[string]*CacheItem
	mu         sync.RWMutex
	maxSize    int
	cleanupInterval time.Duration
}

// CacheItem 缓存项
type CacheItem struct {
	Content    string
	Timestamp  time.Time
	AccessTime time.Time
	AccessCount int
}

// NewCacheManager 创建缓存管理器
func NewCacheManager() *CacheManager {
	cm := &CacheManager{
		cache:           make(map[string]*CacheItem),
		maxSize:         1000,
		cleanupInterval: time.Minute * 10,
	}
	
	// 启动定期清理
	go cm.startCleanup()
	
	return cm
}

// Get 获取缓存
func (cm *CacheManager) Get(key string) (string, bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	item, found := cm.cache[key]
	if !found {
		return "", false
	}
	
	// 更新访问时间和计数
	item.AccessTime = time.Now()
	item.AccessCount++
	
	return item.Content, true
}

// Set 设置缓存
func (cm *CacheManager) Set(key string, value string, ttl time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	// 检查缓存大小
	if len(cm.cache) >= cm.maxSize {
		cm.evictLRU()
	}
	
	cm.cache[key] = &CacheItem{
		Content:    value,
		Timestamp:  time.Now(),
		AccessTime: time.Now(),
		AccessCount: 1,
	}
}

// Delete 删除缓存
func (cm *CacheManager) Delete(key string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	delete(cm.cache, key)
}

// Clear 清空缓存
func (cm *CacheManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	cm.cache = make(map[string]*CacheItem)
}

// evictLRU 按照LRU策略删除缓存
func (cm *CacheManager) evictLRU() {
	if len(cm.cache) == 0 {
		return
	}
	
	var oldestKey string
	var oldestTime time.Time
	
	for key, item := range cm.cache {
		if oldestKey == "" || item.AccessTime.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.AccessTime
		}
	}
	
	delete(cm.cache, oldestKey)
}

// evictLFU 按照LFU策略删除缓存
func (cm *CacheManager) evictLFU() {
	if len(cm.cache) == 0 {
		return
	}
	
	var leastKey string
	var leastCount int
	
	for key, item := range cm.cache {
		if leastKey == "" || item.AccessCount< leastCount {
			leastKey = key
			leastCount = item.AccessCount
		}
	}
	
	delete(cm.cache, leastKey)
}

// startCleanup 启动定期清理
func (cm *CacheManager) startCleanup() {
	ticker := time.NewTicker(cm.cleanupInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		cm.cleanup()
	}
}

// cleanup 清理过期缓存
func (cm *CacheManager) cleanup() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	now := time.Now()
	for key, item := range cm.cache {
		// 清理超过1小时未访问的缓存
		if now.Sub(item.AccessTime) > time.Hour {
			delete(cm.cache, key)
		}
	}
}

// GetStats 获取缓存统计信息
func (cm *CacheManager) GetStats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	totalItems := len(cm.cache)
	totalAccess := 0
	
	for _, item := range cm.cache {
		totalAccess += item.AccessCount
	}
	
	return map[string]interface{}{
		"total_items": totalItems,
		"total_access": totalAccess,
		"max_size": cm.maxSize,
	}
}

// ConcurrencyManager 并发管理器
type ConcurrencyManager struct {
	semaphore    chan struct{}
	maxConcurrent int
}

// NewConcurrencyManager 创建并发管理器
func NewConcurrencyManager() *ConcurrencyManager {
	maxConcurrent := 10 // 默认最大并发数
	return &ConcurrencyManager{
		semaphore:     make(chan struct{}, maxConcurrent),
		maxConcurrent: maxConcurrent,
	}
}

// Acquire 获取并发许可
func (cm *ConcurrencyManager) Acquire() {
	cm.semaphore<- struct{}{}
}

// Release 释放并发许可
func (cm *ConcurrencyManager) Release() {
	<-cm.semaphore
}

// SetMaxConcurrent 设置最大并发数
func (cm *ConcurrencyManager) SetMaxConcurrent(max int) {
	if max <= 0 {
		return
	}
	
	cm.maxConcurrent = max
	cm.semaphore = make(chan struct{}, max)
}

// GetCurrentConcurrent 获取当前并发数
func (cm *ConcurrencyManager) GetCurrentConcurrent() int {
	return len(cm.semaphore)
}

// GetMaxConcurrent 获取最大并发数
func (cm *ConcurrencyManager) GetMaxConcurrent() int {
	return cm.maxConcurrent
}

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	optimizations map[string]*OptimizationMetrics
	cacheHits     int
	cacheMisses   int
	totalTime     time.Duration
	totalRequests int
	mu            sync.RWMutex
}

// OptimizationMetrics 优化指标
type OptimizationMetrics struct {
	Count     int
	TotalTime time.Duration
	AvgTime   time.Duration
	MinTime   time.Duration
	MaxTime   time.Duration
}

// NewPerformanceMetrics 创建性能指标
func NewPerformanceMetrics() *PerformanceMetrics {
	return &PerformanceMetrics{
		optimizations: make(map[string]*OptimizationMetrics),
	}
}

// RecordOptimization 记录优化性能
func (pm *PerformanceMetrics) RecordOptimization(siteURL string, duration time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.totalRequests++
	pm.totalTime += duration
	
	metrics, exists := pm.optimizations[siteURL]
	if !exists {
		metrics = &OptimizationMetrics{
			Count:     0,
			TotalTime: 0,
			AvgTime:   0,
			MinTime:   duration,
			MaxTime:   duration,
		}
		pm.optimizations[siteURL] = metrics
	}
	
	metrics.Count++
	metrics.TotalTime += duration
	metrics.AvgTime = metrics.TotalTime / time.Duration(metrics.Count)
	
	if duration< metrics.MinTime {
		metrics.MinTime = duration
	}
	if duration >metrics.MaxTime {
		metrics.MaxTime = duration
	}
}

// RecordCacheHit 记录缓存命中
func (pm *PerformanceMetrics) RecordCacheHit(siteURL string, duration time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.cacheHits++
	pm.totalRequests++
	pm.totalTime += duration
}

// RecordCacheMiss 记录缓存未命中
func (pm *PerformanceMetrics) RecordCacheMiss(siteURL string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.cacheMisses++
}

// GetStats 获取性能统计信息
func (pm *PerformanceMetrics) GetStats() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	avgTime := time.Duration(0)
	if pm.totalRequests > 0 {
		avgTime = pm.totalTime / time.Duration(pm.totalRequests)
	}
	
	cacheHitRate := 0.0
	if pm.cacheHits+pm.cacheMisses > 0 {
		cacheHitRate = float64(pm.cacheHits) / float64(pm.cacheHits+pm.cacheMisses)
	}
	
	return map[string]interface{}{
		"total_requests":    pm.totalRequests,
		"total_time":        pm.totalTime.String(),
		"average_time":      avgTime.String(),
		"cache_hits":        pm.cacheHits,
		"cache_misses":      pm.cacheMisses,
		"cache_hit_rate":    cacheHitRate,
		"optimization_count": len(pm.optimizations),
	}
}

// GetSiteStats 获取站点性能统计
func (pm *PerformanceMetrics) GetSiteStats(siteURL string) map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	metrics, exists := pm.optimizations[siteURL]
	if !exists {
		return nil
	}
	
	return map[string]interface{}{
		"count":      metrics.Count,
		"total_time": metrics.TotalTime.String(),
		"avg_time":   metrics.AvgTime.String(),
		"min_time":   metrics.MinTime.String(),
		"max_time":   metrics.MaxTime.String(),
	}
}

// ResetStats 重置统计信息
func (pm *PerformanceMetrics) ResetStats() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.optimizations = make(map[string]*OptimizationMetrics)
	pm.cacheHits = 0
	pm.cacheMisses = 0
	pm.totalTime = 0
	pm.totalRequests = 0
}