package resourcepool

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HTTPResource HTTP资源包装器
type HTTPResource struct {
	client    *http.Client
	id        string
	lastUsed  time.Time
	createdAt time.Time
}

// ID 返回资源唯一标识
func (h *HTTPResource) ID() string {
	return h.id
}

// Validate 验证资源是否有效
func (h *HTTPResource) Validate() bool {
	// 检查客户端是否为nil
	if h.client == nil {
		return false
	}

	// 检查创建时间，超过24小时的连接重新创建
	if time.Since(h.createdAt) > 24*time.Hour {
		return false
	}

	return true
}

// Close 关闭资源
func (h *HTTPResource) Close() error {
	// HTTP客户端不需要显式关闭
	return nil
}

// LastUsed 返回最后使用时间
func (h *HTTPResource) LastUsed() time.Time {
	return h.lastUsed
}

// SetLastUsed 设置最后使用时间
func (h *HTTPResource) SetLastUsed(t time.Time) {
	h.lastUsed = t
}

// Client 返回HTTP客户端
func (h *HTTPResource) Client() *http.Client {
	return h.client
}

// HTTPClientFactory HTTP客户端工厂
type HTTPClientFactory struct {
	timeout time.Duration
}

// NewHTTPClientFactory 创建HTTP客户端工厂
func NewHTTPClientFactory(timeout time.Duration) *HTTPClientFactory {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &HTTPClientFactory{
		timeout: timeout,
	}
}

// Create 创建新的HTTP客户端资源
func (f *HTTPClientFactory) Create() (Resource, error) {
	// 优化HTTP客户端配置
	client := &http.Client{
		Timeout: f.timeout,
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100,
			IdleConnTimeout:       90 * time.Second,
			DisableKeepAlives:     false,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableCompression:    false,
		},
	}

	return &HTTPResource{
		client:    client,
		id:        generateID(),
		lastUsed:  time.Now(),
		createdAt: time.Now(),
	}, nil
}

// HTTPPoolManager HTTP连接池管理器
type HTTPPoolManager struct {
	pool          *Pool
	clientMapping map[*http.Client]*HTTPResource
	mutex         sync.RWMutex
}

// NewHTTPPoolManager 创建HTTP连接池管理器
func NewHTTPPoolManager(config PoolConfig) *HTTPPoolManager {
	if config.Name == "" {
		config.Name = "http-pool"
	}

	factory := NewHTTPClientFactory(30 * time.Second)
	pool := NewPool(config, factory)

	return &HTTPPoolManager{
		pool:          pool,
		clientMapping: make(map[*http.Client]*HTTPResource),
		mutex:         sync.RWMutex{},
	}
}

// AcquireHTTPClient 获取HTTP客户端
func (m *HTTPPoolManager) AcquireHTTPClient() (*http.Client, error) {
	resource, err := m.pool.Acquire()
	if err != nil {
		return nil, err
	}

	httpResource, ok := resource.(*HTTPResource)
	if !ok {
		m.pool.Release(resource)
		return nil, ErrInvalidResourceType
	}

	client := httpResource.Client()

	m.mutex.Lock()
	m.clientMapping[client] = httpResource
	m.mutex.Unlock()

	return client, nil
}

// ReleaseHTTPClient 释放HTTP客户端
func (m *HTTPPoolManager) ReleaseHTTPClient(client *http.Client) error {
	m.mutex.Lock()
	httpResource, exists := m.clientMapping[client]
	if exists {
		delete(m.clientMapping, client)
	}
	m.mutex.Unlock()

	if !exists {
		return fmt.Errorf("client not found in mapping")
	}

	return m.pool.Release(httpResource)
}

// Close 关闭管理器
func (m *HTTPPoolManager) Close() error {
	m.mutex.Lock()
	m.clientMapping = make(map[*http.Client]*HTTPResource)
	m.mutex.Unlock()

	m.pool.Close()
	return nil
}

// GetPool 获取底层连接池
func (m *HTTPPoolManager) GetPool() *Pool {
	return m.pool
}

// Validate 验证HTTP客户端有效性
func (f *HTTPClientFactory) Validate(resource Resource) bool {
	httpResource, ok := resource.(*HTTPResource)
	if !ok {
		return false
	}

	return httpResource.Validate()
}

// NewHTTPPool 创建HTTP连接池（兼容旧接口）
func NewHTTPPool(config PoolConfig) *Pool {
	if config.Name == "" {
		config.Name = "http-pool"
	}

	factory := NewHTTPClientFactory(30 * time.Second)
	return NewPool(config, factory)
}

// AcquireHTTPClient 获取HTTP客户端（兼容旧接口）
func AcquireHTTPClient(pool *Pool) (*http.Client, error) {
	resource, err := pool.Acquire()
	if err != nil {
		return nil, err
	}

	httpResource, ok := resource.(*HTTPResource)
	if !ok {
		pool.Release(resource)
		return nil, ErrInvalidResourceType
	}

	return httpResource.Client(), nil
}

// ReleaseHTTPClient 释放HTTP客户端（兼容旧接口）
func ReleaseHTTPClient(pool *Pool, client *http.Client) error {
	// 注意：旧接口无法直接找到对应的资源对象
	// 建议使用HTTPPoolManager的ReleaseHTTPClient方法
	return nil
}
