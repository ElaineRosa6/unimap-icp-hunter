package resourcepool

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Resource 资源接口
type Resource interface {
	// ID 返回资源唯一标识
	ID() string
	
	// Validate 验证资源是否有效
	Validate() bool
	
	// Close 关闭资源
	Close() error
	
	// LastUsed 返回最后使用时间
	LastUsed() time.Time
	
	// SetLastUsed 设置最后使用时间
	SetLastUsed(time.Time)
}

// PoolConfig 资源池配置
type PoolConfig struct {
	// MaxSize 最大资源数量
	MaxSize int
	
	// MinSize 最小资源数量
	MinSize int
	
	// MaxIdleTime 最大空闲时间
	MaxIdleTime time.Duration
	
	// ValidateInterval 验证间隔
	ValidateInterval time.Duration
	
	// CreateTimeout 创建超时时间
	CreateTimeout time.Duration
	
	// Name 资源池名称
	Name string
}

// ResourceFactory 资源工厂接口
type ResourceFactory interface {
	// Create 创建新资源
	Create() (Resource, error)
	
	// Validate 验证资源有效性
	Validate(resource Resource) bool
}

// Pool 资源池
type Pool struct {
	config     PoolConfig
	factory    ResourceFactory
	resources  chan Resource
	inUse      map[string]Resource
	mutex      sync.RWMutex
	closed     bool
	metrics    *PoolMetrics
	cleanupTicker *time.Ticker
	validateTicker *time.Ticker
	stopChan   chan struct{}
}

// PoolMetrics 资源池指标
type PoolMetrics struct {
	TotalCreated   int64
	TotalDestroyed int64
	TotalAcquired  int64
	TotalReleased  int64
	TotalErrors    int64
}

// NewPool 创建新的资源池
func NewPool(config PoolConfig, factory ResourceFactory) *Pool {
	if config.MaxSize <= 0 {
		config.MaxSize = 10
	}
	if config.MinSize <= 0 {
		config.MinSize = 1
	}
	if config.MaxIdleTime <= 0 {
		config.MaxIdleTime = 5 * time.Minute
	}
	if config.ValidateInterval <= 0 {
		config.ValidateInterval = 30 * time.Second
	}
	if config.CreateTimeout <= 0 {
		config.CreateTimeout = 30 * time.Second
	}
	if config.Name == "" {
		config.Name = "resource-pool"
	}
	
	pool := &Pool{
		config:     config,
		factory:    factory,
		resources:  make(chan Resource, config.MaxSize),
		inUse:      make(map[string]Resource),
		metrics:    &PoolMetrics{},
		stopChan:   make(chan struct{}),
	}
	
	// 预创建最小数量的资源
	pool.precreateResources()
	
	// 启动清理和验证任务
	pool.startMaintenance()
	
	return pool
}

// precreateResources 预创建资源
func (p *Pool) precreateResources() {
	for i := 0; i < p.config.MinSize; i++ {
		if err := p.createAndAddResource(); err != nil {
			continue
		}
	}
}

// createAndAddResource 创建并添加资源到池中
func (p *Pool) createAndAddResource() error {
	resource, err := p.factory.Create()
	if err != nil {
		atomic.AddInt64(&p.metrics.TotalErrors, 1)
		return err
	}

	p.resources <- resource
	atomic.AddInt64(&p.metrics.TotalCreated, 1)
	return nil
}

// Acquire 获取资源
func (p *Pool) Acquire() (Resource, error) {
	p.mutex.RLock()
	if p.closed {
		p.mutex.RUnlock()
		return nil, errors.New("pool is closed")
	}
	p.mutex.RUnlock()

	// 尝试从池中获取资源
	select {
	case resource := <-p.resources:
		if p.factory.Validate(resource) {
			p.mutex.Lock()
			p.inUse[resource.ID()] = resource
			p.mutex.Unlock()
			atomic.AddInt64(&p.metrics.TotalAcquired, 1)
			return resource, nil
		} else {
			// 资源无效，关闭并创建新资源
			resource.Close()
			atomic.AddInt64(&p.metrics.TotalDestroyed, 1)
		}
	default:
		// 池中没有可用资源，创建新资源
		if p.getCurrentSize() < p.config.MaxSize {
			resource, err := p.factory.Create()
			if err != nil {
				atomic.AddInt64(&p.metrics.TotalErrors, 1)
				return nil, err
			}
			p.mutex.Lock()
			p.inUse[resource.ID()] = resource
			p.mutex.Unlock()
			atomic.AddInt64(&p.metrics.TotalCreated, 1)
			atomic.AddInt64(&p.metrics.TotalAcquired, 1)
			return resource, nil
		}

		// 达到最大容量，等待可用资源
		select {
		case resource := <-p.resources:
			if p.factory.Validate(resource) {
				p.mutex.Lock()
				p.inUse[resource.ID()] = resource
				p.mutex.Unlock()
				atomic.AddInt64(&p.metrics.TotalAcquired, 1)
				return resource, nil
			} else {
				resource.Close()
				atomic.AddInt64(&p.metrics.TotalDestroyed, 1)
				return nil, errors.New("failed to acquire valid resource")
			}
		case <-time.After(p.config.CreateTimeout):
			return nil, errors.New("resource acquisition timeout")
		}
	}

	return nil, errors.New("failed to acquire resource")
}

