package utils

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// EngineCacheConfig 引擎缓存配置接口
type EngineCacheConfig interface {
	IsEnabled() bool
	GetTTL() time.Duration
	GetMaxSize() int
}

// SimpleEngineCacheConfig 简单的引擎缓存配置实现
type SimpleEngineCacheConfig struct {
	Enabled bool
	TTL     time.Duration
	MaxSize int
}

func (c *SimpleEngineCacheConfig) IsEnabled() bool       { return c.Enabled }
func (c *SimpleEngineCacheConfig) GetTTL() time.Duration { return c.TTL }
func (c *SimpleEngineCacheConfig) GetMaxSize() int       { return c.MaxSize }

// CacheStrategy 缓存策略接口
type CacheStrategy interface {
	// GetCacheDuration 根据查询信息获取缓存时间
	GetCacheDuration(engineName, query string, page, pageSize int) time.Duration
	// RecordQuery 记录查询信息，用于优化策略
	RecordQuery(engineName, query string, page, pageSize int, duration time.Duration, success bool)
	// GetStats 获取策略统计信息
	GetStats() CacheStrategyStats
}

// CacheStrategyStats 缓存策略统计信息
type CacheStrategyStats struct {
	TotalQueries    int           // 总查询次数
	CacheHits       int           // 缓存命中次数
	CacheMisses     int           // 缓存未命中次数
	TotalDuration   time.Duration // 总查询时间
	AverageDuration time.Duration // 平均查询时间
	LastUpdate      time.Time     // 上次更新时间
	StrategyName    string        // 策略名称
}

// DynamicCacheStrategy 动态缓存策略
type DynamicCacheStrategy struct {
	baseDuration  time.Duration
	minDuration   time.Duration
	maxDuration   time.Duration
	queryStats    map[string]queryStat
	engineStats   map[string]engineStat
	mutex         chan struct{}
	totalQueries  int
	cacheHits     int
	cacheMisses   int
	totalDuration time.Duration
	lastUpdate    time.Time
	// 新增维度统计
	queryFrequencyStats    map[string]frequencyStat
	enginePerformanceStats map[string]performanceStat
	dataVolatilityStats    map[string]volatilityStat
}

// frequencyStat 查询频率统计
type frequencyStat struct {
	count          int
	firstQuery     time.Time
	lastQuery      time.Time
	intervalSum    time.Duration
	queryIntervals []time.Duration
}

// performanceStat 引擎性能统计
type performanceStat struct {
	avgResponseTime     time.Duration
	successRate         float64
	errorRate           float64
	timeoutRate         float64
	consecutiveFailures int
}

// volatilityStat 数据波动性统计
type volatilityStat struct {
	totalChanges       int
	checkCount         int
	changeRate         float64
	lastChangeDetected time.Time
}

// engineStat 引擎统计信息
type engineStat struct {
	count         int
	totalDuration time.Duration
	successCount  int
	cacheHitRate  float64
}

// queryStat 查询统计信息
type queryStat struct {
	count         int
	totalDuration time.Duration
	successCount  int
	lastQuery     time.Time
	// 新增字段
	avgResponseTime    time.Duration
	errorCount         int
	timeoutCount       int
	cacheHitRate       float64
	lastCacheHit       time.Time
	queryFrequency     float64 // 查询频率（查询/小时）
	dataChangeDetected bool
}

// NewDynamicCacheStrategy 创建动态缓存策略
func NewDynamicCacheStrategy(baseDuration, minDuration, maxDuration time.Duration) *DynamicCacheStrategy {
	return &DynamicCacheStrategy{
		baseDuration:           baseDuration,
		minDuration:            minDuration,
		maxDuration:            maxDuration,
		queryStats:             make(map[string]queryStat),
		engineStats:            make(map[string]engineStat),
		queryFrequencyStats:    make(map[string]frequencyStat),
		enginePerformanceStats: make(map[string]performanceStat),
		dataVolatilityStats:    make(map[string]volatilityStat),
		mutex:                  make(chan struct{}, 1),
		lastUpdate:             time.Now(),
	}
}

