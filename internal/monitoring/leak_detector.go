package monitoring

import (
	"runtime"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// ResourceLeak 资源泄漏记录
type ResourceLeak struct {
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	AcquireTime  time.Time `json:"acquire_time"`
	ReleaseTime  time.Time `json:"release_time"`
	LeakDuration time.Duration `json:"leak_duration"`
	StackTrace   string    `json:"stack_trace"`
}

// LeakDetector 资源泄漏检测器
type LeakDetector struct {
	mutex           sync.RWMutex
	acquiredResources map[string]*ResourceLeak
	detectedLeaks     []ResourceLeak
	monitorInterval   time.Duration
	maxLeakDuration   time.Duration
	stopChan          chan struct{}
}

// NewLeakDetector 创建资源泄漏检测器
func NewLeakDetector(interval, maxLeakDuration time.Duration) *LeakDetector {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if maxLeakDuration <= 0 {
		maxLeakDuration = 5 * time.Minute
	}
	
	return &LeakDetector{
		acquiredResources: make(map[string]*ResourceLeak),
		detectedLeaks:     make([]ResourceLeak, 0),
		monitorInterval:   interval,
		maxLeakDuration:   maxLeakDuration,
		stopChan:          make(chan struct{}),
	}
}

// Start 启动泄漏检测
func (d *LeakDetector) Start() {
	go func() {
		ticker := time.NewTicker(d.monitorInterval)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				d.detectLeaks()
			case <-d.stopChan:
				return
			}
		}
	}()
	
	logger.Info("Resource leak detector started")
}

// Stop 停止泄漏检测
func (d *LeakDetector) Stop() {
	close(d.stopChan)
	logger.Info("Resource leak detector stopped")
}

// Acquire 记录资源获取
func (d *LeakDetector) Acquire(resourceType, resourceID string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	// 获取堆栈信息
	stackTrace := getStackTrace()
	
	d.acquiredResources[resourceID] = &ResourceLeak{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		AcquireTime:  time.Now(),
		StackTrace:   stackTrace,
	}
}

// Release 记录资源释放
func (d *LeakDetector) Release(resourceID string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	if leak, exists := d.acquiredResources[resourceID]; exists {
		leak.ReleaseTime = time.Now()
		leak.LeakDuration = leak.ReleaseTime.Sub(leak.AcquireTime)
		
		// 如果使用时间超过阈值，记录为潜在泄漏
		if leak.LeakDuration > d.maxLeakDuration {
			d.detectedLeaks = append(d.detectedLeaks, *leak)
			logger.Warnf("Potential resource leak detected: %s (ID: %s), duration: %v", 
				leak.ResourceType, leak.ResourceID, leak.LeakDuration)
		}
		
		delete(d.acquiredResources, resourceID)
	}
}

// detectLeaks 检测资源泄漏
func (d *LeakDetector) detectLeaks() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	now := time.Now()
	var leaksToRemove []string
	
	for resourceID, leak := range d.acquiredResources {
		duration := now.Sub(leak.AcquireTime)
		if duration > d.maxLeakDuration {
			leak.LeakDuration = duration
			d.detectedLeaks = append(d.detectedLeaks, *leak)
			leaksToRemove = append(leaksToRemove, resourceID)
			
			logger.Warnf("Resource leak detected: %s (ID: %s), duration: %v",
				leak.ResourceType, leak.ResourceID, duration)
		}
	}
	
	// 移除检测到的泄漏资源
	for _, resourceID := range leaksToRemove {
		delete(d.acquiredResources, resourceID)
	}
}

// GetDetectedLeaks 获取检测到的泄漏
func (d *LeakDetector) GetDetectedLeaks() []ResourceLeak {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	// 返回副本，避免并发修改
	leaks := make([]ResourceLeak, len(d.detectedLeaks))
	copy(leaks, d.detectedLeaks)
	return leaks
}

// GetActiveResources 获取活跃资源
func (d *LeakDetector) GetActiveResources() []ResourceLeak {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	var activeResources []ResourceLeak
	for _, leak := range d.acquiredResources {
		activeResources = append(activeResources, *leak)
	}
	return activeResources
}

// ClearDetectedLeaks 清除检测到的泄漏
func (d *LeakDetector) ClearDetectedLeaks() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	d.detectedLeaks = make([]ResourceLeak, 0)
}

// GetLeakReport 获取泄漏报告
func (d *LeakDetector) GetLeakReport() map[string]interface{} {
	d.mutex.RLock()
	activeCount := len(d.acquiredResources)
	leakCount := len(d.detectedLeaks)
	leaks := make([]ResourceLeak, leakCount)
	copy(leaks, d.detectedLeaks)
	activeResources := make([]ResourceLeak, 0, activeCount)
	for _, leak := range d.acquiredResources {
		activeResources = append(activeResources, *leak)
	}
	maxLeakDuration := d.maxLeakDuration
	d.mutex.RUnlock()

	report := map[string]interface{}{
		"total_active_resources": activeCount,
		"total_detected_leaks":   leakCount,
		"detected_leaks":         leaks,
		"active_resources":       activeResources,
		"max_leak_duration":      maxLeakDuration,
	}

	return report
}

// getStackTrace 获取堆栈信息
func getStackTrace() string {
	var buf [4096]byte
	n := runtime.Stack(buf[:], false)
	
	// 过滤掉泄漏检测器自身的调用栈
	stack := string(buf[:n])
	lines := splitLines(stack)
	
	// 找到第一个不是泄漏检测器的调用
	for i, line := range lines {
		if !contains(line, "internal/monitoring/leak_detector.go") {
			// 从这里开始返回堆栈
			return joinLines(lines[i:])
		}
	}
	
	return stack
}

// splitLines 分割字符串为行
func splitLines(s string) []string {
	var lines []string
	var currentLine []rune
	
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, string(currentLine))
			currentLine = nil
		} else {
			currentLine = append(currentLine, r)
		}
	}
	
	if len(currentLine) > 0 {
		lines = append(lines, string(currentLine))
	}
	
	return lines
}

// joinLines 连接行为字符串
func joinLines(lines []string) string {
	var result []rune
	for i, line := range lines {
		if i > 0 {
			result = append(result, '\n')
		}
		result = append(result, []rune(line)...)
	}
	return string(result)
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ResourceTracker 资源跟踪器，用于跟踪特定类型的资源
type ResourceTracker struct {
	leakDetector *LeakDetector
	resourceType string
}

// NewResourceTracker 创建资源跟踪器
func NewResourceTracker(leakDetector *LeakDetector, resourceType string) *ResourceTracker {
	return &ResourceTracker{
		leakDetector: leakDetector,
		resourceType: resourceType,
	}
}

// Track 跟踪资源
func (t *ResourceTracker) Track(resourceID string) {
	t.leakDetector.Acquire(t.resourceType, resourceID)
}

// Untrack 取消跟踪资源
func (t *ResourceTracker) Untrack(resourceID string) {
	t.leakDetector.Release(resourceID)
}