package resourcepool

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// generateID 生成唯一ID
func generateID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return time.Now().Format("20060102150405.000000") + hex.EncodeToString(make([]byte, 4))
	}
	return hex.EncodeToString(bytes)
}

// IsValidResource 检查资源是否有效
func IsValidResource(resource Resource) bool {
	if resource == nil {
		return false
	}
	return resource.Validate()
}

// SafeClose 安全关闭资源
func SafeClose(resource Resource) {
	if resource != nil {
		resource.Close()
	}
}

// WithTimeout 带超时的资源获取
func WithTimeout(pool *Pool, timeout time.Duration) (Resource, error) {
	resultCh := make(chan struct {
		resource Resource
		err      error
	})
	
	go func() {
		resource, err := pool.Acquire()
		resultCh <- struct {
			resource Resource
			err      error
		}{resource, err}
	}()
	
	select {
	case result := <-resultCh:
		return result.resource, result.err
	case <-time.After(timeout):
		return nil, ErrResourceAcquireTimeout
	}
}

// ResourceWrapper 资源包装器，用于添加额外功能
type ResourceWrapper struct {
	Resource
	acquireTime time.Time
}

// NewResourceWrapper 创建资源包装器
func NewResourceWrapper(resource Resource) *ResourceWrapper {
	return &ResourceWrapper{
		Resource:    resource,
		acquireTime: time.Now(),
	}
}

// AcquireTime 返回获取时间
func (w *ResourceWrapper) AcquireTime() time.Time {
	return w.acquireTime
}

// UsageDuration 返回使用时长
func (w *ResourceWrapper) UsageDuration() time.Duration {
	return time.Since(w.acquireTime)
}