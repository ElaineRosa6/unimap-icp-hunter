package circuitbreaker

import (
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// State 熔断器状态
type State int

const (
	// StateClosed 闭合状态，允许请求通过
	StateClosed State = iota
	// StateOpen 开路状态，拒绝所有请求
	StateOpen
	// StateHalfOpen 半开路状态，允许部分请求通过以测试服务是否恢复
	StateHalfOpen
)

// Config 熔断器配置
type Config struct {
	// FailureThreshold 失败阈值，达到此阈值时熔断器打开（百分比，0-100）
	FailureThreshold int
	// RecoveryTimeout 熔断器从开路状态恢复到半开路状态的超时时间
	RecoveryTimeout time.Duration
	// HalfOpenMaxRequests 半开路状态下允许的最大请求数
	HalfOpenMaxRequests int
	// MinRequests 触发熔断所需的最小请求数
	MinRequests int
	// Name 熔断器名称，用于日志和监控
	Name string
}

// CircuitBreaker 熔断器接口
type CircuitBreaker interface {
	// Allow 判断是否允许请求通过
	Allow() bool
	// Success 标记请求成功
	Success()
	// Failure 标记请求失败
	Failure()
	// GetState 获取当前状态
	GetState() State
	// GetStats 获取统计信息
	GetStats() Stats
}

// Stats 熔断器统计信息
type Stats struct {
	TotalRequests    int64
	FailedRequests   int64
	SuccessRequests  int64
	RejectedRequests int64
	FailureRate      float64
	CurrentState     State
	LastStateChange  time.Time
}

// circuitBreaker 熔断器实现
type circuitBreaker struct {
	config Config

	mutex            sync.RWMutex
	state            State
	totalRequests    int64
	failedRequests   int64
	successRequests  int64
	rejectedRequests int64
	lastStateChange  time.Time
	halfOpenRequests int
}

// NewCircuitBreaker 创建新的熔断器
func NewCircuitBreaker(config Config) CircuitBreaker {
	// 设置默认值
	if config.FailureThreshold < 0 || config.FailureThreshold > 100 {
		config.FailureThreshold = 50
	}
	if config.RecoveryTimeout <= 0 {
		config.RecoveryTimeout = 30 * time.Second
	}
	if config.HalfOpenMaxRequests <= 0 {
		config.HalfOpenMaxRequests = 10
	}
	if config.MinRequests <= 0 {
		config.MinRequests = 10
	}
	if config.Name == "" {
		config.Name = "default"
	}

	return &circuitBreaker{
		config:          config,
		state:           StateClosed,
		lastStateChange: time.Now(),
	}
}

// Allow 判断是否允许请求通过
func (cb *circuitBreaker) Allow() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		// 检查是否达到恢复超时时间
		if time.Since(cb.lastStateChange) >= cb.config.RecoveryTimeout {
			// 切换到半开路状态
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			cb.state = StateHalfOpen
			cb.halfOpenRequests = 0
			cb.lastStateChange = time.Now()
			logger.Infof("Circuit breaker %s: state changed from OPEN to HALF_OPEN", cb.config.Name)
			cb.mutex.Unlock()
			cb.mutex.RLock()
			return true
		}
		return false

	case StateHalfOpen:
		// 检查半开路状态下的请求数限制
		if cb.halfOpenRequests < cb.config.HalfOpenMaxRequests {
			return true
		}
		return false
	}

	return false
}

// Success 标记请求成功
func (cb *circuitBreaker) Success() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.totalRequests++
	cb.successRequests++

	switch cb.state {
	case StateClosed:
		// 检查是否需要打开熔断器
		if cb.totalRequests >= int64(cb.config.MinRequests) {
			failureRate := float64(cb.failedRequests) / float64(cb.totalRequests) * 100
			if failureRate >= float64(cb.config.FailureThreshold) {
				cb.state = StateOpen
				cb.lastStateChange = time.Now()
				logger.Warnf("Circuit breaker %s: state changed from CLOSED to OPEN, failure rate: %.2f%%",
					cb.config.Name, failureRate)
			}
		}

	case StateHalfOpen:
		cb.halfOpenRequests++
		// 如果半开路状态下的请求都成功，切换到闭合状态
		if cb.halfOpenRequests >= cb.config.HalfOpenMaxRequests {
			cb.state = StateClosed
			cb.totalRequests = 0
			cb.failedRequests = 0
			cb.lastStateChange = time.Now()
			logger.Infof("Circuit breaker %s: state changed from HALF_OPEN to CLOSED", cb.config.Name)
		}
	}
}

// Failure 标记请求失败
func (cb *circuitBreaker) Failure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.totalRequests++
	cb.failedRequests++

	switch cb.state {
	case StateClosed:
		// 检查是否需要打开熔断器
		if cb.totalRequests >= int64(cb.config.MinRequests) {
			failureRate := float64(cb.failedRequests) / float64(cb.totalRequests) * 100
			if failureRate >= float64(cb.config.FailureThreshold) {
				cb.state = StateOpen
				cb.lastStateChange = time.Now()
				logger.Warnf("Circuit breaker %s: state changed from CLOSED to OPEN, failure rate: %.2f%%",
					cb.config.Name, failureRate)
			}
		}

	case StateHalfOpen:
		// 半开路状态下只要有一个失败就立即打开熔断器
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
		logger.Warnf("Circuit breaker %s: state changed from HALF_OPEN to OPEN due to failure", cb.config.Name)
	}
}

// GetState 获取当前状态
func (cb *circuitBreaker) GetState() State {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetStats 获取统计信息
func (cb *circuitBreaker) GetStats() Stats {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	var failureRate float64
	if cb.totalRequests > 0 {
		failureRate = float64(cb.failedRequests) / float64(cb.totalRequests) * 100
	}

	return Stats{
		TotalRequests:    cb.totalRequests,
		FailedRequests:   cb.failedRequests,
		SuccessRequests:  cb.successRequests,
		RejectedRequests: cb.rejectedRequests,
		FailureRate:      failureRate,
		CurrentState:     cb.state,
		LastStateChange:  cb.lastStateChange,
	}
}
