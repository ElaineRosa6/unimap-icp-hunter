package utils

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// 错误类型定义
const (
	ErrorTypeNetwork        = "network"        // 网络错误
	ErrorTypeHTTP           = "http"           // HTTP错误
	ErrorTypeRateLimit      = "rate_limit"     // 速率限制错误
	ErrorTypeQuota          = "quota"          // 配额错误
	ErrorTypeAuthentication = "authentication" // 认证错误
	ErrorTypeInvalidInput   = "invalid_input"  // 无效输入错误
	ErrorTypeInternal       = "internal"       // 内部错误
	ErrorTypeUnknown        = "unknown"        // 未知错误
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries      int                // 最大重试次数
	BaseDelay       time.Duration      // 基础延迟
	MaxDelay        time.Duration      // 最大延迟
	Exponential     bool               // 是否使用指数退避
	Jitter          bool               // 是否添加随机抖动
	RetryableFunc   func(error) bool   // 判断错误是否可重试的函数
	ErrorHandler    func(error, int)   // 错误处理函数（参数：错误，重试次数）
	ErrorClassifier func(error) string // 错误分类函数
}

// AppError 应用错误
type AppError struct {
	Type    string
	Message string
	Err     error
}

// Error 实现error接口
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap 实现errors.Unwrap接口
func (e *AppError) Unwrap() error {
	return e.Err
}

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
	MaxRetries:  3,
	BaseDelay:   100 * time.Millisecond,
	MaxDelay:    2 * time.Second,
	Exponential: true,
	Jitter:      true,
	RetryableFunc: func(err error) bool {
		return IsRetryableError(err)
	},
	ErrorClassifier: ClassifyError,
}

// NewAppError 创建应用错误
func NewAppError(errorType, message string, err error) *AppError {
	return &AppError{
		Type:    errorType,
		Message: message,
		Err:     err,
	}
}

// ClassifyError 分类错误
func ClassifyError(err error) string {
	if err == nil {
		return ErrorTypeUnknown
	}

	// 检查是否是AppError
	if appErr, ok := err.(*AppError); ok {
		return appErr.Type
	}

	errMsg := err.Error()

	// 网络错误
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "network is unreachable") {
		return ErrorTypeNetwork
	}

	// HTTP错误
	if strings.Contains(errMsg, "HTTP 5") {
		return ErrorTypeHTTP
	}

	// 速率限制错误
	if strings.Contains(errMsg, "HTTP 429") ||
		strings.Contains(errMsg, "rate limit") ||
		strings.Contains(errMsg, "too many requests") {
		return ErrorTypeRateLimit
	}

	// 配额错误
	if strings.Contains(errMsg, "quota") ||
		strings.Contains(errMsg, "balance") ||
		strings.Contains(errMsg, "insufficient") ||
		strings.Contains(errMsg, "payment required") {
		return ErrorTypeQuota
	}

	// 认证错误
	if strings.Contains(errMsg, "unauthorized") ||
		strings.Contains(errMsg, "authentication failed") ||
		strings.Contains(errMsg, "invalid api key") ||
		strings.Contains(errMsg, "401") {
		return ErrorTypeAuthentication
	}

	// 无效输入错误
	if strings.Contains(errMsg, "invalid") ||
		strings.Contains(errMsg, "bad request") ||
		strings.Contains(errMsg, "400") {
		return ErrorTypeInvalidInput
	}

	// 默认为未知错误
	return ErrorTypeUnknown
}

// IsRetryableErrorByType 根据错误类型判断是否可重试
func IsRetryableErrorByType(errorType string) bool {
	switch errorType {
	case ErrorTypeNetwork:
		return true
	case ErrorTypeHTTP:
		return true
	case ErrorTypeRateLimit:
		return true
	case ErrorTypeInternal:
		return true
	case ErrorTypeUnknown:
		return true
	default:
		return false
	}
}

// Retry 执行带重试的函数
func Retry(config RetryConfig, fn func() error) error {
	if config.MaxRetries <= 0 {
		return fn()
	}

	if config.BaseDelay <= 0 {
		config.BaseDelay = DefaultRetryConfig.BaseDelay
	}

	if config.MaxDelay <= 0 {
		config.MaxDelay = DefaultRetryConfig.MaxDelay
	}

	if config.RetryableFunc == nil {
		config.RetryableFunc = DefaultRetryConfig.RetryableFunc
	}

	if config.ErrorClassifier == nil {
		config.ErrorClassifier = DefaultRetryConfig.ErrorClassifier
	}

	var lastErr error
	for i := 0; i <= config.MaxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		// 处理错误
		if config.ErrorHandler != nil {
			config.ErrorHandler(err, i+1) // 尝试次数从1开始
		}

		if !config.RetryableFunc(err) {
			return err
		}

		lastErr = err

		if i < config.MaxRetries {
			delay := calculateDelay(i, config)
			time.Sleep(delay)
		}
	}

	return lastErr
}

// RetryWithErrorHandling 带错误处理的重试
func RetryWithErrorHandling(fn func() error, errorHandler func(error, int)) error {
	config := DefaultRetryConfig
	config.ErrorHandler = errorHandler
	return Retry(config, fn)
}

// IsRetryableError 判断错误是否可重试
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 检查是否是AppError
	if appErr, ok := err.(*AppError); ok {
		return IsRetryableErrorByType(appErr.Type)
	}

	errMsg := err.Error()

	// 网络错误
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "network is unreachable") {
		return true
	}

	// HTTP错误
	if strings.Contains(errMsg, "HTTP 5") {
		return true // 5xx错误可重试
	}

	// 429 Too Many Requests
	if strings.Contains(errMsg, "HTTP 429") {
		return true
	}

	// 配额错误（不可重试）
	if strings.Contains(errMsg, "quota") ||
		strings.Contains(errMsg, "balance") ||
		strings.Contains(errMsg, "insufficient") ||
		strings.Contains(errMsg, "payment required") {
		return false
	}

	// 认证错误（不可重试）
	if strings.Contains(errMsg, "unauthorized") ||
		strings.Contains(errMsg, "authentication failed") ||
		strings.Contains(errMsg, "invalid api key") {
		return false
	}

	// 默认可重试
	return true
}

// calculateDelay 计算重试延迟
func calculateDelay(attempt int, config RetryConfig) time.Duration {
	var delay time.Duration

	if config.Exponential {
		// 指数退避：baseDelay * (2^attempt)
		delay = config.BaseDelay * (1 << uint(attempt))
	} else {
		// 固定延迟
		delay = config.BaseDelay
	}

	// 限制最大延迟
	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}

	// 添加随机抖动 (±10%)
	if config.Jitter {
		jitter := time.Duration(rand.Int63n(int64(delay * 20 / 100)))
		delay = delay - (delay * 10 / 100) + jitter
	}

	return delay
}

// RetryWithBackoff 使用指数退避策略执行带重试的函数
func RetryWithBackoff(fn func() error) error {
	return Retry(DefaultRetryConfig, fn)
}

// RetryWithCustomConfig 使用自定义配置执行带重试的函数
func RetryWithCustomConfig(maxRetries int, baseDelay time.Duration, fn func() error) error {
	config := DefaultRetryConfig
	config.MaxRetries = maxRetries
	config.BaseDelay = baseDelay
	return Retry(config, fn)
}
