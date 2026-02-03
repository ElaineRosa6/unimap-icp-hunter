package utils

import (
	"crypto/tls"
	"net/http"
	"time"
)

// NewHTTPClient 创建HTTP客户端
func NewHTTPClient(timeout time.Duration, disableTLSVerify bool) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: disableTLSVerify,
			},
			// 启用连接复用以提高性能
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false, // 启用 Keep-Alive
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// HTTPResponse 封装HTTP响应
type HTTPResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       string
	Bytes      []byte
	Error      error
}