// GetCacheDuration 根据查询信息获取缓存时间
func (s *DynamicCacheStrategy) GetCacheDuration(engineName, query string, page, pageSize int) time.Duration {
	s.lock()
	defer s.unlock()

	// 生成查询键
	queryKey := s.generateQueryKey(engineName, query, page, pageSize)

	// 检查是否有查询统计信息
	stat, exists := s.queryStats[queryKey]
	if exists {
		s.cacheHits++
	} else {
		s.cacheMisses++
	}

	// 基础缓存时间
	duration := s.baseDuration

	// 根据查询统计调整缓存时间
	if exists {
		// 如果查询频繁且成功，增加缓存时间
		if stat.count > 5 && stat.successCount > int(float64(stat.count)*0.8) {
			duration = duration * 2
		}

		// 如果查询时间较长，增加缓存时间
		if stat.avgResponseTime > 5*time.Second {
			duration = duration * 15 / 10
		}

		// 如果最近查询过，减少缓存时间
		if time.Since(stat.lastQuery) < 5*time.Minute {
			duration = duration / 2
		}

		// 根据缓存命中率调整
		if stat.cacheHitRate > 0.8 {
			duration = duration * 3 / 2
		} else if stat.cacheHitRate < 0.2 {
			duration = duration / 2
		}

		// 根据查询频率调整
		if stat.queryFrequency > 100 { // 每小时查询超过100次
			duration = duration * 3 / 2
		} else if stat.queryFrequency < 1 { // 每小时查询少于1次
			duration = duration * 3 / 2 // 低频查询可以缓存更长时间
		}

		// 如果检测到数据变化，减少缓存时间
		if stat.dataChangeDetected {
			duration = duration / 3
		}
	}

	// 根据引擎统计调整缓存时间
	engStat, engineExists := s.engineStats[engineName]
	if engineExists {
		// 如果引擎缓存命中率高，增加缓存时间
		if engStat.cacheHitRate > 0.7 {
			duration = duration * 3 / 2
		}

		// 如果引擎查询成功率低，减少缓存时间
		if engStat.successCount < int(float64(engStat.count)*0.5) {
			duration = duration / 2
		}
	}

	// 根据引擎性能调整
	perfStat, perfExists := s.enginePerformanceStats[engineName]
	if perfExists {
		// 如果引擎响应时间长，增加缓存时间
		if perfStat.avgResponseTime > 10*time.Second {
			duration = duration * 2
		}

		// 如果引擎错误率高，减少缓存时间
		if perfStat.errorRate > 0.1 {
			duration = duration / 2
		}

		// 如果有连续失败，减少缓存时间
		if perfStat.consecutiveFailures > 3 {
			duration = duration / 3
		}
	}

	// 根据数据波动性调整
	volStat, volExists := s.dataVolatilityStats[engineName]
	if volExists {
		// 如果数据变化率高，减少缓存时间
		if volStat.changeRate > 0.5 {
			duration = duration / 4
		} else if volStat.changeRate > 0.1 {
			duration = duration / 2
		}

		// 如果最近检测到变化，减少缓存时间
		if time.Since(volStat.lastChangeDetected) < 1*time.Hour {
			duration = duration / 2
		}
	}

	// 根据查询特征调整缓存时间
	duration = s.adjustByQueryFeatures(duration, query)

	// 根据查询频率统计调整
	freqStat, freqExists := s.queryFrequencyStats[queryKey]
	if freqExists {
		if freqStat.count > 10 {
			// 计算平均查询间隔
			avgInterval := freqStat.intervalSum / time.Duration(freqStat.count-1)
			// 如果查询间隔短，减少缓存时间
			if avgInterval < 5*time.Minute {
				duration = duration / 2
			} else if avgInterval > 1*time.Hour {
				duration = duration * 2
			}
		}
	}

	// 确保缓存时间在合理范围内
	if duration < s.minDuration {
		duration = s.minDuration
	}
	if duration > s.maxDuration {
		duration = s.maxDuration
	}

	return duration
}

