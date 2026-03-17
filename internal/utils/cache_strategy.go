package utils

import (
	"fmt"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

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
}

// queryStat 查询统计信息
type queryStat struct {
	count         int
	totalDuration time.Duration
	successCount  int
	lastQuery     time.Time
}

// engineStat 引擎统计信息
type engineStat struct {
	count         int
	totalDuration time.Duration
	successCount  int
	cacheHitRate  float64
}

// NewDynamicCacheStrategy 创建动态缓存策略
func NewDynamicCacheStrategy(baseDuration, minDuration, maxDuration time.Duration) *DynamicCacheStrategy {
	return &DynamicCacheStrategy{
		baseDuration: baseDuration,
		minDuration:  minDuration,
		maxDuration:  maxDuration,
		queryStats:   make(map[string]queryStat),
		engineStats:  make(map[string]engineStat),
		mutex:        make(chan struct{}, 1),
		lastUpdate:   time.Now(),
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
		averageDuration := stat.totalDuration / time.Duration(stat.count)
		if averageDuration > 5*time.Second {
			duration = duration * 15 / 10
		}

		// 如果最近查询过，减少缓存时间
		if time.Since(stat.lastQuery) < 5*time.Minute {
			duration = duration / 2
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

	// 根据查询特征调整缓存时间
	duration = s.adjustByQueryFeatures(duration, query)

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
	stat.count++
	stat.totalDuration += duration
	if success {
		stat.successCount++
	}
	stat.lastQuery = time.Now()
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
	for key, stat := range s.queryStats {
		if stat.lastQuery.Before(threshold) {
			delete(s.queryStats, key)
		}
	}
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
	s.stats.TotalQueries++
	s.stats.TotalDuration += duration
	if s.stats.TotalQueries > 0 {
		s.stats.AverageDuration = s.stats.TotalDuration / time.Duration(s.stats.TotalQueries)
	}
	s.stats.LastUpdate = time.Now()
}

// GetStats 获取策略统计信息
func (s *DefaultCacheStrategy) GetStats() CacheStrategyStats {
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
