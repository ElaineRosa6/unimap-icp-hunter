package utils

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestNewExponentialBackoffStrategy(t *testing.T) {
	tests := []struct {
		name        string
		maxRetries  int
		baseDelay   time.Duration
		maxDelay    time.Duration
		jitter      float64
		wantRetries int
		wantBase    time.Duration
		wantMax     time.Duration
	}{
		{
			name:        "valid config",
			maxRetries:  5,
			baseDelay:   200 * time.Millisecond,
			maxDelay:    10 * time.Second,
			jitter:      0.3,
			wantRetries: 5,
			wantBase:    200 * time.Millisecond,
			wantMax:     10 * time.Second,
		},
		{
			name:        "zero retries uses default",
			maxRetries:  0,
			baseDelay:   100 * time.Millisecond,
			maxDelay:    5 * time.Second,
			jitter:      0.2,
			wantRetries: 3,
			wantBase:    100 * time.Millisecond,
			wantMax:     5 * time.Second,
		},
		{
			name:        "negative retries uses default",
			maxRetries:  -1,
			baseDelay:   100 * time.Millisecond,
			maxDelay:    5 * time.Second,
			jitter:      0.2,
			wantRetries: 3,
			wantBase:    100 * time.Millisecond,
			wantMax:     5 * time.Second,
		},
		{
			name:        "zero base delay uses default",
			maxRetries:  3,
			baseDelay:   0,
			maxDelay:    5 * time.Second,
			jitter:      0.2,
			wantRetries: 3,
			wantBase:    100 * time.Millisecond,
			wantMax:     5 * time.Second,
		},
		{
			name:        "zero max delay uses default",
			maxRetries:  3,
			baseDelay:   100 * time.Millisecond,
			maxDelay:    0,
			jitter:      0.2,
			wantRetries: 3,
			wantBase:    100 * time.Millisecond,
			wantMax:     30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := NewExponentialBackoffStrategy(tt.maxRetries, tt.baseDelay, tt.maxDelay, tt.jitter)

			if strategy.MaxRetries() != tt.wantRetries {
				t.Errorf("MaxRetries() = %d, want %d", strategy.MaxRetries(), tt.wantRetries)
			}
			if strategy.baseDelay != tt.wantBase {
				t.Errorf("baseDelay = %v, want %v", strategy.baseDelay, tt.wantBase)
			}
			if strategy.maxDelay != tt.wantMax {
				t.Errorf("maxDelay = %v, want %v", strategy.maxDelay, tt.wantMax)
			}
		})
	}
}