// RecordQuery 记录查询信息
func (s *DynamicCacheStrategy) RecordQuery(engineName, query string, page, pageSize int, duration time.Duration, success bool) {
	s.lock()
	defer s.unlock()

	// 生成查询键
	queryKey := s.generateQueryKey(engineName, query, page, pageSize)

	// 更新查询统计
	stat, exists := s.queryStats[queryKey]
	if !exists {
		stat = queryStat{}
	}

	// 更新查询频率统计
	freqStat, freqExists := s.queryFrequencyStats[queryKey]
	if !freqExists {
		freqStat = frequencyStat{
			firstQuery: time.Now(),
		}
	} else {
		// 计算查询间隔
		interval := time.Since(freqStat.lastQuery)
		freqStat.intervalSum += interval
		freqStat.queryIntervals = append(freqStat.queryIntervals, interval)
		if len(freqStat.queryIntervals) > 100 {
			freqStat.queryIntervals = freqStat.queryIntervals[len(freqStat.queryIntervals)-100:]
		}
	}
	freqStat.count++
	freqStat.lastQuery = time.Now()
	s.queryFrequencyStats[queryKey] = freqStat

	// 更新查询统计
	stat.count++
	stat.totalDuration += duration
	if success {
		stat.successCount++
	} else {
		stat.errorCount++
		if duration >= s.baseDuration {
			stat.timeoutCount++
		}
	}
	stat.lastQuery = time.Now()

	// 计算平均响应时间
	if stat.count > 0 {
		stat.avgResponseTime = stat.totalDuration / time.Duration(stat.count)
	}

	// 计算缓存命中率
	totalCache := s.cacheHits + s.cacheMisses
	if totalCache > 0 {
		stat.cacheHitRate = float64(s.cacheHits) / float64(totalCache)
	}

	// 计算查询频率（查询/小时）
	if stat.count > 1 {
		hours := time.Since(stat.lastQuery).Hours()
		if hours > 0 {
			stat.queryFrequency = float64(stat.count) / hours
		}
	}

	s.queryStats[queryKey] = stat

	// 更新引擎统计
	engStat, engineExists := s.engineStats[engineName]
	if !engineExists {
		engStat = engineStat{}
	}
	engStat.count++
	engStat.totalDuration += duration
	if success {
		engStat.successCount++
	}
	// 简单的缓存命中率估算（实际应该根据缓存系统的统计）
	engStat.cacheHitRate = float64(engStat.successCount) / float64(engStat.count)
	s.engineStats[engineName] = engStat

	// 更新引擎性能统计
	perfStat, perfExists := s.enginePerformanceStats[engineName]
	if !perfExists {
		perfStat = performanceStat{}
	}
	// 更新性能统计
	if success {
		perfStat.avgResponseTime = (perfStat.avgResponseTime*time.Duration(engStat.count-1) + duration) / time.Duration(engStat.count)
		perfStat.successRate = float64(engStat.successCount) / float64(engStat.count)
		perfStat.errorRate = float64(engStat.count-engStat.successCount) / float64(engStat.count)
		perfStat.consecutiveFailures = 0
	} else {
		perfStat.consecutiveFailures++
		if duration >= s.baseDuration {
			perfStat.timeoutRate = float64(stat.timeoutCount) / float64(stat.count)
		}
	}
	s.enginePerformanceStats[engineName] = perfStat

	// 更新全局统计
	s.totalQueries++
	s.totalDuration += duration
	s.lastUpdate = time.Now()

	// 定期清理过期的查询统计
	s.cleanupOldStats()
}

// GetStats 获取策略统计信息
func (s *DynamicCacheStrategy) GetStats() CacheStrategyStats {
	s.lock()
	defer s.unlock()

	averageDuration := time.Duration(0)
	if s.totalQueries > 0 {
		averageDuration = s.totalDuration / time.Duration(s.totalQueries)
	}

	return CacheStrategyStats{
		TotalQueries:    s.totalQueries,
		CacheHits:       s.cacheHits,
		CacheMisses:     s.cacheMisses,
		TotalDuration:   s.totalDuration,
		AverageDuration: averageDuration,
		LastUpdate:      s.lastUpdate,
		StrategyName:    "DynamicCacheStrategy",
	}
}

// adjustByQueryFeatures 根据查询特征调整缓存时间
func (s *DynamicCacheStrategy) adjustByQueryFeatures(duration time.Duration, query string) time.Duration {
	// 分析查询特征
	query = strings.ToLower(query)

	// 如果查询包含时间相关的关键词，减少缓存时间
	timeKeywords := []string{"recent", "latest", "today", "yesterday", "last", "current", "now", "live"}
	for _, keyword := range timeKeywords {
		if strings.Contains(query, keyword) {
			return duration / 2
		}
	}

	// 如果查询包含静态内容关键词，增加缓存时间
	staticKeywords := []string{"static", "stable", "fixed", "permanent", "constant"}
	for _, keyword := range staticKeywords {
		if strings.Contains(query, keyword) {
			return duration * 2
		}
	}

	// 如果查询很简单，增加缓存时间
	if len(query) < 10 {
		return duration * 3 / 2
	}

	// 如果查询很复杂，减少缓存时间
	if len(query) > 100 {
		return duration / 2
	}

	return duration
}

// generateQueryKey 生成查询键
func (s *DynamicCacheStrategy) generateQueryKey(engineName, query string, page, pageSize int) string {
	// 包含分页维度，避免不同页复用同一统计键导致策略偏差
	return fmt.Sprintf("%s:%s:p%d:s%d", engineName, query, page, pageSize)
}

