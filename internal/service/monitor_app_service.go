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

	"github.com/unimap-icp-hunter/project/internal/util/workerpool"
)

// URLReachabilityResult 单个 URL 可达性结果。
type URLReachabilityResult struct {
	Input      string `json:"input"`
	URL        string `json:"url,omitempty"`
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
type MonitorAppService struct{}

func NewMonitorAppService() *MonitorAppService {
	return &MonitorAppService{}
}

type reachabilityTaskPayload struct {
	index int
	item  URLReachabilityResult
}

type reachabilityTask struct {
	ctx        context.Context
	index      int
	input      string
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

	reachable, statusCode, reasonType, reason := probeURLReachabilityForService(probeCtx, normalizedURL)
	status := "reachable"
	if !reachable {
		status = "unreachable"
	}

	t.resultChan <- reachabilityTaskPayload{
		index: t.index,
		item: URLReachabilityResult{
			Input:      t.input,
			URL:        normalizedURL,
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

func probeURLReachabilityForService(ctx context.Context, targetURL string) (bool, int, string, string) {
	client := &http.Client{Timeout: 8 * time.Second}
	var headErr error

	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		errType, reason := classifyReachabilityErrorForService(err)
		return false, 0, errType, reason
	}

	headResp, err := client.Do(headReq)
	if err == nil {
		defer headResp.Body.Close()
		if headResp.StatusCode != http.StatusMethodNotAllowed {
			return true, headResp.StatusCode, "http_status", fmt.Sprintf("HTTP %d", headResp.StatusCode)
		}
	} else {
		headErr = err
	}

	getReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if reqErr != nil {
		if headErr != nil {
			errType, reason := classifyReachabilityErrorForService(headErr)
			return false, 0, errType, reason
		}
		errType, reason := classifyReachabilityErrorForService(reqErr)
		return false, 0, errType, reason
	}

	getResp, getErr := client.Do(getReq)
	if getErr != nil {
		if headErr != nil {
			errType, reason := classifyReachabilityErrorForService(headErr)
			return false, 0, errType, reason
		}
		errType, reason := classifyReachabilityErrorForService(getErr)
		return false, 0, errType, reason
	}
	defer getResp.Body.Close()

	return true, getResp.StatusCode, "http_status", fmt.Sprintf("HTTP %d", getResp.StatusCode)
}
