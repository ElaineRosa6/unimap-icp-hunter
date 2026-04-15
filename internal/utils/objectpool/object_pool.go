package objectpool

import (
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// ObjectPool 对象池接口
type ObjectPool interface {
	// Acquire 获取对象
	Acquire() (interface{}, error)
	// Release 释放对象
	Release(obj interface{})
	// Close 关闭对象池
	Close()
	// GetStats 获取对象池统计信息
	GetStats() PoolStats
}

// PoolStats 对象池统计信息
type PoolStats struct {
	TotalObjects     int           // 总对象数
	ActiveObjects    int           // 活跃对象数
	IdleObjects      int           // 空闲对象数
	AcquireCount     int64         // 获取次数
	ReleaseCount     int64         // 释放次数
	WaitCount        int64         // 等待次数
	MaxWaitTime      time.Duration // 最大等待时间
	TotalWaitTime    time.Duration // 总等待时间
	AverageWaitTime  time.Duration // 平均等待时间
}

// Config 对象池配置
type Config struct {
	MaxSize           int           // 最大对象数
	InitialSize       int           // 初始对象数
	MaxWaitTime       time.Duration // 最大等待时间
	ObjectFactory     func() (interface{}, error) // 对象工厂函数
	ObjectValidator   func(interface{}) bool      // 对象验证函数
	ObjectDestroyer   func(interface{})           // 对象销毁函数
}

// SimpleObjectPool 简单对象池实现
type SimpleObjectPool struct {
	config         Config
	pool           chan interface{}
	mutex          sync.Mutex
	stats          PoolStats
	totalObjects   int
	activeObjects  int
	closed         bool
}

// NewSimpleObjectPool 创建简单对象池
func NewSimpleObjectPool(config Config) ObjectPool {
	if config.MaxSize<= 0 {
		config.MaxSize = 10
	}
	if config.InitialSize <= 0 {
		config.InitialSize = 0
	}
	if config.MaxWaitTime <= 0 {
		config.MaxWaitTime = 5 * time.Second
	}
	if config.ObjectFactory == nil {
		config.ObjectFactory = func() (interface{}, error) {
			return nil, nil
		}
	}
	if config.ObjectValidator == nil {
		config.ObjectValidator = func(obj interface{}) bool {
			return obj != nil
		}
	}
	if config.ObjectDestroyer == nil {
		config.ObjectDestroyer = func(obj interface{}) {
			// 默认不做任何操作
		}
	}

	pool := &SimpleObjectPool{
		config:        config,
		pool:          make(chan interface{}, config.MaxSize),
		totalObjects:  0,
		activeObjects: 0,
		closed:        false,
	}

	// 预创建初始对象
	for i := 0; i< config.InitialSize; i++ {
		obj, err := config.ObjectFactory()
		if err != nil {
			logger.Warnf("Failed to create initial object: %v", err)
			continue
		}
		pool.pool <- obj
		pool.totalObjects++
	}

	return pool
}

// Acquire 获取对象
func (p *SimpleObjectPool) Acquire() (interface{}, error) {
	if p.closed {
		return nil, ErrPoolClosed
	}

	startTime := time.Now()
	
	select {
	case obj := <-p.pool:
		p.mutex.Lock()
		p.activeObjects++
		p.stats.AcquireCount++
		p.mutex.Unlock()

		// 验证对象是否有效
		if p.config.ObjectValidator(obj) {
			return obj, nil
		}

		// 对象无效，销毁并创建新对象
		p.config.ObjectDestroyer(obj)
		p.mutex.Lock()
		p.totalObjects--
		p.activeObjects--
		p.mutex.Unlock()

		// 创建新对象
		return p.createNewObject()

	default:
		// 池为空，尝试创建新对象
		p.mutex.Lock()
		if p.totalObjects< p.config.MaxSize {
			p.mutex.Unlock()
			return p.createNewObject()
		}
		p.mutex.Unlock()

		// 等待对象释放
		p.mutex.Lock()
		p.stats.WaitCount++
		p.mutex.Unlock()

		select {
		case obj := <-p.pool:
			waitTime := time.Since(startTime)
			p.mutex.Lock()
			p.activeObjects++
			p.stats.AcquireCount++
			p.stats.TotalWaitTime += waitTime
			if waitTime >p.stats.MaxWaitTime {
				p.stats.MaxWaitTime = waitTime
			}
			p.mutex.Unlock()

			if p.config.ObjectValidator(obj) {
				return obj, nil
			}

			p.config.ObjectDestroyer(obj)
			p.mutex.Lock()
			p.totalObjects--
			p.activeObjects--
			p.mutex.Unlock()

			return p.createNewObject()

		case<-time.After(p.config.MaxWaitTime):
			p.mutex.Lock()
			p.stats.WaitCount++
			p.mutex.Unlock()
			return nil, ErrWaitTimeout
		}
	}
}

// createNewObject 创建新对象
func (p *SimpleObjectPool) createNewObject() (interface{}, error) {
	obj, err := p.config.ObjectFactory()
	if err != nil {
		return nil, err
	}

	p.mutex.Lock()
	p.totalObjects++
	p.activeObjects++
	p.stats.AcquireCount++
	p.mutex.Unlock()

	return obj, nil
}

// Release 释放对象
func (p *SimpleObjectPool) Release(obj interface{}) {
	if p.closed {
		p.config.ObjectDestroyer(obj)
		return
	}

	if !p.config.ObjectValidator(obj) {
		p.mutex.Lock()
		p.totalObjects--
		p.activeObjects--
		p.mutex.Unlock()
		p.config.ObjectDestroyer(obj)
		return
	}

	p.mutex.Lock()
	p.activeObjects--
	p.stats.ReleaseCount++
	p.mutex.Unlock()

	select {
	case p.pool <- obj:
		// 对象成功返回池中
	default:
		// 池已满，销毁对象
		p.config.ObjectDestroyer(obj)
		p.mutex.Lock()
		p.totalObjects--
		p.mutex.Unlock()
	}
}

// Close 关闭对象池
func (p *SimpleObjectPool) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return
	}

	p.closed = true

	// 销毁所有对象
	close(p.pool)
	for obj := range p.pool {
		p.config.ObjectDestroyer(obj)
	}

	p.totalObjects = 0
	p.activeObjects = 0
}

// GetStats 获取对象池统计信息
func (p *SimpleObjectPool) GetStats() PoolStats {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	stats := p.stats
	stats.TotalObjects = p.totalObjects
	stats.ActiveObjects = p.activeObjects
	stats.IdleObjects = len(p.pool)

	if stats.WaitCount > 0 {
		stats.AverageWaitTime = stats.TotalWaitTime / time.Duration(stats.WaitCount)
	}

	return stats
}