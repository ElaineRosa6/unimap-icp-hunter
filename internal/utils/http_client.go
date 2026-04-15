package utils

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	defaultHTTPClient     *http.Client
	defaultHTTPClientOnce sync.Once

	fastHTTPClient     *http.Client
	fastHTTPClientOnce sync.Once

	secureHTTPClient     *http.Client
	secureHTTPClientOnce sync.Once
)

// RetryStrategy 重试策略接口
type RetryStrategy interface {
	// ShouldRetry 判断是否应该重试
	ShouldRetry(attempt int, err error, resp *http.Response) bool

	// GetDelay 获取重试延迟时间
	GetDelay(attempt int) time.Duration

	// MaxRetries 获取最大重试次数
	MaxRetries() int
}

// ExponentialBackoffStrategy 指数退避重试策略
type ExponentialBackoffStrategy struct {
	maxRetries           int
	baseDelay            time.Duration
	maxDelay             time.Duration
	jitterFactor         float64
	retryableStatusCodes []int
}

// NewExponentialBackoffStrategy 创建指数退避重试策略
func NewExponentialBackoffStrategy(maxRetries int, baseDelay, maxDelay time.Duration, jitterFactor float64) *ExponentialBackoffStrategy {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}
	if jitterFactor <= 0 {
		jitterFactor = 0.2
	}

	return &ExponentialBackoffStrategy{
		maxRetries:   maxRetries,
		baseDelay:    baseDelay,
		maxDelay:     maxDelay,
		jitterFactor: jitterFactor,
		retryableStatusCodes: []int{
			http.StatusRequestTimeout,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
		},
	}
}

// ShouldRetry 判断是否应该重试
func (s *ExponentialBackoffStrategy) ShouldRetry(attempt int, err error, resp *http.Response) bool {
	if attempt >= s.maxRetries {
		return false
	}

	// 网络错误应该重试
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) {
			return netErr.Timeout() || netErr.Temporary()
		}
		return true
	}

	// 特定HTTP状态码应该重试
	if resp != nil {
		for _, statusCode := range s.retryableStatusCodes {
			if resp.StatusCode == statusCode {
				return true
			}
		}
	}

	return false
}

// GetDelay 获取重试延迟时间（指数退避 + 随机抖动）
func (s *ExponentialBackoffStrategy) GetDelay(attempt int) time.Duration {
	// 指数退避：baseDelay * 2^attempt
	delay := s.baseDelay * time.Duration(1<<uint(attempt))

	// 限制最大延迟
	if delay > s.maxDelay {
		delay = s.maxDelay
	}

	// 添加随机抖动
	jitter := time.Duration(float64(delay) * s.jitterFactor)
	now := time.Now()
	if now.UnixNano()%2 == 0 {
		delay += jitter
	} else {
		delay -= jitter
	}

	if delay < s.baseDelay {
		delay = s.baseDelay
	}

	return delay
}

// MaxRetries 获取最大重试次数
func (s *ExponentialBackoffStrategy) MaxRetries() int {
	return s.maxRetries
}

// FixedIntervalStrategy 固定间隔重试策略
type FixedIntervalStrategy struct {
	maxRetries           int
	fixedDelay           time.Duration
	retryableStatusCodes []int
}

// NewFixedIntervalStrategy 创建固定间隔重试策略
func NewFixedIntervalStrategy(maxRetries int, fixedDelay time.Duration) *FixedIntervalStrategy {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if fixedDelay <= 0 {
		fixedDelay = 500 * time.Millisecond
	}

	return &FixedIntervalStrategy{
		maxRetries: maxRetries,
		fixedDelay: fixedDelay,
		retryableStatusCodes: []int{
			http.StatusRequestTimeout,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
		},
	}
}

// ShouldRetry 判断是否应该重试
func (s *FixedIntervalStrategy) ShouldRetry(attempt int, err error, resp *http.Response) bool {
	if attempt >= s.maxRetries {
		return false
	}

	// 网络错误应该重试
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) {
			return netErr.Timeout() || netErr.Temporary()
		}
		return true
	}

	// 特定HTTP状态码应该重试
	if resp != nil {
		for _, statusCode := range s.retryableStatusCodes {
			if resp.StatusCode == statusCode {
				return true
			}
		}
	}

	return false
}

