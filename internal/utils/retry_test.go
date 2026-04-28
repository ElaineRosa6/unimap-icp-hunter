package utils

import (
	"errors"
	"testing"
	"time"
)

func TestNewAppError(t *testing.T) {
	err := NewAppError(ErrorTypeNetwork, "connection failed", nil)

	if err.Type != ErrorTypeNetwork {
		t.Errorf("NewAppError() Type = %v, want %v", err.Type, ErrorTypeNetwork)
	}
	if err.Message != "connection failed" {
		t.Errorf("NewAppError() Message = %v, want %v", err.Message, "connection failed")
	}
	if err.Err != nil {
		t.Errorf("NewAppError() Err = %v, want nil", err.Err)
	}
}

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		appErr   *AppError
		wantMsg  string
	}{
		{
			name: "with underlying error",
			appErr: &AppError{
				Type:    ErrorTypeNetwork,
				Message: "connection failed",
				Err:     errors.New("timeout"),
			},
			wantMsg: "network: connection failed: timeout",
		},
		{
			name: "without underlying error",
			appErr: &AppError{
				Type:    ErrorTypeQuota,
				Message: "insufficient balance",
				Err:     nil,
			},
			wantMsg: "quota: insufficient balance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.appErr.Error(); got != tt.wantMsg {
				t.Errorf("AppError.Error() = %v, want %v", got, tt.wantMsg)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	appErr := &AppError{
		Type:    ErrorTypeHTTP,
		Message: "request failed",
		Err:     underlying,
	}

	unwrapped := appErr.Unwrap()
	if unwrapped != underlying {
		t.Errorf("AppError.Unwrap() = %v, want %v", unwrapped, underlying)
	}

	// Test nil Err
	appErrNil := &AppError{Err: nil}
	if appErrNil.Unwrap() != nil {
		t.Error("AppError.Unwrap() should return nil for nil Err")
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantType  string
	}{
		{
			name:     "nil error",
			err:      nil,
			wantType: ErrorTypeUnknown,
		},
		{
			name:     "AppError",
			err:      NewAppError(ErrorTypeRateLimit, "too many", nil),
			wantType: ErrorTypeRateLimit,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			wantType: ErrorTypeNetwork,
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			wantType: ErrorTypeNetwork,
		},
		{
			name:     "timeout",
			err:      errors.New("operation timeout"),
			wantType: ErrorTypeNetwork,
		},
		{
			name:     "no such host",
			err:      errors.New("no such host"),
			wantType: ErrorTypeNetwork,
		},
		{
			name:     "HTTP 500",
			err:      errors.New("HTTP 500 internal server error"),
			wantType: ErrorTypeHTTP,
		},
		{
			name:     "HTTP 429",
			err:      errors.New("HTTP 429 too many requests"),
			wantType: ErrorTypeRateLimit,
		},
		{
			name:     "rate limit message",
			err:      errors.New("rate limit exceeded"),
			wantType: ErrorTypeRateLimit,
		},
		{
			name:     "quota error",
			err:      errors.New("insufficient quota"),
			wantType: ErrorTypeQuota,
		},
		{
			name:     "balance error",
			err:      errors.New("balance too low"),
			wantType: ErrorTypeQuota,
		},
		{
			name:     "unauthorized",
			err:      errors.New("unauthorized access"),
			wantType: ErrorTypeAuthentication,
		},
		{
			name:     "invalid api key",
			err:      errors.New("invalid api key"),
			wantType: ErrorTypeAuthentication,
		},
		{
			name:     "invalid input",
			err:      errors.New("invalid parameter"),
			wantType: ErrorTypeInvalidInput,
		},
		{
			name:     "bad request",
			err:      errors.New("bad request"),
			wantType: ErrorTypeInvalidInput,
		},
		{
			name:     "unknown error",
			err:      errors.New("something unexpected"),
			wantType: ErrorTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.err); got != tt.wantType {
				t.Errorf("ClassifyError() = %v, want %v", got, tt.wantType)
			}
		})
	}
}

