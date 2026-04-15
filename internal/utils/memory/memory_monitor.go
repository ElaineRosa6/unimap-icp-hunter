package memory

import (
	"runtime"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// MemoryStats 内存统计信息
type MemoryStats struct {
	Timestamp     time.Time
	Alloc         uint64    // 当前分配的内存（字节）
	TotalAlloc    uint64    // 累计分配的内存（字节）
	Sys           uint64    // 从系统获取的内存（字节）
	Lookups       uint64    // 指针查找次数
	Mallocs       uint64    // 内存分配次数
	Frees         uint64    // 内存释放次数
	HeapAlloc     uint64    // 堆内存分配（字节）
	HeapSys       uint64    // 堆内存系统分配（字节）
	HeapIdle      uint64    // 空闲堆内存（字节）
	HeapInuse     uint64    // 使用中的堆内存（字节）
	HeapReleased  uint64    // 释放回系统的堆内存（字节）
	HeapObjects   uint64    // 堆中的对象数量
	StackInuse    uint64    // 栈内存使用（字节）
	StackSys      uint64    // 栈内存系统分配（字节）
	GCCPUFraction float64   // GC占用CPU时间比例
	NumGC         uint32    // GC次数
	LastGC        time.Time // 上次GC时间
	PauseTotalNs  uint64    // GC暂停总时间（纳秒）
	NumGoRoutines int       // Goroutine数量
}

// MemoryMonitor 内存监控器
type MemoryMonitor struct {
	mutex      sync.RWMutex
	stats      []MemoryStats
	maxHistory int
	stopChan   chan struct{}
	running    bool
	wg         sync.WaitGroup
}

// Config 内存监控配置
type Config struct {
	Interval      time.Duration // 监控间隔
	MaxHistory    int           // 最大历史记录数
	EnableLogging bool          // 是否启用日志记录
}

// NewMemoryMonitor 创建内存监控器
func NewMemoryMonitor(config Config) *MemoryMonitor {
	if config.Interval <= 0 {
		config.Interval = 30 * time.Second
	}
	if config.MaxHistory <= 0 {
		config.MaxHistory = 1000
	}

	monitor := &MemoryMonitor{
		stats:      make([]MemoryStats, 0, config.MaxHistory),
		maxHistory: config.MaxHistory,
		stopChan:   make(chan struct{}),
		running:    false,
	}

	return monitor
}

// Start 启动内存监控
func (m *MemoryMonitor) Start() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return
	}

	m.running = true
	m.wg.Add(1)

	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(30 * time.Second) // 默认30秒间隔
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

	logger.Info("Memory monitor started")
}

// Stop 停止内存监控
func (m *MemoryMonitor) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return
	}

	m.running = false
	close(m.stopChan)
	m.wg.Wait()

	logger.Info("Memory monitor stopped")
}

// collectStats 收集内存统计信息
func (m *MemoryMonitor) collectStats() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	stats := MemoryStats{
		Timestamp:     time.Now(),
		Alloc:         memStats.Alloc,
		TotalAlloc:    memStats.TotalAlloc,
		Sys:           memStats.Sys,
		Lookups:       memStats.Lookups,
		Mallocs:       memStats.Mallocs,
		Frees:         memStats.Frees,
		HeapAlloc:     memStats.HeapAlloc,
		HeapSys:       memStats.HeapSys,
		HeapIdle:      memStats.HeapIdle,
		HeapInuse:     memStats.HeapInuse,
		HeapReleased:  memStats.HeapReleased,
		HeapObjects:   memStats.HeapObjects,
		StackInuse:    memStats.StackInuse,
		StackSys:      memStats.StackSys,
		GCCPUFraction: memStats.GCCPUFraction,
		NumGC:         memStats.NumGC,
		LastGC:        time.Unix(0, int64(memStats.LastGC)),
		PauseTotalNs:  memStats.PauseTotalNs,
		NumGoRoutines: runtime.NumGoroutine(),
	}

	m.mutex.Lock()
	m.stats = append(m.stats, stats)
	if len(m.stats) > m.maxHistory {
		m.stats = m.stats[1:]
	}
	m.mutex.Unlock()

	// 总是记录日志
	m.logStats(stats)
}

// logStats 记录内存统计信息
func (m *MemoryMonitor) logStats(stats MemoryStats) {
	logger.Infof("Memory Stats - Alloc: %.2f MB, Sys: %.2f MB, HeapAlloc: %.2f MB, Goroutines: %d, GC: %d times",
		float64(stats.Alloc)/1024/1024,
		float64(stats.Sys)/1024/1024,
		float64(stats.HeapAlloc)/1024/1024,
		stats.NumGoRoutines,
		stats.NumGC,
	)
}

// GetCurrentStats 获取当前内存统计信息
func (m *MemoryMonitor) GetCurrentStats() MemoryStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return MemoryStats{
		Timestamp:     time.Now(),
		Alloc:         memStats.Alloc,
		TotalAlloc:    memStats.TotalAlloc,
		Sys:           memStats.Sys,
		Lookups:       memStats.Lookups,
		Mallocs:       memStats.Mallocs,
		Frees:         memStats.Frees,
		HeapAlloc:     memStats.HeapAlloc,
		HeapSys:       memStats.HeapSys,
		HeapIdle:      memStats.HeapIdle,
		HeapInuse:     memStats.HeapInuse,
		HeapReleased:  memStats.HeapReleased,
		HeapObjects:   memStats.HeapObjects,
		StackInuse:    memStats.StackInuse,
		StackSys:      memStats.StackSys,
		GCCPUFraction: memStats.GCCPUFraction,
		NumGC:         memStats.NumGC,
		LastGC:        time.Unix(0, int64(memStats.LastGC)),
		PauseTotalNs:  memStats.PauseTotalNs,
		NumGoRoutines: runtime.NumGoroutine(),
	}
}

// GetHistoryStats 获取历史内存统计信息
func (m *MemoryMonitor) GetHistoryStats() []MemoryStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := make([]MemoryStats, len(m.stats))
	copy(stats, m.stats)
	return stats
}

// GetMemoryUsage 获取内存使用百分比
func (m *MemoryMonitor) GetMemoryUsage() float64 {
	stats := m.GetCurrentStats()
	if stats.HeapSys == 0 {
		return 0
	}
	return float64(stats.HeapInuse) / float64(stats.HeapSys) * 100
}

// ForceGC 强制触发垃圾回收
func (m *MemoryMonitor) ForceGC() {
	runtime.GC()
	logger.Info("Forced garbage collection triggered")
}

// GetMemoryGrowthRate 获取内存增长率（基于历史数据）
func (m *MemoryMonitor) GetMemoryGrowthRate() float64 {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if len(m.stats) < 2 {
		return 0
	}

	first := m.stats[0]
	last := m.stats[len(m.stats)-1]

	timeDiff := last.Timestamp.Sub(first.Timestamp).Seconds()
	if timeDiff <= 0 {
		return 0
	}

	memDiff := float64(last.Alloc - first.Alloc)
	return memDiff / timeDiff
}
