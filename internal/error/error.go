package unierror

import (
	"fmt"
	"runtime"
	"strings"
)

// ErrorType 错误类型
type ErrorType string

const (
	// ErrorTypeNetwork 网络错误
	ErrorTypeNetwork ErrorType = "network"
	// ErrorTypeAPI API错误
	ErrorTypeAPI ErrorType = "api"
	// ErrorTypeConfig 配置错误
	ErrorTypeConfig ErrorType = "config"
	// ErrorTypeRuntime 运行时错误
	ErrorTypeRuntime ErrorType = "runtime"
	// ErrorTypeBusiness 业务逻辑错误
	ErrorTypeBusiness ErrorType = "business"
	// ErrorTypeValidation 验证错误
	ErrorTypeValidation ErrorType = "validation"
)

// 错误码定义
const (
	// 网络错误 (1000-1999)
	ErrNetworkTimeout          = 1001
	ErrNetworkConnection       = 1002
	ErrNetworkDNSResolution    = 1003
	ErrNetworkProxyFailure     = 1004
	ErrNetworkSSLHandshake     = 1005
	ErrNetworkTooManyRedirects = 1006

	// API错误 (2000-2999)
	ErrAPIUnauthorized   = 2001
	ErrAPIForbidden      = 2002
	ErrAPINotFound       = 2003
	ErrAPIInternalServer = 2004
	ErrAPIRateLimit      = 2005
	ErrAPIBadRequest     = 2006
	ErrAPITimeout        = 2007

	// 配置错误 (3000-3999)
	ErrConfigInvalid      = 3001
	ErrConfigMissing      = 3002
	ErrConfigParse        = 3003
	ErrConfigValidation   = 3004
	ErrConfigFileNotFound = 3005
	ErrConfigPermission   = 3006

	// 运行时错误 (4000-4999)
	ErrRuntimeOutOfMemory       = 4001
	ErrRuntimeDeadlock          = 4002
	ErrRuntimePanic             = 4003
	ErrRuntimeResourceExhausted = 4004
	ErrRuntimeConcurrency       = 4005

	// 业务逻辑错误 (5000-5999)
	ErrBusinessAlreadyExists   = 5001
	ErrBusinessNotFound        = 5002
	ErrBusinessConflict        = 5003
	ErrBusinessInvalidState    = 5004
	ErrBusinessOperationFailed = 5005

	// 验证错误 (6000-6999)
	ErrValidationRequired = 6001
	ErrValidationFormat   = 6002
	ErrValidationRange    = 6003
	ErrValidationUnique   = 6004
	ErrValidationLength   = 6005
	ErrValidationPattern  = 6006
)

// UnimapError 统一错误结构
type UnimapError struct {
	Type        ErrorType `json:"type"`
	Code        int       `json:"code"`
	Message     string    `json:"message"`
	Details     string    `json:"details,omitempty"`
	StackTrace  string    `json:"stack_trace,omitempty"`
	OriginalErr error     `json:"original_err,omitempty"`
}

// Error 实现error接口
func (e *UnimapError) Error() string {
	if e.OriginalErr != nil {
		return fmt.Sprintf("%s error: %s (original: %v)", e.Type, e.Message, e.OriginalErr)
	}
	return fmt.Sprintf("%s error: %s", e.Type, e.Message)
}

// Unwrap 实现errors.Unwrap接口
func (e *UnimapError) Unwrap() error {
	return e.OriginalErr
}

// New 创建新的统一错误
func New(errType ErrorType, code int, message string, args ...interface{}) *UnimapError {
	details := ""
	if len(args) > 0 {
		details = fmt.Sprintf(message, args...)
	} else {
		details = message
	}

	return &UnimapError{
		Type:       errType,
		Code:       code,
		Message:    message,
		Details:    details,
		StackTrace: getStackTrace(),
	}
}

// Wrap 包装现有错误
func Wrap(err error, errType ErrorType, code int, message string, args ...interface{}) *UnimapError {
	details := ""
	if len(args) > 0 {
		details = fmt.Sprintf(message, args...)
	} else {
		details = message
	}

	return &UnimapError{
		Type:        errType,
		Code:        code,
		Message:     message,
		Details:     details,
		StackTrace:  getStackTrace(),
		OriginalErr: err,
	}
}

// Network 网络错误
func Network(code int, message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeNetwork, code, message, args...)
}

// API API错误
func API(code int, message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeAPI, code, message, args...)
}

// Config 配置错误
func Config(code int, message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeConfig, code, message, args...)
}

// Runtime 运行时错误
func Runtime(code int, message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeRuntime, code, message, args...)
}

// Business 业务逻辑错误
func Business(code int, message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeBusiness, code, message, args...)
}

// Validation 验证错误
func Validation(code int, message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeValidation, code, message, args...)
}

// 便捷的错误创建函数（使用预定义错误码）

// NetworkTimeout 创建网络超时错误
func NetworkTimeout(message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeNetwork, ErrNetworkTimeout, message, args...)
}

// NetworkConnection 创建网络连接错误
func NetworkConnection(message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeNetwork, ErrNetworkConnection, message, args...)
}

// APIUnauthorized 创建API未授权错误
func APIUnauthorized(message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeAPI, ErrAPIUnauthorized, message, args...)
}

// APIForbidden 创建API禁止访问错误
func APIForbidden(message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeAPI, ErrAPIForbidden, message, args...)
}

// APIRateLimit 创建API速率限制错误
func APIRateLimit(message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeAPI, ErrAPIRateLimit, message, args...)
}

// ConfigInvalid 创建配置无效错误
func ConfigInvalid(message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeConfig, ErrConfigInvalid, message, args...)
}

// ConfigMissing 创建配置缺失错误
func ConfigMissing(message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeConfig, ErrConfigMissing, message, args...)
}

// BusinessNotFound 创建业务实体未找到错误
func BusinessNotFound(message string, args ...interface{}) *UnimapError {
	return New(ErrorTypeBusiness, ErrBusinessNotFound, message, args...)
}

// ValidationRequired 创建验证必填错误
func ValidationRequired(field string) *UnimapError {
	return New(ErrorTypeValidation, ErrValidationRequired, "Field %s is required", field)
}

// ValidationFormat 创建验证格式错误
func ValidationFormat(field string, format string) *UnimapError {
	return New(ErrorTypeValidation, ErrValidationFormat, "Field %s must match format: %s", field, format)
}

// getStackTrace 获取堆栈信息
func getStackTrace() string {
	var buf [4096]byte
	n := runtime.Stack(buf[:], false)
	stack := string(buf[:n])

	// 过滤掉错误处理相关的堆栈帧
	lines := strings.Split(stack, "\n")
	var filtered []string
	for _, line := range lines {
		if !strings.Contains(line, "internal/error/") {
			filtered = append(filtered, line)
		}
	}

	return strings.Join(filtered, "\n")
}

// Is 检查错误是否为指定类型
func Is(err error, errType ErrorType) bool {
	if ue, ok := err.(*UnimapError); ok {
		return ue.Type == errType
	}
	return false
}

// GetCode 获取错误代码
func GetCode(err error) int {
	if ue, ok := err.(*UnimapError); ok {
		return ue.Code
	}
	return 0
}

// GetMessage 获取错误信息
func GetMessage(err error) string {
	if ue, ok := err.(*UnimapError); ok {
		return ue.Message
	}
	return err.Error()
}

// GetDetails 获取错误详情
func GetDetails(err error) string {
	if ue, ok := err.(*UnimapError); ok {
		return ue.Details
	}
	return err.Error()
}
