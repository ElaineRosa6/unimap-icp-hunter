package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestAuditRateLimitRetryHeaders(t *testing.T) {
	getGlobalLimiter() // Complete lazy initialization before substituting a fixture.
	for _, scenario := range []string{"normal-control", "expired-prefix", "fractional-second"} {
		t.Run(scenario, func(t *testing.T) {
			window := time.Minute
			if scenario == "fractional-second" {
				window = 400 * time.Millisecond
			}
			limiter := NewRateLimiter(1, window)
			t.Cleanup(limiter.Stop)
			now := time.Now()
			limiter.requests["192.0.2.10"] = []time.Time{now}
			if scenario == "expired-prefix" {
				limiter.requests["192.0.2.10"] = []time.Time{now.Add(-2 * time.Minute), now}
			}
			globalLimiterMu.Lock()
			previous := globalLimiter
			globalLimiter = limiter
			globalLimiterMu.Unlock()
			wasEnabled := rateLimitEnabled.Swap(true)
			t.Cleanup(func() {
				globalLimiterMu.Lock()
				globalLimiter = previous
				globalLimiterMu.Unlock()
				rateLimitEnabled.Store(wasEnabled)
			})
			request := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
			request.RemoteAddr = "192.0.2.10:12345"
			response := httptest.NewRecorder()
			rateLimitMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("exhausted client reached handler") })).ServeHTTP(response, request)
			if response.Code != http.StatusTooManyRequests {
				t.Fatalf("status=%d", response.Code)
			}
			retry, err := strconv.Atoi(response.Header().Get("Retry-After"))
			if err != nil || retry < 1 {
				t.Errorf("rejected live window has Retry-After=%q err=%v", response.Header().Get("Retry-After"), err)
			}
			var body struct {
				Error struct {
					Details struct {
						RetryAfter int `json:"retry_after"`
					} `json:"details"`
				} `json:"error"`
			}
			if decodeErr := json.Unmarshal(response.Body.Bytes(), &body); decodeErr != nil || body.Error.Details.RetryAfter != retry {
				t.Fatalf("retry header/body mismatch: header=%d body=%d err=%v", retry, body.Error.Details.RetryAfter, decodeErr)
			}
			reset, err := strconv.ParseInt(response.Header().Get("X-RateLimit-Reset"), 10, 64)
			if err != nil || reset < now.UnixMilli() {
				t.Errorf("live window reset points into past: %d err=%v", reset, err)
			}
		})
	}
}

func TestRateLimitAtomicRemaining(t *testing.T) {
	limiter := NewRateLimiter(50, time.Hour)
	defer limiter.Stop()
	var wg sync.WaitGroup
	results := make(chan rateLimitDecision, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- limiter.allowWithState("fixture") }()
	}
	wg.Wait()
	close(results)
	seen := map[int]bool{}
	rejected := 0
	for result := range results {
		if result.allowed {
			if result.remaining < 0 || result.remaining >= 50 || seen[result.remaining] {
				t.Fatal("duplicate or invalid post-consumption balance")
			}
			seen[result.remaining] = true
		} else {
			rejected++
			if result.remaining != 0 || result.retryAfter < 1 {
				t.Fatal("invalid rejected state")
			}
		}
	}
	if len(seen) != 50 || rejected != 50 {
		t.Fatalf("accepted=%d rejected=%d", len(seen), rejected)
	}
}

func TestRateLimitExpiryAndRetryRounding(t *testing.T) {
	now := time.Now()
	live := now.Add(time.Nanosecond)
	got := activeRateTimestamps([]time.Time{now.Add(-time.Second), now, live}, now)
	if len(got) != 1 || !got[0].Equal(live) {
		t.Fatal("expiry boundary wrong")
	}
	for _, test := range []struct {
		wait time.Duration
		want int64
	}{
		{-time.Second, 1}, {0, 1}, {time.Nanosecond, 1}, {400 * time.Millisecond, 1}, {time.Second, 1}, {time.Second + time.Nanosecond, 2},
	} {
		if gotSeconds := retryAfterSeconds(test.wait); gotSeconds != test.want {
			t.Errorf("wait=%v got=%d want=%d", test.wait, gotSeconds, test.want)
		}
	}
}
