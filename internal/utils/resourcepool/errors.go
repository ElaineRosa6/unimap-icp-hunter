package resourcepool

import "errors"

// 资源池错误定义
var (
	// ErrPoolClosed 资源池已关闭
	ErrPoolClosed = errors.New("resource pool is closed")
	
	// ErrInvalidResourceType 无效的资源类型
	ErrInvalidResourceType = errors.New("invalid resource type")
	
	// ErrResourceNotFound 资源未找到
	ErrResourceNotFound = errors.New("resource not found")
	
	// ErrResourceAcquireTimeout 资源获取超时
	ErrResourceAcquireTimeout = errors.New("resource acquisition timeout")
	
	// ErrResourceValidationFailed 资源验证失败
	ErrResourceValidationFailed = errors.New("resource validation failed")
)