// cleanupOldStats 清理过期的查询统计
func (s *DynamicCacheStrategy) cleanupOldStats() {
	// 清理30天前的查询统计
	threshold := time.Now().Add(-30 * 24 * time.Hour)

	// 清理查询统计
	for key, stat := range s.queryStats {
		if stat.lastQuery.Before(threshold) {
			delete(s.queryStats, key)
		}
	}

	// 清理查询频率统计
	for key, stat := range s.queryFrequencyStats {
		if stat.lastQuery.Before(threshold) {
			delete(s.queryFrequencyStats, key)
		}
	}

	// 清理引擎性能统计（保留所有引擎的统计）
	// 清理数据波动性统计（保留所有引擎的统计）
}

// lock 加锁
func (s *DynamicCacheStrategy) lock() {
	s.mutex <- struct{}{}
}

// unlock 解锁
func (s *DynamicCacheStrategy) unlock() {
	<-s.mutex
}

// DefaultCacheStrategy 默认缓存策略
type DefaultCacheStrategy struct {
	baseDuration time.Duration
	stats        CacheStrategyStats
	mu           sync.RWMutex
}

// NewDefaultCacheStrategy 创建默认缓存策略
func NewDefaultCacheStrategy(baseDuration time.Duration) *DefaultCacheStrategy {
	return &DefaultCacheStrategy{
		baseDuration: baseDuration,
		stats: CacheStrategyStats{
			StrategyName: "DefaultCacheStrategy",
			LastUpdate:   time.Now(),
		},
	}
}

// GetCacheDuration 获取缓存时间
func (s *DefaultCacheStrategy) GetCacheDuration(engineName, query string, page, pageSize int) time.Duration {
	return s.baseDuration
}

// RecordQuery 记录查询信息
func (s *DefaultCacheStrategy) RecordQuery(engineName, query string, page, pageSize int, duration time.Duration, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.TotalQueries++
	s.stats.TotalDuration += duration
	if s.stats.TotalQueries > 0 {
		s.stats.AverageDuration = s.stats.TotalDuration / time.Duration(s.stats.TotalQueries)
	}
	s.stats.LastUpdate = time.Now()
}

// GetStats 获取策略统计信息
func (s *DefaultCacheStrategy) GetStats() CacheStrategyStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// CacheStrategyManager 缓存策略管理器
type CacheStrategyManager struct {
	strategies      map[string]CacheStrategy
	defaultStrategy CacheStrategy
}

// NewCacheStrategyManager 创建缓存策略管理器
func NewCacheStrategyManager() *CacheStrategyManager {
	return &CacheStrategyManager{
		strategies:      make(map[string]CacheStrategy),
		defaultStrategy: NewDefaultCacheStrategy(30 * time.Minute),
	}
}

// RegisterStrategy 注册缓存策略
func (m *CacheStrategyManager) RegisterStrategy(name string, strategy CacheStrategy) {
	m.strategies[name] = strategy
}

// GetStrategy 获取缓存策略
func (m *CacheStrategyManager) GetStrategy(name string) CacheStrategy {
	if strategy, exists := m.strategies[name]; exists {
		return strategy
	}
	return m.defaultStrategy
}

// GetCacheDuration 获取缓存时间
func (m *CacheStrategyManager) GetCacheDuration(strategyName, engineName, query string, page, pageSize int) time.Duration {
	strategy := m.GetStrategy(strategyName)
	return strategy.GetCacheDuration(engineName, query, page, pageSize)
}

// RecordQuery 记录查询信息
func (m *CacheStrategyManager) RecordQuery(strategyName, engineName, query string, page, pageSize int, duration time.Duration, success bool) {
	strategy := m.GetStrategy(strategyName)
	strategy.RecordQuery(engineName, query, page, pageSize, duration, success)
}

// GetStats 获取策略统计信息
func (m *CacheStrategyManager) GetStats(strategyName string) CacheStrategyStats {
	strategy := m.GetStrategy(strategyName)
	return strategy.GetStats()
}