// Release 释放资源
func (p *Pool) Release(resource Resource) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		resource.Close()
		atomic.AddInt64(&p.metrics.TotalDestroyed, 1)
		return nil
	}

	// 检查资源是否在使用中
	if _, exists := p.inUse[resource.ID()]; !exists {
		return errors.New("resource not in use")
	}

	// 移除使用中的资源
	delete(p.inUse, resource.ID())

	// 更新最后使用时间
	resource.SetLastUsed(time.Now())

	// 将资源放回池中
	select {
	case p.resources <- resource:
		atomic.AddInt64(&p.metrics.TotalReleased, 1)
	default:
		// 池已满，关闭多余资源
		resource.Close()
		atomic.AddInt64(&p.metrics.TotalDestroyed, 1)
	}

	return nil
}

// Close 关闭资源池
func (p *Pool) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return
	}

	// 停止维护任务
	close(p.stopChan)
	if p.cleanupTicker != nil {
		p.cleanupTicker.Stop()
	}
	if p.validateTicker != nil {
		p.validateTicker.Stop()
	}

	// 关闭池中所有资源
	close(p.resources)
	for resource := range p.resources {
		resource.Close()
		atomic.AddInt64(&p.metrics.TotalDestroyed, 1)
	}

	// 关闭正在使用的资源
	for _, resource := range p.inUse {
		resource.Close()
		atomic.AddInt64(&p.metrics.TotalDestroyed, 1)
	}

	p.inUse = make(map[string]Resource)
	p.closed = true
}

// GetMetrics 获取资源池指标
func (p *Pool) GetMetrics() *PoolMetrics {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	
	return &PoolMetrics{
		TotalCreated:   p.metrics.TotalCreated,
		TotalDestroyed: p.metrics.TotalDestroyed,
		TotalAcquired:  p.metrics.TotalAcquired,
		TotalReleased:  p.metrics.TotalReleased,
		TotalErrors:    p.metrics.TotalErrors,
	}
}

// GetStats 获取资源池状态
func (p *Pool) GetStats() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	
	return map[string]interface{}{
		"name":               p.config.Name,
		"max_size":           p.config.MaxSize,
		"min_size":           p.config.MinSize,
		"available":          len(p.resources),
		"in_use":             len(p.inUse),
		"total":              len(p.resources) + len(p.inUse),
		"closed":             p.closed,
		"total_created":      p.metrics.TotalCreated,
		"total_destroyed":    p.metrics.TotalDestroyed,
		"total_acquired":     p.metrics.TotalAcquired,
		"total_released":     p.metrics.TotalReleased,
		"total_errors":       p.metrics.TotalErrors,
	}
}

// getCurrentSize 获取当前资源数量
func (p *Pool) getCurrentSize() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	
	return len(p.resources) + len(p.inUse)
}

// startMaintenance 启动维护任务
func (p *Pool) startMaintenance() {
	// 清理过期资源
	p.cleanupTicker = time.NewTicker(p.config.MaxIdleTime)
	go func() {
		for {
			select {
			case <-p.cleanupTicker.C:
				p.cleanupIdleResources()
			case <-p.stopChan:
				return
			}
		}
	}()
	
	// 验证资源有效性
	p.validateTicker = time.NewTicker(p.config.ValidateInterval)
	go func() {
		for {
			select {
			case <-p.validateTicker.C:
				p.validateResources()
			case <-p.stopChan:
				return
			}
		}
	}()
}

// cleanupIdleResources 清理空闲资源
func (p *Pool) cleanupIdleResources() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return
	}

	// 临时存储需要保留的资源
	var validResources []Resource
	now := time.Now()

	// 检查池中资源
drainIdle:
	for {
		select {
		case resource := <-p.resources:
			if now.Sub(resource.LastUsed()) <= p.config.MaxIdleTime {
				validResources = append(validResources, resource)
			} else {
				resource.Close()
				atomic.AddInt64(&p.metrics.TotalDestroyed, 1)
			}
		default:
			break drainIdle
		}
	}

	// 将有效资源放回池中
	for _, resource := range validResources {
		p.resources <- resource
	}
}

// validateResources 验证资源有效性
func (p *Pool) validateResources() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return
	}

	// 临时存储有效资源
	var validResources []Resource

	// 检查池中资源
drainValidate:
	for {
		select {
		case resource := <-p.resources:
			if p.factory.Validate(resource) {
				validResources = append(validResources, resource)
			} else {
				resource.Close()
				atomic.AddInt64(&p.metrics.TotalDestroyed, 1)
			}
		default:
			break drainValidate
		}
	}

	// 将有效资源放回池中
	for _, resource := range validResources {
		p.resources <- resource
	}
}