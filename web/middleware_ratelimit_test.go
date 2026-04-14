package web

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_SlidingWindow_Allow(t *testing.T) {
	limiter := NewRateLimiter(3, time.Second)
	defer limiter.Stop()

	// 前 3 次应该允许
	for i := 0; i < 3; i++ {
		if !limiter.Allow("client-1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// 第 4 次应该被拒绝
	if limiter.Allow("client-1") {
		t.Fatal("request 4 should be rate limited")
	}
}

func TestRateLimiter_SlidingWindow_Expiry(t *testing.T) {
	limiter := NewRateLimiter(2, 100*time.Millisecond)
	defer limiter.Stop()

	// 发送 2 个请求
	limiter.Allow("client-1")
	limiter.Allow("client-1")

	// 应该被拒绝
	if limiter.Allow("client-1") {
		t.Fatal("should be limited")
	}

	// 等待窗口过期
	time.Sleep(150 * time.Millisecond)

	// 应该再次允许
	if !limiter.Allow("client-1") {
		t.Fatal("should be allowed after window expiry")
	}
}

func TestRateLimiter_PerClientIsolation(t *testing.T) {
	limiter := NewRateLimiter(2, time.Second)
	defer limiter.Stop()

	// client-1 耗尽配额
	limiter.Allow("client-1")
	limiter.Allow("client-1")

	// client-1 被拒绝
	if limiter.Allow("client-1") {
		t.Fatal("client-1 should be limited")
	}

	// client-2 仍然允许
	if !limiter.Allow("client-2") {
		t.Fatal("client-2 should not be affected by client-1")
	}
}

func TestRateLimiter_GetRemaining(t *testing.T) {
	limiter := NewRateLimiter(5, time.Second)
	defer limiter.Stop()

	remaining, _ := limiter.GetRemaining("client-1")
	if remaining != 5 {
		t.Fatalf("expected 5 remaining, got %d", remaining)
	}

	limiter.Allow("client-1")
	limiter.Allow("client-1")

	remaining, _ = limiter.GetRemaining("client-1")
	if remaining != 3 {
		t.Fatalf("expected 3 remaining after 2 requests, got %d", remaining)
	}
}

func TestRateLimiter_GetRemaining_ResetTime(t *testing.T) {
	limiter := NewRateLimiter(3, time.Second)
	defer limiter.Stop()

	limiter.Allow("client-1")

	_, resetAt := limiter.GetRemaining("client-1")
	// resetAt 应该在未来
	if resetAt.Before(time.Now()) {
		t.Fatal("resetAt should be in the future")
	}
}

func TestRateLimiter_ConcurrentSafety(t *testing.T) {
	limiter := NewRateLimiter(100, time.Second)
	defer limiter.Stop()

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- limiter.Allow("concurrent-client")
		}()
	}
	wg.Wait()
	close(allowed)

	count := 0
	for ok := range allowed {
		if ok {
			count++
		}
	}

	// 应该恰好允许 100 次
	if count != 100 {
		t.Fatalf("expected 100 allowed, got %d", count)
	}
}

func TestRateLimitMiddleware_Headers(t *testing.T) {
	// 保存旧的限流器
	oldLimiter := globalLimiter
	defer func() { globalLimiter = oldLimiter }()

	// 设置一个高限制的限流器
	globalLimiter = NewRateLimiter(1000, time.Minute)
	defer globalLimiter.Stop()

	handler := rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// 检查响应头
	limit := rec.Header().Get("X-RateLimit-Limit")
	remaining := rec.Header().Get("X-RateLimit-Remaining")
	reset := rec.Header().Get("X-RateLimit-Reset")

	if limit == "" {
		t.Fatal("X-RateLimit-Limit header missing")
	}
	if remaining == "" {
		t.Fatal("X-RateLimit-Remaining header missing")
	}
	if reset == "" {
		t.Fatal("X-RateLimit-Reset header missing")
	}
}

func TestRateLimitMiddleware_RejectionHeaders(t *testing.T) {
	// 保存旧的限流器
	oldLimiter := globalLimiter
	defer func() { globalLimiter = oldLimiter }()

	// 设置很低的限流
	globalLimiter = NewRateLimiter(1, time.Minute)
	defer globalLimiter.Stop()

	handler := rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "reject-client:12345"
	rec := httptest.NewRecorder()

	// 第一个请求（允许）
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rec.Code)
	}

	// 第二个请求（拒绝）
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rec2.Code)
	}

	// 检查拒绝时的响应头
	if rec2.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("expected X-RateLimit-Remaining=0, got %s", rec2.Header().Get("X-RateLimit-Remaining"))
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header missing on rejection")
	}
}

func TestStringInt(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{1000, "1000"},
	}

	for _, tt := range tests {
		if got := stringInt(tt.n); got != tt.want {
			t.Errorf("stringInt(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestSetRateLimitConfig(t *testing.T) {
	oldLimiter := globalLimiter
	oldEnabled := rateLimitEnabled
	defer func() {
		globalLimiter = oldLimiter
		rateLimitEnabled = oldEnabled
	}()

	// 设置自定义限流配置
	SetRateLimitConfig(5, 2*time.Second)

	if globalLimiter == nil {
		t.Fatal("expected globalLimiter to be set")
	}

	// 验证限流生效：前 5 次允许
	for i := 0; i < 5; i++ {
		if !globalLimiter.Allow("test-config") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	// 第 6 次应该被拒绝
	if globalLimiter.Allow("test-config") {
		t.Fatal("request 6 should be rate limited")
	}
}

func TestSetRateLimitConfig_InvalidValues(t *testing.T) {
	oldLimiter := globalLimiter
	defer func() { globalLimiter = oldLimiter }()

	// 无效值应该回退到默认值
	SetRateLimitConfig(-1, -1)

	if globalLimiter == nil {
		t.Fatal("expected globalLimiter to be set")
	}
	// 默认 rate=60，应该允许前 60 个请求
	for i := 0; i < 60; i++ {
		if !globalLimiter.Allow("test-defaults") {
			t.Fatalf("request %d should be allowed with default rate", i+1)
		}
	}
}

func TestSetRateLimitEnabled(t *testing.T) {
	oldEnabled := rateLimitEnabled
	defer func() { rateLimitEnabled = oldEnabled }()

	SetRateLimitEnabled(false)
	if rateLimitEnabled {
		t.Error("expected rateLimitEnabled to be false")
	}

	SetRateLimitEnabled(true)
	if !rateLimitEnabled {
		t.Error("expected rateLimitEnabled to be true")
	}
}