func TestExponentialBackoffStrategy_ShouldRetry(t *testing.T) {
	strategy := NewExponentialBackoffStrategy(3, 100*time.Millisecond, 5*time.Second, 0.2)

	tests := []struct {
		name     string
		attempt  int
		err      error
		resp     *http.Response
		want     bool
	}{
		{
			name:    "within max retries with error",
			attempt: 1,
			err:     errors.New("connection timeout"),
			want:    true,
		},
		{
			name:    "at max retries",
			attempt: 3,
			err:     errors.New("timeout"),
			want:    false,
		},
		{
			name:    "over max retries",
			attempt: 5,
			err:     errors.New("timeout"),
			want:    false,
		},
		{
			name:    "retryable status code 429",
			attempt: 1,
			resp:    &http.Response{StatusCode: http.StatusTooManyRequests},
			want:    true,
		},
		{
			name:    "retryable status code 503",
			attempt: 1,
			resp:    &http.Response{StatusCode: http.StatusServiceUnavailable},
			want:    true,
		},
		{
			name:    "non-retryable status code 200",
			attempt: 1,
			resp:    &http.Response{StatusCode: http.StatusOK},
			want:    false,
		},
		{
			name:    "no error and no response",
			attempt: 1,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strategy.ShouldRetry(tt.attempt, tt.err, tt.resp); got != tt.want {
				t.Errorf("ShouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExponentialBackoffStrategy_GetDelay(t *testing.T) {
	// Use jitterFactor=0.1 to get some variance but predictable range
	strategy := NewExponentialBackoffStrategy(3, 100*time.Millisecond, 5*time.Second, 0.1)

	tests := []struct {
		attempt int
		base    time.Duration
	}{
		{attempt: 0, base: 100 * time.Millisecond},
		{attempt: 1, base: 200 * time.Millisecond},
		{attempt: 2, base: 400 * time.Millisecond},
	}

	for _, tt := range tests {
		got := strategy.GetDelay(tt.attempt)
		// Allow 20% tolerance due to jitter calculation
		minAllowed := tt.base - tt.base/5
		maxAllowed := tt.base + tt.base/5
		if got < minAllowed || got > maxAllowed {
			t.Errorf("GetDelay(%d) = %v, want between %v and %v (base %v)", tt.attempt, got, minAllowed, maxAllowed, tt.base)
		}
	}
}

func TestExponentialBackoffStrategy_GetDelay_MaxCap(t *testing.T) {
	strategy := NewExponentialBackoffStrategy(10, 100*time.Millisecond, 500*time.Millisecond, 0)

	// Large attempt should be capped at maxDelay (with jitter, it may slightly exceed)
	delay := strategy.GetDelay(10)
	// Allow some tolerance for jitter effects
	if delay > strategy.maxDelay+100*time.Millisecond {
		t.Errorf("GetDelay() = %v, should be capped near %v", delay, strategy.maxDelay)
	}
}

func TestNewFixedIntervalStrategy(t *testing.T) {
	tests := []struct {
		name        string
		maxRetries  int
		fixedDelay  time.Duration
		wantRetries int
		wantDelay   time.Duration
	}{
		{
			name:        "valid config",
			maxRetries:  5,
			fixedDelay:  1 * time.Second,
			wantRetries: 5,
			wantDelay:   1 * time.Second,
		},
		{
			name:        "zero retries uses default",
			maxRetries:  0,
			fixedDelay:  500 * time.Millisecond,
			wantRetries: 3,
			wantDelay:   500 * time.Millisecond,
		},
		{
			name:        "zero delay uses default",
			maxRetries:  3,
			fixedDelay:  0,
			wantRetries: 3,
			wantDelay:   500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := NewFixedIntervalStrategy(tt.maxRetries, tt.fixedDelay)

			if strategy.MaxRetries() != tt.wantRetries {
				t.Errorf("MaxRetries() = %d, want %d", strategy.MaxRetries(), tt.wantRetries)
			}
			if strategy.fixedDelay != tt.wantDelay {
				t.Errorf("fixedDelay = %v, want %v", strategy.fixedDelay, tt.wantDelay)
			}
		})
	}
}

func TestFixedIntervalStrategy_ShouldRetry(t *testing.T) {
	strategy := NewFixedIntervalStrategy(3, 500*time.Millisecond)

	tests := []struct {
		name    string
		attempt int
		err     error
		resp    *http.Response
		want    bool
	}{
		{
			name:    "within retries",
			attempt: 1,
			err:     errors.New("timeout"),
			want:    true,
		},
		{
			name:    "at max retries",
			attempt: 3,
			err:     errors.New("timeout"),
			want:    false,
		},
		{
			name:    "429 status",
			attempt: 1,
			resp:    &http.Response{StatusCode: http.StatusTooManyRequests},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strategy.ShouldRetry(tt.attempt, tt.err, tt.resp); got != tt.want {
				t.Errorf("ShouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFixedIntervalStrategy_GetDelay(t *testing.T) {
	strategy := NewFixedIntervalStrategy(3, 500*time.Millisecond)

	for i := 0; i < 5; i++ {
		delay := strategy.GetDelay(i)
		if delay != strategy.fixedDelay {
			t.Errorf("GetDelay(%d) = %v, want %v", i, delay, strategy.fixedDelay)
		}
	}
}

func TestDefaultHTTPClient(t *testing.T) {
	client := DefaultHTTPClient()

	if client == nil {
		t.Fatal("DefaultHTTPClient() returned nil")
	}
	if client.Timeout != 30*time.Second {
		t.Errorf("DefaultHTTPClient() Timeout = %v, want 30s", client.Timeout)
	}

	// Should return same instance on multiple calls
	client2 := DefaultHTTPClient()
	if client != client2 {
		t.Error("DefaultHTTPClient() should return singleton")
	}
}

func TestFastHTTPClient(t *testing.T) {
	client := FastHTTPClient()

	if client == nil {
		t.Fatal("FastHTTPClient() returned nil")
	}
	if client.Timeout != 5*time.Second {
		t.Errorf("FastHTTPClient() Timeout = %v, want 5s", client.Timeout)
	}

	client2 := FastHTTPClient()
	if client != client2 {
		t.Error("FastHTTPClient() should return singleton")
	}
}

func TestSecureHTTPClient(t *testing.T) {
	client := SecureHTTPClient()

	if client == nil {
		t.Fatal("SecureHTTPClient() returned nil")
	}
	if client.Timeout != 60*time.Second {
		t.Errorf("SecureHTTPClient() Timeout = %v, want 60s", client.Timeout)
	}

	client2 := SecureHTTPClient()
	if client != client2 {
		t.Error("SecureHTTPClient() should return singleton")
	}
}

func TestCustomHTTPClient(t *testing.T) {
	client := CustomHTTPClient(20*time.Second, 50, 5)

	if client == nil {
		t.Fatal("CustomHTTPClient() returned nil")
	}
	if client.Timeout != 20*time.Second {
		t.Errorf("CustomHTTPClient() Timeout = %v, want 20s", client.Timeout)
	}
}

func TestHTTPClientWithProxy(t *testing.T) {
	// Without proxy
	client := HTTPClientWithProxy("", 30*time.Second)
	if client == nil {
		t.Fatal("HTTPClientWithProxy() returned nil")
	}
	if client.Timeout != 30*time.Second {
		t.Errorf("HTTPClientWithProxy() Timeout = %v, want 30s", client.Timeout)
	}

	// With invalid proxy URL (should still create client)
	client2 := HTTPClientWithProxy("not-a-valid-url", 30*time.Second)
	if client2 == nil {
		t.Fatal("HTTPClientWithProxy() with invalid proxy should still return client")
	}
}

func TestNewRetryHTTPClient(t *testing.T) {
	// With nil client and strategy (uses defaults)
	client := NewRetryHTTPClient(nil, nil)
	if client == nil {
		t.Fatal("NewRetryHTTPClient() returned nil")
	}
	if client.client == nil {
		t.Error("NewRetryHTTPClient() should use DefaultHTTPClient when nil passed")
	}
	if client.retryStrategy == nil {
		t.Error("NewRetryHTTPClient() should use default strategy when nil passed")
	}

	// With custom client and strategy
	customClient := &http.Client{Timeout: 10 * time.Second}
	customStrategy := NewFixedIntervalStrategy(2, 100*time.Millisecond)
	client2 := NewRetryHTTPClient(customClient, customStrategy)
	if client2.client != customClient {
		t.Error("NewRetryHTTPClient() should use passed client")
	}
	if client2.retryStrategy != customStrategy {
		t.Error("NewRetryHTTPClient() should use passed strategy")
	}
}

// Mock network error for testing
type mockNetError struct {
	timeout   bool
	temporary bool
	msg       string
}

func (e *mockNetError) Error() string   { return e.msg }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return e.temporary }

func TestExponentialBackoffStrategy_NetError(t *testing.T) {
	strategy := NewExponentialBackoffStrategy(3, 100*time.Millisecond, 5*time.Second, 0.2)

	tests := []struct {
		name    string
		err     error
		want    bool
	}{
		{
			name: "timeout net error",
			err:  &mockNetError{timeout: true, temporary: false, msg: "timeout"},
			want: true,
		},
		{
			name: "temporary net error",
			err:  &mockNetError{timeout: false, temporary: true, msg: "temporary"},
			want: true,
		},
		{
			name: "non timeout/temporary net error",
			err:  &mockNetError{timeout: false, temporary: false, msg: "other"},
			want: false,
		},
		{
			name: "regular error",
			err:  errors.New("some error"),
			want: true, // regular errors are retryable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strategy.ShouldRetry(0, tt.err, nil)
			if got != tt.want {
				t.Errorf("ShouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}