func TestIsRetryableErrorByType(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
		want      bool
	}{
		{name: "network is retryable", errorType: ErrorTypeNetwork, want: true},
		{name: "HTTP is retryable", errorType: ErrorTypeHTTP, want: true},
		{name: "rate_limit is retryable", errorType: ErrorTypeRateLimit, want: true},
		{name: "internal is retryable", errorType: ErrorTypeInternal, want: true},
		{name: "unknown is retryable", errorType: ErrorTypeUnknown, want: true},
		{name: "quota is not retryable", errorType: ErrorTypeQuota, want: false},
		{name: "auth is not retryable", errorType: ErrorTypeAuthentication, want: false},
		{name: "invalid_input is not retryable", errorType: ErrorTypeInvalidInput, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryableErrorByType(tt.errorType); got != tt.want {
				t.Errorf("IsRetryableErrorByType() = %v, want %v", got, tt.want)
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
		{name: "nil error", err: nil, want: false},
		{name: "AppError network", err: NewAppError(ErrorTypeNetwork, "test", nil), want: true},
		{name: "AppError quota", err: NewAppError(ErrorTypeQuota, "test", nil), want: false},
		{name: "connection refused", err: errors.New("connection refused"), want: true},
		{name: "timeout", err: errors.New("timeout"), want: true},
		{name: "HTTP 500", err: errors.New("HTTP 500"), want: true},
		{name: "HTTP 429", err: errors.New("HTTP 429"), want: true},
		{name: "quota", err: errors.New("quota exceeded"), want: false},
		{name: "unauthorized", err: errors.New("unauthorized"), want: false},
		{name: "unknown error", err: errors.New("random error"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryableError(tt.err); got != tt.want {
				t.Errorf("IsRetryableError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetry_SuccessFirstTry(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return nil
	}

	config := RetryConfig{MaxRetries: 3}
	err := Retry(config, fn)

	if err != nil {
		t.Errorf("Retry() returned error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("Retry() callCount = %d, want 1", callCount)
	}
}

func TestRetry_MaxRetriesZero(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("always fails")
	}

	config := RetryConfig{MaxRetries: 0}
	err := Retry(config, fn)

	if err == nil {
		t.Error("Retry() should return error when MaxRetries=0 and fn fails")
	}
	if callCount != 1 {
		t.Errorf("Retry() callCount = %d, want 1", callCount)
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return NewAppError(ErrorTypeQuota, "quota exceeded", nil)
	}

	config := RetryConfig{
		MaxRetries:    3,
		RetryableFunc: IsRetryableError,
	}
	err := Retry(config, fn)

	if err == nil {
		t.Error("Retry() should return error for non-retryable error")
	}
	if callCount != 1 {
		t.Errorf("Retry() should not retry non-retryable error, callCount = %d", callCount)
	}
}

func TestRetry_AllRetriesFail(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("connection timeout")
	}

	config := RetryConfig{
		MaxRetries:    2,
		BaseDelay:     1 * time.Millisecond,
		MaxDelay:      10 * time.Millisecond,
		RetryableFunc: func(err error) bool { return true },
		Exponential:   false,
		Jitter:        false,
	}

	err := Retry(config, fn)

	if err == nil {
		t.Error("Retry() should return error after all retries fail")
	}
	if callCount != 3 { // Initial + 2 retries
		t.Errorf("Retry() callCount = %d, want 3", callCount)
	}
}

func TestRetry_ErrorHandlerCalled(t *testing.T) {
	handlerCalls := []int{}
	fn := func() error {
		return errors.New("network error")
	}

	config := RetryConfig{
		MaxRetries:    2,
		BaseDelay:     1 * time.Millisecond,
		MaxDelay:      10 * time.Millisecond,
		RetryableFunc: func(err error) bool { return true },
		ErrorHandler: func(err error, attempt int) {
			handlerCalls = append(handlerCalls, attempt)
		},
		Exponential: false,
		Jitter:      false,
	}

	Retry(config, fn)

	if len(handlerCalls) != 3 {
		t.Errorf("ErrorHandler called %d times, want 3", len(handlerCalls))
	}
}

func TestCalculateDelay(t *testing.T) {
	tests := []struct {
		name      string
		attempt   int
		config    RetryConfig
		wantMin   time.Duration
		wantMax   time.Duration
	}{
		{
			name:    "exponential attempt 0",
			attempt: 0,
			config: RetryConfig{
				BaseDelay:   100 * time.Millisecond,
				MaxDelay:    1 * time.Second,
				Exponential: true,
				Jitter:      false,
			},
			wantMin: 100 * time.Millisecond,
			wantMax: 100 * time.Millisecond,
		},
		{
			name:    "exponential attempt 2",
			attempt: 2,
			config: RetryConfig{
				BaseDelay:   100 * time.Millisecond,
				MaxDelay:    1 * time.Second,
				Exponential: true,
				Jitter:      false,
			},
			wantMin: 400 * time.Millisecond,
			wantMax: 400 * time.Millisecond,
		},
		{
			name:    "fixed delay",
			attempt: 3,
			config: RetryConfig{
				BaseDelay:   200 * time.Millisecond,
				MaxDelay:    1 * time.Second,
				Exponential: false,
				Jitter:      false,
			},
			wantMin: 200 * time.Millisecond,
			wantMax: 200 * time.Millisecond,
		},
		{
			name:    "max delay cap",
			attempt: 10,
			config: RetryConfig{
				BaseDelay:   100 * time.Millisecond,
				MaxDelay:    500 * time.Millisecond,
				Exponential: true,
				Jitter:      false,
			},
			wantMin: 500 * time.Millisecond,
			wantMax: 500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateDelay(tt.attempt, tt.config)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("calculateDelay() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestRetryWithBackoff(t *testing.T) {
	callCount := 0
	err := RetryWithBackoff(func() error {
		callCount++
		if callCount < 3 {
			return errors.New("connection timeout")
		}
		return nil
	})

	if err != nil {
		t.Errorf("RetryWithBackoff() returned error: %v", err)
	}
}

func TestRetryWithCustomConfig(t *testing.T) {
	callCount := 0
	err := RetryWithCustomConfig(2, 10*time.Millisecond, func() error {
		callCount++
		return errors.New("always fails")
	})

	if err == nil {
		t.Error("RetryWithCustomConfig() should return error")
	}
}

func TestRetryWithErrorHandling(t *testing.T) {
	handlerCalled := false
	err := RetryWithErrorHandling(
		func() error { return errors.New("test error") },
		func(err error, attempt int) { handlerCalled = true },
	)

	if err == nil {
		t.Error("RetryWithErrorHandling() should return error")
	}
	if !handlerCalled {
		t.Error("ErrorHandler not called")
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	if DefaultRetryConfig.MaxRetries != 3 {
		t.Errorf("DefaultRetryConfig.MaxRetries = %d, want 3", DefaultRetryConfig.MaxRetries)
	}
	if DefaultRetryConfig.BaseDelay != 100*time.Millisecond {
		t.Errorf("DefaultRetryConfig.BaseDelay = %v, want 100ms", DefaultRetryConfig.BaseDelay)
	}
	if DefaultRetryConfig.MaxDelay != 2*time.Second {
		t.Errorf("DefaultRetryConfig.MaxDelay = %v, want 2s", DefaultRetryConfig.MaxDelay)
	}
	if !DefaultRetryConfig.Exponential {
		t.Error("DefaultRetryConfig.Exponential should be true")
	}
	if !DefaultRetryConfig.Jitter {
		t.Error("DefaultRetryConfig.Jitter should be true")
	}
}