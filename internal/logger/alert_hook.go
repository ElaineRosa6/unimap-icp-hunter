package logger

import (
	"sync"
	"time"
)

// ErrorAlertHook 是日志 ERROR 级别的告警钩子。
// 在滑动窗口内累计 ERROR 数量，超过阈值时触发回调。
type ErrorAlertHook struct {
	mu          sync.Mutex
	window      time.Duration
	threshold   int
	timestamps  []time.Time
	triggered   int
	onTrigger   func(windowErrors int, triggerCount int)
}

// NewErrorAlertHook 创建 ERROR 告警钩子。
// window: 滑动窗口大小
// threshold: 窗口内 ERROR 数量阈值
// onTrigger: 触发回调（异步执行，不阻塞日志）
func NewErrorAlertHook(window time.Duration, threshold int, onTrigger func(int, int)) *ErrorAlertHook {
	return &ErrorAlertHook{
		window:    window,
		threshold: threshold,
		onTrigger: onTrigger,
	}
}

// OnError 在每次 ERROR 日志时调用。
func (h *ErrorAlertHook) OnError() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	// 清理窗口外的记录
	cutoff := now.Add(-h.window)
	idx := 0
	for idx < len(h.timestamps) && h.timestamps[idx].Before(cutoff) {
		idx++
	}
	if idx > 0 {
		h.timestamps = h.timestamps[idx:]
	}

	h.timestamps = append(h.timestamps, now)

	if len(h.timestamps) >= h.threshold {
		h.triggered++
		count := len(h.timestamps)
		triggers := h.triggered
		// 异步触发，不阻塞日志
		if h.onTrigger != nil {
			go h.onTrigger(count, triggers)
		}
		// 重置计数，避免每个 ERROR 都触发
		h.timestamps = nil
	}
}

// GetTriggerCount 获取触发次数
func (h *ErrorAlertHook) GetTriggerCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.triggered
}

// GetCurrentErrorCount 获取当前窗口内 ERROR 数量
func (h *ErrorAlertHook) GetCurrentErrorCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	cutoff := time.Now().Add(-h.window)
	count := 0
	for _, ts := range h.timestamps {
		if !ts.Before(cutoff) {
			count++
		}
	}
	return count
}

// Reset 重置钩子状态
func (h *ErrorAlertHook) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.timestamps = nil
	h.triggered = 0
}
