package degradation

import (
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// ServiceLevel 服务级别
type ServiceLevel int

const (
	// LevelCritical 关键服务，必须保证可用
	LevelCritical ServiceLevel = iota
	// LevelImportant 重要服务，尽量保证可用
	LevelImportant
	// LevelNormal 普通服务，可以降级
	LevelNormal
	// LevelOptional 可选服务，可以完全降级
	LevelOptional
)

// DegradationStrategy 降级策略类型
type DegradationStrategy int

const (
	// StrategyLoadBased 基于负载的降级策略
	StrategyLoadBased DegradationStrategy = iota
	// StrategyErrorRateBased 基于错误率的降级策略
	StrategyErrorRateBased
	// StrategyResponseTimeBased 基于响应时间的降级策略
	StrategyResponseTimeBased
)

// Config 降级配置
type Config struct {
	// ServiceLevel 服务级别
	ServiceLevel ServiceLevel
	// Strategy 降级策略
	Strategy DegradationStrategy
	// LoadThreshold 负载阈值（仅用于负载策略）
	LoadThreshold float64
	// ErrorRateThreshold 错误率阈值（百分比，仅用于错误率策略）
	ErrorRateThreshold float64
	// ResponseTimeThreshold 响应时间阈值（仅用于响应时间策略）
	ResponseTimeThreshold time.Duration
	// RecoveryInterval 恢复检查间隔
	RecoveryInterval time.Duration
	// Name 服务名称
	Name string
}

// ServiceStatus 服务状态
type ServiceStatus struct {
	IsDegraded     bool
	CurrentLevel   ServiceLevel
	LastCheckTime  time.Time
	Load           float64
	ErrorRate      float64
	ResponseTime   time.Duration
}

// DegradationManager 服务降级管理器接口
type DegradationManager interface {
	// ShouldDegrade 判断是否需要降级
	ShouldDegrade() bool
	// GetStatus 获取服务状态
	GetStatus() ServiceStatus
	// UpdateMetrics 更新服务指标
	UpdateMetrics(load float64, errorRate float64, responseTime time.Duration)
	// Reset 重置降级状态
	Reset()
}

// degradationManager 降级管理器实现
type degradationManager struct {
	config Config

	mutex          sync.RWMutex
	status         ServiceStatus
	degradedLevel  ServiceLevel
	lastDegradeTime time.Time
}

// NewDegradationManager 创建降级管理器
func NewDegradationManager(config Config) DegradationManager {
	// 设置默认值
	if config.RecoveryInterval<= 0 {
		config.RecoveryInterval = 30 * time.Second
	}
	if config.LoadThreshold <= 0 {
		config.LoadThreshold = 0.8 // 80%
	}
	if config.ErrorRateThreshold <= 0 {
		config.ErrorRateThreshold = 0.3 // 30%
	}
	if config.ResponseTimeThreshold<= 0 {
		config.ResponseTimeThreshold = 500 * time.Millisecond
	}
	if config.Name == "" {
		config.Name = "default"
	}

	return &degradationManager{
		config: config,
		status: ServiceStatus{
			IsDegraded:    false,
			CurrentLevel:  config.ServiceLevel,
			LastCheckTime: time.Now(),
		},
		degradedLevel: config.ServiceLevel,
	}
}

// ShouldDegrade 判断是否需要降级
func (dm *degradationManager) ShouldDegrade() bool {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	return dm.status.IsDegraded
}

// GetStatus 获取服务状态
func (dm *degradationManager) GetStatus() ServiceStatus {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	return dm.status
}

// UpdateMetrics 更新服务指标并判断是否需要降级
func (dm *degradationManager) UpdateMetrics(load float64, errorRate float64, responseTime time.Duration) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	// 更新指标
	dm.status.Load = load
	dm.status.ErrorRate = errorRate
	dm.status.ResponseTime = responseTime
	dm.status.LastCheckTime = time.Now()

	// 根据策略判断是否需要降级
	shouldDegrade := false
	
	switch dm.config.Strategy {
	case StrategyLoadBased:
		shouldDegrade = load >dm.config.LoadThreshold
	case StrategyErrorRateBased:
		shouldDegrade = errorRate > dm.config.ErrorRateThreshold
	case StrategyResponseTimeBased:
		shouldDegrade = responseTime > dm.config.ResponseTimeThreshold
	default:
		// 默认使用负载策略
		shouldDegrade = load > dm.config.LoadThreshold
	}

	// 如果需要降级且当前未降级
	if shouldDegrade && !dm.status.IsDegraded {
		dm.status.IsDegraded = true
		dm.lastDegradeTime = time.Now()
		
		// 根据服务级别确定降级程度
		switch dm.config.ServiceLevel {
		case LevelCritical:
			dm.degradedLevel = LevelImportant
		case LevelImportant:
			dm.degradedLevel = LevelNormal
		case LevelNormal:
			dm.degradedLevel = LevelOptional
		case LevelOptional:
			dm.degradedLevel = LevelOptional
		}
		
		dm.status.CurrentLevel = dm.degradedLevel
		logger.Warnf("Service %s degraded: level=%d, load=%.2f, errorRate=%.2f, responseTime=%v", 
			dm.config.Name, dm.degradedLevel, load, errorRate, responseTime)
	}
	
	// 如果不需要降级但当前处于降级状态，检查是否可以恢复
	if !shouldDegrade && dm.status.IsDegraded {
		if time.Since(dm.lastDegradeTime) >= dm.config.RecoveryInterval {
			dm.status.IsDegraded = false
			dm.status.CurrentLevel = dm.config.ServiceLevel
			logger.Infof("Service %s recovered: level=%d", dm.config.Name, dm.config.ServiceLevel)
		}
	}
}

// Reset 重置降级状态
func (dm *degradationManager) Reset() {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()
	
	dm.status.IsDegraded = false
	dm.status.CurrentLevel = dm.config.ServiceLevel
	dm.lastDegradeTime = time.Time{}
	
	logger.Infof("Service %s reset degradation state", dm.config.Name)
}