package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/metrics"
)

// RateLimiter 限流器
type RateLimiter struct {
	requests   map[string]*clientInfo
	mu         sync.RWMutex
	rate       int           // 每分钟最大请求数
	window     time.Duration // 时间窗口
	stopChan   chan struct{} // 停止信号
	stopped    bool
}

type clientInfo struct {
	count     int
	lastReset time.Time
}

// NewRateLimiter 创建限流器
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	limiter := &RateLimiter{
		requests: make(map[string]*clientInfo),
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

// Allow 检查是否允许请求
func (r *RateLimiter) Allow(clientID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	info, exists := r.requests[clientID]

	if !exists || now.Sub(info.lastReset) > r.window {
		r.requests[clientID] = &clientInfo{
			count:     1,
			lastReset: now,
		}
		return true
	}

	if info.count >= r.rate {
		return false
	}

	info.count++
	return true
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
			for clientID, info := range r.requests {
				if now.Sub(info.lastReset) > r.window*2 {
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

		if !limiter.Allow(clientID) {
			metrics.IncRateLimitRejected(r.URL.Path)
			w.Header().Set("Retry-After", "60")
			writeAPIError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many requests", map[string]int{"retry_after": 60})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP 获取客户端真实 IP
func getClientIP(r *http.Request) string {
	// 检查 X-Forwarded-For 头
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// 取第一个 IP
		for i, c := range xff {
			if c == ',' {
				return strings.TrimSpace(xff[:i])
			}
		}
		return strings.TrimSpace(xff)
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
