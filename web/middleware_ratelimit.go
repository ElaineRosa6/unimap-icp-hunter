package web

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unimap/project/internal/metrics"
)

// RateLimiter 滑动窗口限流器
type RateLimiter struct {
	requests map[string][]time.Time // 每个客户端的请求时间戳列表
	mu       sync.RWMutex
	rate     int           // 窗口内最大请求数
	window   time.Duration // 滑动窗口大小
	stopChan chan struct{} // 停止信号
	stopped  bool
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

// rateLimitDecision is a single atomic request decision and its response state.
type rateLimitDecision struct {
	allowed    bool
	remaining  int
	resetAt    time.Time
	retryAfter int64
}

// Allow checks and consumes one request slot.
func (r *RateLimiter) Allow(clientID string) bool {
	return r.allowWithState(clientID).allowed
}

func activeRateTimestamps(timestamps []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(timestamps) && !timestamps[i].After(cutoff) {
		i++
	}
	return timestamps[i:]
}

func retryAfterSeconds(wait time.Duration) int64 {
	seconds := int64(wait / time.Second)
	if wait%time.Second > 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}

func (r *RateLimiter) allowWithState(clientID string) rateLimitDecision {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Read time inside the lock so concurrently appended timestamps stay ordered.
	now := time.Now()
	timestamps := activeRateTimestamps(r.requests[clientID], now.Add(-r.window))
	allowed := len(timestamps) < r.rate
	if allowed {
		timestamps = append(timestamps, now)
	}
	r.requests[clientID] = timestamps
	remaining, resetAt := r.remainingState(timestamps, now)
	return rateLimitDecision{allowed: allowed, remaining: remaining, resetAt: resetAt, retryAfter: retryAfterSeconds(resetAt.Sub(now))}
}

// remainingState requires the caller to hold a read or write lock and pass
// only live timestamps. The reset denotes the next slot becoming available.
func (r *RateLimiter) remainingState(timestamps []time.Time, now time.Time) (int, time.Time) {
	remaining := r.rate - len(timestamps)
	if remaining < 0 {
		remaining = 0
	}
	resetAt := now.Add(r.window)
	if len(timestamps) > 0 {
		resetAt = timestamps[0].Add(r.window)
	}
	return remaining, resetAt
}

// GetRemaining observes a client's live window without consuming a slot.
func (r *RateLimiter) GetRemaining(clientID string) (int, time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	timestamps := activeRateTimestamps(r.requests[clientID], now.Add(-r.window))
	return r.remainingState(timestamps, now)
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
	globalLimiterMu   sync.RWMutex
	globalLimiterOnce sync.Once
	rateLimitEnabled  atomic.Bool
	trustedProxyMu    sync.RWMutex
	trustedProxyCIDRs []*net.IPNet
)

func init() {
	rateLimitEnabled.Store(true)
}

// getGlobalLimiter 获取全局限流器（懒加载）
func getGlobalLimiter() *RateLimiter {
	globalLimiterOnce.Do(func() {
		globalLimiterMu.Lock()
		// SetRateLimitConfig may have already installed a configured limiter
		// (e.g. from config.yaml) before the first request triggered this Once.
		// Only create the default 60/min limiter if none was set yet, otherwise
		// we would overwrite the configured rate (was bug: 300/min → 60/min).
		if globalLimiter == nil {
			globalLimiter = NewRateLimiter(60, time.Minute)
		}
		globalLimiterMu.Unlock()
	})
	globalLimiterMu.RLock()
	limiter := globalLimiter
	globalLimiterMu.RUnlock()
	return limiter
}

// rateLimitMiddleware 限流中间件
func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rateLimitEnabled.Load() {
			next.ServeHTTP(w, r)
			return
		}

		limiter := getGlobalLimiter()
		if limiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		// 使用客户端 IP 作为标识
		clientID := getClientIP(r)

		// Decide and capture response state atomically for this request.
		decision := limiter.allowWithState(clientID)

		if !decision.allowed {
			metrics.IncRateLimitRejected(metricRoutePath(r))
			// 设置限流响应头
			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(int64(limiter.rate), 10))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(decision.resetAt.UnixMilli(), 10))
			w.Header().Set("Retry-After", strconv.FormatInt(decision.retryAfter, 10))
			writeAPIError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many requests", map[string]int64{
				"retry_after": decision.retryAfter,
			})
			return
		}

		// 请求允许，设置响应头
		w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(int64(limiter.rate), 10))
		rem := int64(decision.remaining)
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(rem, 10))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(decision.resetAt.UnixMilli(), 10))

		next.ServeHTTP(w, r)
	})
}

func getClientIP(r *http.Request) string {
	peer := remoteHost(r.RemoteAddr)
	if !isTrustedProxyIP(net.ParseIP(peer)) {
		return peer
	}
	chain := make([]net.IP, 0)
	for _, value := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		if ip := net.ParseIP(strings.TrimSpace(value)); ip != nil {
			chain = append(chain, ip)
		}
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if !isTrustedProxyIP(chain[i]) {
			return chain[i].String()
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
		return ip.String()
	}
	return peer
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}

func isTrustedProxyIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	trustedProxyMu.RLock()
	defer trustedProxyMu.RUnlock()
	for _, network := range trustedProxyCIDRs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// SetTrustedProxyCIDRs replaces the proxy allowlist used for forwarded headers.
func SetTrustedProxyCIDRs(values []string) error {
	parsed := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q: %w", value, err)
		}
		parsed = append(parsed, network)
	}
	trustedProxyMu.Lock()
	trustedProxyCIDRs = parsed
	trustedProxyMu.Unlock()
	return nil
}

// SetRateLimitConfig 设置限流配置
func SetRateLimitConfig(rate int, window time.Duration) {
	if rate <= 0 {
		rate = 60
	}
	if window <= 0 {
		window = time.Minute
	}

	newLimiter := NewRateLimiter(rate, window)

	globalLimiterMu.Lock()
	old := globalLimiter
	globalLimiter = newLimiter
	globalLimiterMu.Unlock()

	if old != nil {
		old.Stop()
	}
}

// SetRateLimitEnabled 设置是否启用限流
func SetRateLimitEnabled(enabled bool) {
	rateLimitEnabled.Store(enabled)
}
