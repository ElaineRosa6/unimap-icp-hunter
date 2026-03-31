package service

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/proxypool"
	"github.com/unimap-icp-hunter/project/internal/util/workerpool"
)

// URLReachabilityResult 单个 URL 可达性结果。
type URLReachabilityResult struct {
	Input      string `json:"input"`
	URL        string `json:"url,omitempty"`
	Proxy      string `json:"proxy,omitempty"`
	Status     string `json:"status"`
	ReasonType string `json:"reason_type,omitempty"`
	Reachable  bool   `json:"reachable"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// URLReachabilitySummary 批量可达性统计。
type URLReachabilitySummary struct {
	Total         int `json:"total"`
	FormatValid   int `json:"formatValid"`
	InvalidFormat int `json:"invalidFormat"`
	Reachable     int `json:"reachable"`
	Unreachable   int `json:"unreachable"`
}

// URLReachabilityResponse 批量可达性响应。
type URLReachabilityResponse struct {
	Summary URLReachabilitySummary  `json:"summary"`
	Results []URLReachabilityResult `json:"results"`
}

// MonitorAppService 封装监控相关业务流程。
type MonitorAppService struct {
	proxyPool *proxypool.Pool
}

func NewMonitorAppService(pool *proxypool.Pool) *MonitorAppService {
	return &MonitorAppService{proxyPool: pool}
}

type reachabilityTaskPayload struct {
	index int
	item  URLReachabilityResult
}

type reachabilityTask struct {
	ctx        context.Context
	index      int
	input      string
	proxyPool  *proxypool.Pool
	resultChan chan<- reachabilityTaskPayload
	wg         *sync.WaitGroup
}

func (t *reachabilityTask) Execute() error {
	defer t.wg.Done()

	normalizedURL, normalizeErr := normalizeMonitorURLForService(t.input)
	if normalizeErr != nil {
		t.resultChan <- reachabilityTaskPayload{
			index: t.index,
			item: URLReachabilityResult{
				Input:      t.input,
				Status:     "invalid_format",
				ReasonType: "invalid_format",
				Reachable:  false,
				Reason:     normalizeErr.Error(),
			},
		}
		return nil
	}

	probeCtx, cancel := context.WithTimeout(t.ctx, 10*time.Second)
	defer cancel()

	reachable, statusCode, reasonType, reason, selectedProxy := probeURLReachabilityForService(probeCtx, normalizedURL, t.proxyPool)
	status := "reachable"
	if !reachable {
		status = "unreachable"
	}

	t.resultChan <- reachabilityTaskPayload{
		index: t.index,
		item: URLReachabilityResult{
			Input:      t.input,
			URL:        normalizedURL,
			Proxy:      selectedProxy,
			Status:     status,
			ReasonType: reasonType,
			Reachable:  reachable,
			HTTPStatus: statusCode,
			Reason:     reason,
		},
	}
	return nil
}

// CheckURLReachability 批量检查 URL 可达性。
func (s *MonitorAppService) CheckURLReachability(ctx context.Context, urls []string, concurrency int) (*URLReachabilityResponse, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}
	if concurrency <= 0 || concurrency > 10 {
		concurrency = 5
	}

	results := make([]URLReachabilityResult, len(urls))

	pool := workerpool.NewPool(concurrency)
	pool.Start()

	resultChan := make(chan reachabilityTaskPayload, len(urls))
	var wg sync.WaitGroup

	for i, rawURL := range urls {
		wg.Add(1)
		pool.Submit(&reachabilityTask{
			ctx:        ctx,
			index:      i,
			input:      rawURL,
			proxyPool:  s.proxyPool,
			resultChan: resultChan,
			wg:         &wg,
		})
	}

	go func() {
		wg.Wait()
		pool.Stop()
		close(resultChan)
	}()

	for item := range resultChan {
		results[item.index] = item.item
	}

	summary := URLReachabilitySummary{Total: len(results)}
	for _, result := range results {
		switch result.Status {
		case "invalid_format":
			summary.InvalidFormat++
		case "reachable":
			summary.FormatValid++
			summary.Reachable++
		case "unreachable":
			summary.FormatValid++
			summary.Unreachable++
		}
	}

	return &URLReachabilityResponse{Summary: summary, Results: results}, nil
}

func normalizeMonitorURLForService(rawURL string) (string, error) {
	urlText := strings.TrimSpace(rawURL)
	if urlText == "" {
		return "", fmt.Errorf("empty URL")
	}

	if !strings.HasPrefix(urlText, "http://") && !strings.HasPrefix(urlText, "https://") {
		urlText = "https://" + urlText
	}

	parsed, err := url.ParseRequestURI(urlText)
	if err != nil {
		return "", err
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme: %s", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("missing host")
	}
	return parsed.String(), nil
}

func classifyReachabilityErrorForService(err error) (string, string) {
	if err == nil {
		return "unknown", "unknown error"
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns", dnsErr.Error()
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout", netErr.Error()
	}

	var certErr x509.UnknownAuthorityError
	if errors.As(err, &certErr) {
		return "tls", err.Error()
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "tls") || strings.Contains(msg, "certificate") || strings.Contains(msg, "ssl"):
		return "tls", err.Error()
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "connrefused"):
		return "connection_refused", err.Error()
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out"):
		return "timeout", err.Error()
	case strings.Contains(msg, "name not resolved") || strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		return "dns", err.Error()
	default:
		return "network", err.Error()
	}
}

func probeURLReachabilityForService(ctx context.Context, targetURL string, pool *proxypool.Pool) (bool, int, string, string, string) {
	selectedProxy := ""
	if pool != nil {
		if proxyAddr, ok := pool.Select(); ok {
			selectedProxy = proxyAddr
		}
	}

	client, clientErr := buildReachabilityHTTPClient(selectedProxy)
	if clientErr != nil {
		if pool != nil && selectedProxy != "" {
			pool.Report(selectedProxy, false)
		}
		return false, 0, "proxy", clientErr.Error(), selectedProxy
	}

	succeeded := false
	defer func() {
		if pool != nil && selectedProxy != "" {
			pool.Report(selectedProxy, succeeded)
		}
	}()

	var headErr error

	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		errType, reason := classifyReachabilityErrorForService(err)
		return false, 0, errType, reason, selectedProxy
	}

	headResp, err := client.Do(headReq)
	if err == nil {
		defer headResp.Body.Close()
		if headResp.StatusCode != http.StatusMethodNotAllowed {
			succeeded = true
			return true, headResp.StatusCode, "http_status", fmt.Sprintf("HTTP %d", headResp.StatusCode), selectedProxy
		}
	} else {
		headErr = err
	}

	getReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if reqErr != nil {
		if headErr != nil {
			errType, reason := classifyReachabilityErrorForService(headErr)
			return false, 0, errType, reason, selectedProxy
		}
		errType, reason := classifyReachabilityErrorForService(reqErr)
		return false, 0, errType, reason, selectedProxy
	}

	getResp, getErr := client.Do(getReq)
	if getErr != nil {
		if headErr != nil {
			errType, reason := classifyReachabilityErrorForService(headErr)
			return false, 0, errType, reason, selectedProxy
		}
		errType, reason := classifyReachabilityErrorForService(getErr)
		return false, 0, errType, reason, selectedProxy
	}
	defer getResp.Body.Close()

	succeeded = true
	return true, getResp.StatusCode, "http_status", fmt.Sprintf("HTTP %d", getResp.StatusCode), selectedProxy
}

func buildReachabilityHTTPClient(proxyAddr string) (*http.Client, error) {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if strings.TrimSpace(proxyAddr) != "" {
		parsedProxy, err := url.Parse(proxyAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy address: %w", err)
		}
		transport.Proxy = http.ProxyURL(parsedProxy)
	}

	return &http.Client{Timeout: 8 * time.Second, Transport: transport}, nil
}