// PrintStats 打印策略统计信息
func (m *CacheStrategyManager) PrintStats() {
	for name, strategy := range m.strategies {
		stats := strategy.GetStats()
		logger.Infof("Cache strategy stats for %s:", name)
		logger.Infof("  Total queries: %d", stats.TotalQueries)
		logger.Infof("  Cache hits: %d", stats.CacheHits)
		logger.Infof("  Cache misses: %d", stats.CacheMisses)
		logger.Infof("  Average duration: %v", stats.AverageDuration)
		logger.Infof("  Last update: %v", stats.LastUpdate)
	}

	defaultStats := m.defaultStrategy.GetStats()
	logger.Infof("Default cache strategy stats:")
	logger.Infof("  Total queries: %d", defaultStats.TotalQueries)
	logger.Infof("  Cache hits: %d", defaultStats.CacheHits)
	logger.Infof("  Cache misses: %d", defaultStats.CacheMisses)
	logger.Infof("  Average duration: %v", defaultStats.AverageDuration)
	logger.Infof("  Last update: %v", defaultStats.LastUpdate)
}

// ConfigBasedCacheStrategy 基于配置的缓存策略
// 根据引擎配置提供不同的缓存时间
type ConfigBasedCacheStrategy struct {
	engineConfigs map[string]EngineCacheConfig
	defaultTTL    time.Duration
	stats         CacheStrategyStats
	mu            sync.RWMutex
}

// NewConfigBasedCacheStrategy 创建基于配置的缓存策略
func NewConfigBasedCacheStrategy(defaultTTL time.Duration) *ConfigBasedCacheStrategy {
	return &ConfigBasedCacheStrategy{
		engineConfigs: make(map[string]EngineCacheConfig),
		defaultTTL:    defaultTTL,
		stats: CacheStrategyStats{
			StrategyName: "ConfigBasedCacheStrategy",
			LastUpdate:   time.Now(),
		},
	}
}

// SetEngineConfig 设置引擎缓存配置
func (s *ConfigBasedCacheStrategy) SetEngineConfig(engineName string, cfg EngineCacheConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.engineConfigs[strings.ToLower(engineName)] = cfg
}

// SetEngineConfigFromMap 从 map 设置引擎缓存配置
// cfgMap 格式: map[engineName]{enabled, ttl_seconds, max_size}
func (s *ConfigBasedCacheStrategy) SetEngineConfigFromMap(cfgMap map[string]struct {
	Enabled bool
	TTL     int
	MaxSize int
}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for engine, cfg := range cfgMap {
		s.engineConfigs[strings.ToLower(engine)] = &SimpleEngineCacheConfig{
			Enabled: cfg.Enabled,
			TTL:     time.Duration(cfg.TTL) * time.Second,
			MaxSize: cfg.MaxSize,
		}
	}
}

// GetCacheDuration 根据引擎配置获取缓存时间
func (s *ConfigBasedCacheStrategy) GetCacheDuration(engineName, query string, page, pageSize int) time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	engineName = strings.ToLower(engineName)
	if cfg, exists := s.engineConfigs[engineName]; exists {
		if !cfg.IsEnabled() {
			return 0 // 缓存禁用
		}
		ttl := cfg.GetTTL()
		if ttl > 0 {
			return ttl
		}
	}
	return s.defaultTTL
}

// IsCacheEnabledForEngine 检查指定引擎是否启用缓存
func (s *ConfigBasedCacheStrategy) IsCacheEnabledForEngine(engineName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	engineName = strings.ToLower(engineName)
	if cfg, exists := s.engineConfigs[engineName]; exists {
		return cfg.IsEnabled()
	}
	return true // 默认启用
}

// GetMaxSizeForEngine 获取指定引擎的最大缓存条目数
func (s *ConfigBasedCacheStrategy) GetMaxSizeForEngine(engineName string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	engineName = strings.ToLower(engineName)
	if cfg, exists := s.engineConfigs[engineName]; exists {
		if size := cfg.GetMaxSize(); size > 0 {
			return size
		}
	}
	return 1000 // 默认值
}

// RecordQuery 记录查询信息
func (s *ConfigBasedCacheStrategy) RecordQuery(engineName, query string, page, pageSize int, duration time.Duration, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.TotalQueries++
	s.stats.TotalDuration += duration
	if s.stats.TotalQueries > 0 {
		s.stats.AverageDuration = s.stats.TotalDuration / time.Duration(s.stats.TotalQueries)
	}
	s.stats.LastUpdate = time.Now()
}

// GetStats 获取策略统计信息
func (s *ConfigBasedCacheStrategy) GetStats() CacheStrategyStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// GetAllEngineConfigs 获取所有引擎的缓存配置
func (s *ConfigBasedCacheStrategy) GetAllEngineConfigs() map[string]EngineCacheConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]EngineCacheConfig, len(s.engineConfigs))
	for k, v := range s.engineConfigs {
		result[k] = v
	}
	return result
}
