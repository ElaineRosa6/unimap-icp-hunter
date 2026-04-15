package utils

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	unierror "github.com/unimap-icp-hunter/project/internal/error"
)

// ValidationError 验证错误
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Validator 输入验证器
type Validator struct {
	errors []ValidationError
}

// NewValidator 创建新的验证器
func NewValidator() *Validator {
	return &Validator{
		errors: make([]ValidationError, 0),
	}
}

// ValidateRequired 验证必填字段
func (v *Validator) ValidateRequired(field, value string, fieldName string) {
	if strings.TrimSpace(value) == "" {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fieldName + " is required",
			Code:    6001,
		})
	}
}

// ValidateMinLength 验证最小长度
func (v *Validator) ValidateMinLength(field, value string, minLength int, fieldName string) {
	if utf8.RuneCountInString(value) < minLength {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fieldName + " must be at least " + strconv.Itoa(minLength) + " characters",
			Code:    6005,
		})
	}
}

// ValidateMaxLength 验证最大长度
func (v *Validator) ValidateMaxLength(field, value string, maxLength int, fieldName string) {
	if utf8.RuneCountInString(value) > maxLength {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fieldName + " must not exceed " + strconv.Itoa(maxLength) + " characters",
			Code:    6005,
		})
	}
}

// ValidateEmail 验证邮箱格式
func (v *Validator) ValidateEmail(field, value string) {
	if value == "" {
		return
	}
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !re.MatchString(value) {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "Invalid email format",
			Code:    6002,
		})
	}
}

// ValidateURL 验证URL格式
func (v *Validator) ValidateURL(field, value string) {
	if value == "" {
		return
	}
	_, err := url.ParseRequestURI(value)
	if err != nil {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "Invalid URL format",
			Code:    6002,
		})
	}
}

// ValidatePattern 验证正则表达式
func (v *Validator) ValidatePattern(field, value, pattern, fieldName string) {
	if value == "" {
		return
	}
	re := regexp.MustCompile(pattern)
	if !re.MatchString(value) {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fieldName + " does not match required pattern",
			Code:    6006,
		})
	}
}

// ValidateRange 验证数值范围
func (v *Validator) ValidateRange(field string, value, min, max int, fieldName string) {
	if value < min || value > max {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: fieldName + " must be between " + strconv.Itoa(min) + " and " + strconv.Itoa(max),
			Code:    6003,
		})
	}
}

// ValidateHTTPMethod 验证HTTP方法
func (v *Validator) ValidateHTTPMethod(method string) {
	validMethods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	valid := false
	for _, m := range validMethods {
		if method == m {
			valid = true
			break
		}
	}
	if !valid {
		v.errors = append(v.errors, ValidationError{
			Field:   "method",
			Message: "Invalid HTTP method",
			Code:    6002,
		})
	}
}

// ValidateAPIKey 验证API密钥格式
func (v *Validator) ValidateAPIKey(field, value string) {
	if value == "" {
		return
	}
	// 简单的API密钥验证：至少32个字符，包含字母和数字
	if len(value) < 32 {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "API key must be at least 32 characters",
			Code:    6005,
		})
	}
	re := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	if !re.MatchString(value) {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "API key must contain only alphanumeric characters",
			Code:    6002,
		})
	}
}

// ValidateURLs 验证URL列表
func (v *Validator) ValidateURLs(field string, urls []string) {
	for i, urlStr := range urls {
		if strings.TrimSpace(urlStr) == "" {
			v.errors = append(v.errors, ValidationError{
				Field:   field + "[" + strconv.Itoa(i) + "]",
				Message: "URL cannot be empty",
				Code:    6001,
			})
			continue
		}
		_, err := url.ParseRequestURI(urlStr)
		if err != nil {
			v.errors = append(v.errors, ValidationError{
				Field:   field + "[" + strconv.Itoa(i) + "]",
				Message: "Invalid URL format: " + urlStr,
				Code:    6002,
			})
		}
	}
}

// ValidateConcurrency 验证并发数
func (v *Validator) ValidateConcurrency(field string, concurrency int) {
	if concurrency < 1 {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "Concurrency must be at least 1",
			Code:    6003,
		})
	}
	if concurrency > 100 {
		v.errors = append(v.errors, ValidationError{
			Field:   field,
			Message: "Concurrency must not exceed 100",
			Code:    6003,
		})
	}
}

// HasErrors 检查是否有验证错误
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors 获取所有验证错误
func (v *Validator) Errors() []ValidationError {
	return v.errors
}

// ErrorMessage 获取错误消息
func (v *Validator) ErrorMessage() string {
	if len(v.errors) == 0 {
		return ""
	}
	var messages []string
	for _, err := range v.errors {
		messages = append(messages, err.Message)
	}
	return strings.Join(messages, "; ")
}

// ToUnimapError 转换为UnimapError
func (v *Validator) ToUnimapError() *unierror.UnimapError {
	if !v.HasErrors() {
		return nil
	}
	return unierror.Validation(6000, "%s", v.ErrorMessage())
}

var strictPolicy = bluemonday.StrictPolicy()

// SanitizeInput 清理输入数据，使用 bluemonday 严格策略移除所有 HTML
// 以防止 XSS 攻击。该策略会剥离所有 HTML 标签和属性，仅保留纯文本。
func SanitizeInput(input string) string {
	return strictPolicy.Sanitize(input)
}

// SanitizeQuery 清理查询参数，移除可能导致 SQL 注入的危险字符。
// 注意：此函数仅提供基础的输入清洗，真正的 SQL 注入防护应使用
// 参数化查询（parameterized queries），而非字符串拼接。
func SanitizeQuery(query string) string {
	// 移除常见的 SQL 注入控制字符和注释语法
	query = strings.ReplaceAll(query, "'", "")
	query = strings.ReplaceAll(query, ";", "")
	query = strings.ReplaceAll(query, "--", "")
	query = strings.ReplaceAll(query, "/*", "")
	query = strings.ReplaceAll(query, "*/", "")
	query = strings.TrimSpace(query)
	return query
}

// ValidateRequestSize 验证请求大小
func ValidateRequestSize(contentLength int64, maxSizeMB int) bool {
	maxSizeBytes := int64(maxSizeMB * 1024 * 1024)
	return contentLength <= maxSizeBytes
}

// ValidateContentType 验证Content-Type
func ValidateContentType(contentType, expectedType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), strings.ToLower(expectedType))
}
