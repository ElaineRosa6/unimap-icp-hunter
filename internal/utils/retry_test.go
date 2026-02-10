package utils

import (
	"errors"
	"testing"
	"time"
)

func TestAppError_Error(t *testing.T) {
	originalErr := errors.New("original error")
	appErr := NewAppError(ErrorTypeNetwork, "network error", originalErr)

	errorMsg := appErr.Error()
	if errorMsg == "" {
		t.Error("AppError.Error() returned empty string")
	}
	if appErr.Type != ErrorTypeNetwork {
		t.Errorf("AppError.Type should be %s, got %s", ErrorTypeNetwork, appErr.Type)
	}
	if appErr.Message != "network error" {
		t.Errorf("AppError.Message should be 'network error', got '%s'", appErr.Message)
	}
	if appErr.Err != originalErr {
		t.Error("AppError.Err should be the original error")
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "Nil error",
			err:  nil,
			want: ErrorTypeUnknown,
		},
		{
			name: "Network error",
			err:  errors.New("connection refused"),
			want: ErrorTypeNetwork,
		},
		{
			name: "HTTP 5xx error",
			err:  errors.New("HTTP 500 Internal Server Error"),
			want: ErrorTypeHTTP,
		},
		{
			name: "Rate limit error",
			err:  errors.New("HTTP 429 Too Many Requests"),
			want: ErrorTypeRateLimit,
		},
		{
			name: "Quota error",
			err:  errors.New("quota exceeded"),
			want: ErrorTypeQuota,
		},
		{
			name: "Authentication error",
			err:  errors.New("unauthorized"),
			want: ErrorTypeAuthentication,
		},
		{
			name: "Invalid input error",
			err:  errors.New("invalid input"),
			want: ErrorTypeInvalidInput,
		},
		{
			name: "Unknown error",
			err:  errors.New("some unknown error"),
			want: ErrorTypeUnknown,
		},
		{
			name: "AppError with known type",
			err:  NewAppError(ErrorTypeNetwork, "network error", nil),
			want: ErrorTypeNetwork,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.want {
				t.Errorf("ClassifyError() got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestIsRetryableErrorByType(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		want    bool
	}{
		{
			name:    "Network error (retryable)",
			errType: ErrorTypeNetwork,
			want:    true,
		},
		{
			name:    "HTTP error (retryable)",
			errType: ErrorTypeHTTP,
			want:    true,
		},
		{
			name:    "Rate limit error (retryable)",
			errType: ErrorTypeRateLimit,
			want:    true,
		},
		{
			name:    "Quota error (not retryable)",
			errType: ErrorTypeQuota,
			want:    false,
		},
		{
			name:    "Authentication error (not retryable)",
			errType: ErrorTypeAuthentication,
			want:    false,
		},
		{
			name:    "Invalid input error (not retryable)",
			errType: ErrorTypeInvalidInput,
			want:    false,
		},
		{
			name:    "Internal error (retryable)",
			errType: ErrorTypeInternal,
			want:    true,
		},
		{
			name:    "Unknown error (retryable)",
			errType: ErrorTypeUnknown,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryableErrorByType(tt.errType)
			if got != tt.want {
				t.Errorf("IsRetryableErrorByType() got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Nil error (not retryable)",
			err:  nil,
			want: false,
		},
		{
			name: "Network error (retryable)",
			err:  errors.New("connection refused"),
			want: true,
		},
		{
			name: "HTTP 5xx error (retryable)",
			err:  errors.New("HTTP 500 Internal Server Error"),
			want: true,
		},
		{
			name: "Rate limit error (retryable)",
			err:  errors.New("HTTP 429 Too Many Requests"),
			want: true,
		},
		{
			name: "Quota error (not retryable)",
			err:  errors.New("quota exceeded"),
			want: false,
		},
		{
			name: "Authentication error (not retryable)",
			err:  errors.New("unauthorized"),
			want: false,
		},
		{
			name: "AppError with retryable type",
			err:  NewAppError(ErrorTypeNetwork, "network error", nil),
			want: true,
		},
		{
			name: "AppError with non-retryable type",
			err:  NewAppError(ErrorTypeAuthentication, "auth error", nil),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryableError(tt.err)
			if got != tt.want {
				t.Errorf("IsRetryableError() got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetry_Success(t *testing.T) {
	// 测试成功执行，不需要重试
	count := 0
	err := Retry(DefaultRetryConfig, func() error {
		count++
		return nil
	})

	if err != nil {
		t.Errorf("Retry() returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("Retry() should call the function once, got %d calls", count)
	}
}

func TestRetry_WithRetry(t *testing.T) {
	// 测试失败后重试，直到成功
	count := 0
	err := Retry(DefaultRetryConfig, func() error {
		count++
		if count < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Retry() returned error: %v", err)
	}
	if count != 3 {
		t.Errorf("Retry() should call the function 3 times, got %d calls", count)
	}
}

func TestRetry_WithMaxRetries(t *testing.T) {
	// 测试达到最大重试次数后返回错误
	count := 0
	config := DefaultRetryConfig
	config.MaxRetries = 2

	err := Retry(config, func() error {
		count++
		return errors.New("persistent error")
	})

	if err == nil {
		t.Error("Retry() should return error after max retries")
	}
	if count != config.MaxRetries+1 {
		t.Errorf("Retry() should call the function %d times, got %d calls", config.MaxRetries+1, count)
	}
}

func TestRetry_WithNonRetryableError(t *testing.T) {
	// 测试遇到不可重试错误时立即返回
	count := 0
	err := Retry(DefaultRetryConfig, func() error {
		count++
		return NewAppError(ErrorTypeAuthentication, "auth error", nil)
	})

	if err == nil {
		t.Error("Retry() should return error for non-retryable error")
	}
	if count != 1 {
		t.Errorf("Retry() should call the function once for non-retryable error, got %d calls", count)
	}
}

func TestRetryWithBackoff(t *testing.T) {
	// 测试使用指数退避策略
	count := 0
	err := RetryWithBackoff(func() error {
		count++
		if count < 2 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("RetryWithBackoff() returned error: %v", err)
	}
	if count != 2 {
		t.Errorf("RetryWithBackoff() should call the function 2 times, got %d calls", count)
	}
}

func TestRetryWithCustomConfig(t *testing.T) {
	// 测试使用自定义配置
	count := 0
	maxRetries := 1
	baseDelay := 50 * time.Millisecond

	err := RetryWithCustomConfig(maxRetries, baseDelay, func() error {
		count++
		if count < 2 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("RetryWithCustomConfig() returned error: %v", err)
	}
	if count != 2 {
		t.Errorf("RetryWithCustomConfig() should call the function 2 times, got %d calls", count)
	}
}

func TestRetryWithErrorHandling(t *testing.T) {
	// 测试带错误处理的重试
	count := 0
	errorHandlerCalled := 0

	err := RetryWithErrorHandling(func() error {
		count++
		if count < 2 {
			return errors.New("temporary error")
		}
		return nil
	}, func(err error, attempt int) {
		errorHandlerCalled++
		if err == nil {
			t.Error("Error handler should receive error")
		}
		if attempt < 1 {
			t.Error("Error handler should receive attempt number >= 1")
		}
	})

	if err != nil {
		t.Errorf("RetryWithErrorHandling() returned error: %v", err)
	}
	if count != 2 {
		t.Errorf("RetryWithErrorHandling() should call the function 2 times, got %d calls", count)
	}
	if errorHandlerCalled != 1 {
		t.Errorf("Error handler should be called once, got %d calls", errorHandlerCalled)
	}
}

func TestCalculateDelay(t *testing.T) {
	config := DefaultRetryConfig

	// 禁用随机抖动，确保测试结果可重现
	config.Jitter = false

	// 测试指数退避
	config.Exponential = true
	delay1 := calculateDelay(0, config)
	delay2 := calculateDelay(1, config)
	if delay2 <= delay1 {
		t.Error("Exponential delay should increase with attempt number")
	}

	// 测试固定延迟
	config.Exponential = false
	delay1 = calculateDelay(0, config)
	delay2 = calculateDelay(1, config)
	if delay2 != delay1 {
		t.Error("Fixed delay should be the same for all attempts")
	}

	// 测试最大延迟限制
	config.Exponential = true
	config.BaseDelay = time.Millisecond
	config.MaxDelay = time.Millisecond * 10
	delay := calculateDelay(10, config) // 应该远大于maxDelay
	if delay > config.MaxDelay {
		t.Error("Delay should not exceed maxDelay")
	}

	// 测试随机抖动
	config.Jitter = true
	delay1 = calculateDelay(1, config)
	delay2 = calculateDelay(1, config)
	if delay1 == delay2 {
		t.Error("Jitter should produce different delays for the same attempt")
	}
}
