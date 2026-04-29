package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/metrics"
)

// RateLimiter 滑动窗口限流器
type RateLimiter struct {
	requests   map[string][]time.Time // 每个客户端的请求时间戳列表
	mu         sync.RWMutex
	rate       int           // 窗口内最大请求数
	window     time.Duration // 滑动窗口大小
	stopChan   chan struct{} // 停止信号
	stopped    bool
}

// NewRateLimiter 创建限流器
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	limiter := &RateLimiter{
		requests: make(map[string][]time.Time),
		rate:     rate,
		window:   window,
		stopChan: make(chan struct{}),
	}

	// 启动后台清理任务
	go limiter.cleanup()

	return limiter
}

// Stop 停止限流器的清理goroutine
func (r *RateLimiter) Stop() {
	r.mu.Lock()
	if !r.stopped {
		r.stopped = true
		close(r.stopChan)
	}
	r.mu.Unlock()
}

// Allow 检查是否允许请求（滑动窗口）
func (r *RateLimiter) Allow(clientID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// 过滤掉窗口外的时间戳
	timestamps := r.requests[clientID]
	idx := 0
	for idx < len(timestamps) && timestamps[idx].Before(cutoff) {
		idx++
	}
	if idx > 0 {
		timestamps = timestamps[idx:]
	}

	// 检查是否超过限制
	if len(timestamps) >= r.rate {
		r.requests[clientID] = timestamps
		return false
	}

	// 记录新请求
	r.requests[clientID] = append(timestamps, now)
	return true
}

// GetRemaining 获取客户端剩余请求数
func (r *RateLimiter) GetRemaining(clientID string) (remaining int, resetAt time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	timestamps := r.requests[clientID]
	// 计算窗口内的请求数
	count := 0
	for _, ts := range timestamps {
		if !ts.Before(cutoff) {
			count++
		}
	}

	remaining = r.rate - count
	if remaining < 0 {
		remaining = 0
	}

	// 计算窗口重置时间（最早记录的过期时间）
	if len(timestamps) > 0 {
		resetAt = timestamps[0].Add(r.window)
	} else {
		resetAt = now.Add(r.window)
	}

	return remaining, resetAt
}

// cleanup 定期清理过期的客户端记录
func (r *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			return
		case <-ticker.C:
			r.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-r.window * 2)
			for clientID, timestamps := range r.requests {
				// 移除所有记录都过期的客户端
				if len(timestamps) == 0 || timestamps[len(timestamps)-1].Before(cutoff) {
					delete(r.requests, clientID)
				}
			}
			r.mu.Unlock()
		}
	}
}

// 全局限流器实例
var (
	globalLimiter     *RateLimiter
	globalLimiterOnce sync.Once
	rateLimitEnabled  = true
)

// getGlobalLimiter 获取全局限流器（懒加载）
func getGlobalLimiter() *RateLimiter {
	globalLimiterOnce.Do(func() {
		// 默认: 每分钟 60 次请求
		globalLimiter = NewRateLimiter(60, time.Minute)
	})
	return globalLimiter
}

// rateLimitMiddleware 限流中间件
func rateLimitMiddleware(next http.Handler) http.Handler {
	limiter := getGlobalLimiter()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rateLimitEnabled {
			next.ServeHTTP(w, r)
			return
		}

		// 使用客户端 IP 作为标识
		clientID := getClientIP(r)

		// 获取剩余请求数（在 Allow 之前）
		remaining, resetAt := limiter.GetRemaining(clientID)

		if !limiter.Allow(clientID) {
			metrics.IncRateLimitRejected(r.URL.Path)
			// 设置限流响应头
			w.Header().Set("X-RateLimit-Limit", stringInt(int64(limiter.rate)))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", stringInt(unixMillis(resetAt)))
			w.Header().Set("Retry-After", stringInt(int64(time.Until(resetAt).Seconds()+0.5)))
			writeAPIError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many requests", map[string]int64{
				"retry_after": int64(time.Until(resetAt).Seconds()) + 1,
			})
			return
		}

		// 请求允许，设置响应头
		w.Header().Set("X-RateLimit-Limit", stringInt(int64(limiter.rate)))
		rem := int64(remaining) - 1
		if rem < 0 {
			rem = 0
		}
		w.Header().Set("X-RateLimit-Remaining", stringInt(rem))
		w.Header().Set("X-RateLimit-Reset", stringInt(unixMillis(resetAt)))

		next.ServeHTTP(w, r)
	})
}

func unixMillis(t time.Time) int64 {
	return t.UnixMilli()
}

func stringInt(n int64) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// getClientIP 获取客户端真实 IP
func getClientIP(r *http.Request) string {
	// Only trust X-Forwarded-For when the immediate connection is a known proxy
	if isPrivateOrInternalHost(r.Context(), r.RemoteAddr) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Take the first IP (original client)
			for i, c := range xff {
				if c == ',' {
					return strings.TrimSpace(xff[:i])
				}
			}
			return strings.TrimSpace(xff)
		}
	}

	// 检查 X-Real-IP 头
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// 使用 RemoteAddr
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

// SetRateLimitConfig 设置限流配置
func SetRateLimitConfig(rate int, window time.Duration) {
	if rate <= 0 {
		rate = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	// 停止旧的限流器
	if globalLimiter != nil {
		globalLimiter.Stop()
	}
	globalLimiter = NewRateLimiter(rate, window)
}

// SetRateLimitEnabled 设置是否启用限流
func SetRateLimitEnabled(enabled bool) {
	rateLimitEnabled = enabled
}
