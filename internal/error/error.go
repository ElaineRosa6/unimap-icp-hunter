package error

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
		Type:    errType,
		Code:    code,
		Message: message,
		Details: details,
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