// GetDelay 获取重试延迟时间
func (s *FixedIntervalStrategy) GetDelay(attempt int) time.Duration {
	return s.fixedDelay
}

// MaxRetries 获取最大重试次数
func (s *FixedIntervalStrategy) MaxRetries() int {
	return s.maxRetries
}

// RetryHTTPClient 支持重试的HTTP客户端
type RetryHTTPClient struct {
	client        *http.Client
	retryStrategy RetryStrategy
}

// NewRetryHTTPClient 创建支持重试的HTTP客户端
func NewRetryHTTPClient(client *http.Client, retryStrategy RetryStrategy) *RetryHTTPClient {
	if client == nil {
		client = DefaultHTTPClient()
	}
	if retryStrategy == nil {
		retryStrategy = NewExponentialBackoffStrategy(3, 100*time.Millisecond, 30*time.Second, 0.2)
	}

	return &RetryHTTPClient{
		client:        client,
		retryStrategy: retryStrategy,
	}
}

// Do 执行HTTP请求并自动重试
func (c *RetryHTTPClient) Do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; ; attempt++ {
		// 创建请求的深拷贝以避免重试时的问题
		reqCopy := req.Clone(req.Context())

		resp, err = c.client.Do(reqCopy)

		// 判断是否需要重试
		if !c.retryStrategy.ShouldRetry(attempt, err, resp) {
			break
		}

		// 关闭当前响应（如果有）
		if resp != nil {
			resp.Body.Close()
		}

		// 等待重试延迟
		delay := c.retryStrategy.GetDelay(attempt)
		time.Sleep(delay)
	}

	return resp, err
}

// Get 执行GET请求并自动重试
func (c *RetryHTTPClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post 执行POST请求并自动重试
func (c *RetryHTTPClient) Post(url, contentType string, body any) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}

	if body != nil {
		var bodyReader io.Reader
		switch v := body.(type) {
		case io.Reader:
			bodyReader = v
		case []byte:
			bodyReader = bytes.NewReader(v)
		case string:
			bodyReader = strings.NewReader(v)
		default:
			return nil, errors.New("unsupported body type")
		}
		req.Body = io.NopCloser(bodyReader)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return c.Do(req)
}

// DefaultHTTPClient 获取默认HTTP客户端
func DefaultHTTPClient() *http.Client {
	defaultHTTPClientOnce.Do(func() {
		defaultHTTPClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
			},
		}
	})
	return defaultHTTPClient
}

// FastHTTPClient 获取快速HTTP客户端（短超时）
func FastHTTPClient() *http.Client {
	fastHTTPClientOnce.Do(func() {
		fastHTTPClient = &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout:   3 * time.Second,
				ResponseHeaderTimeout: 4 * time.Second,
				ExpectContinueTimeout: 500 * time.Millisecond,
				MaxIdleConns:          50,
				MaxIdleConnsPerHost:   5,
				IdleConnTimeout:       30 * time.Second,
			},
		}
	})
	return fastHTTPClient
}

// SecureHTTPClient 获取安全HTTP客户端（长超时，适合重要请求）
func SecureHTTPClient() *http.Client {
	secureHTTPClientOnce.Do(func() {
		secureHTTPClient = &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 60 * time.Second}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 2 * time.Second,
				MaxIdleConns:          200,
				MaxIdleConnsPerHost:   20,
				IdleConnTimeout:       120 * time.Second,
			},
		}
	})
	return secureHTTPClient
}

// CustomHTTPClient 创建自定义HTTP客户端
func CustomHTTPClient(timeout time.Duration, maxIdleConns, maxIdleConnsPerHost int) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: timeout / 2, KeepAlive: timeout}).DialContext,
			TLSHandshakeTimeout:   timeout / 3,
			ResponseHeaderTimeout: timeout / 2,
			ExpectContinueTimeout: time.Second,
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

// HTTPClientWithProxy 创建带代理的HTTP客户端
func HTTPClientWithProxy(proxyURL string, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: timeout / 2, KeepAlive: timeout}).DialContext,
		TLSHandshakeTimeout:   timeout / 3,
		ResponseHeaderTimeout: timeout / 2,
		ExpectContinueTimeout: time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
	}

	if proxyURL != "" {
		proxyURLParsed, err := url.Parse(proxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURLParsed)
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
