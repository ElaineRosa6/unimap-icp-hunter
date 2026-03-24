package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/screenshot"
)

// ScreenshotAppService 封装截图相关应用层流程。
type ScreenshotAppService struct{}

func NewScreenshotAppService() *ScreenshotAppService {
	return &ScreenshotAppService{}
}

type BatchScreenshotRequest struct {
	QueryID string
	Engines []struct {
		Engine string
		Query  string
	}
	Targets []struct {
		URL      string
		IP       string
		Port     string
		Protocol string
	}
}

type BatchScreenshotResponse struct {
	QueryID       string                   `json:"query_id"`
	SearchEngines []map[string]interface{} `json:"search_engines"`
	Targets       []map[string]interface{} `json:"targets"`
	Errors        []string                 `json:"errors"`
}

type BatchURLsRequest struct {
	URLs        []string
	BatchID     string
	Concurrency int
}

type BatchURLsResponse struct {
	BatchID       string                             `json:"batch_id"`
	Total         int                                `json:"total"`
	Success       int                                `json:"success"`
	Failed        int                                `json:"failed"`
	Results       []screenshot.BatchScreenshotResult `json:"results"`
	ScreenshotDir string                             `json:"screenshot_dir"`
}

func (s *ScreenshotAppService) CaptureSearchEngineResult(ctx context.Context, mgr *screenshot.Manager, engine, query, queryID string) (string, string, string, string, error) {
	if mgr == nil {
		return "", "", "", "", fmt.Errorf("screenshot manager not initialized")
	}
	engine = strings.TrimSpace(engine)
	query = strings.TrimSpace(query)
	if engine == "" || query == "" {
		return "", "", "", "", fmt.Errorf("missing engine or query parameter")
	}
	if strings.TrimSpace(queryID) == "" {
		queryID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	path, err := mgr.CaptureSearchEngineResult(ctx, engine, query, queryID)
	if err != nil {
		return "", "", "", "", err
	}
	return path, engine, query, queryID, nil
}

func (s *ScreenshotAppService) CaptureTargetWebsite(ctx context.Context, mgr *screenshot.Manager, targetURL, ip, port, protocol, queryID string) (string, string, string, string, string, string, error) {
	if mgr == nil {
		return "", "", "", "", "", "", fmt.Errorf("screenshot manager not initialized")
	}
	targetURL = strings.TrimSpace(targetURL)
	ip = strings.TrimSpace(ip)
	port = strings.TrimSpace(port)
	protocol = strings.TrimSpace(protocol)
	queryID = strings.TrimSpace(queryID)
	if targetURL == "" && ip == "" {
		return "", "", "", "", "", "", fmt.Errorf("missing url or ip parameter")
	}
	if queryID == "" {
		queryID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	path, err := mgr.CaptureTargetWebsite(ctx, targetURL, ip, port, protocol, queryID)
	if err != nil {
		return "", "", "", "", "", "", err
	}
	return path, targetURL, ip, port, protocol, queryID, nil
}

func (s *ScreenshotAppService) CaptureBatch(ctx context.Context, mgr *screenshot.Manager, req BatchScreenshotRequest) (*BatchScreenshotResponse, error) {
	if mgr == nil {
		return nil, fmt.Errorf("screenshot manager not initialized")
	}
	if strings.TrimSpace(req.QueryID) == "" {
		req.QueryID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	resp := &BatchScreenshotResponse{
		QueryID:       req.QueryID,
		SearchEngines: []map[string]interface{}{},
		Targets:       []map[string]interface{}{},
		Errors:        []string{},
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, engine := range req.Engines {
		wg.Add(1)
		go func(engineName, query string) {
			defer wg.Done()
			path, err := mgr.CaptureSearchEngineResult(ctx, engineName, query, req.QueryID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				resp.Errors = append(resp.Errors, fmt.Sprintf("%s: %v", engineName, err))
				return
			}
			resp.SearchEngines = append(resp.SearchEngines, map[string]interface{}{
				"engine": engineName,
				"query":  query,
				"path":   path,
			})
		}(engine.Engine, engine.Query)
	}

	for _, target := range req.Targets {
		wg.Add(1)
		go func(url, ip, port, protocol string) {
			defer wg.Done()
			path, err := mgr.CaptureTargetWebsite(ctx, url, ip, port, protocol, req.QueryID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				resp.Errors = append(resp.Errors, fmt.Sprintf("%s:%s: %v", ip, port, err))
				return
			}
			resp.Targets = append(resp.Targets, map[string]interface{}{
				"url":      url,
				"ip":       ip,
				"port":     port,
				"protocol": protocol,
				"path":     path,
			})
		}(target.URL, target.IP, target.Port, target.Protocol)
	}

	wg.Wait()
	return resp, nil
}

func (s *ScreenshotAppService) CaptureBatchURLs(ctx context.Context, mgr *screenshot.Manager, req BatchURLsRequest) (*BatchURLsResponse, error) {
	if mgr == nil {
		return nil, fmt.Errorf("screenshot manager not initialized")
	}
	if len(req.URLs) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}
	if len(req.URLs) > 100 {
		return nil, fmt.Errorf("too many URLs")
	}
	if strings.TrimSpace(req.BatchID) == "" {
		req.BatchID = fmt.Sprintf("batch_%d", time.Now().UnixNano())
	}
	if req.Concurrency <= 0 || req.Concurrency > 10 {
		req.Concurrency = 5
	}

	results, err := mgr.CaptureBatchURLs(ctx, req.URLs, req.BatchID, req.Concurrency)
	if err != nil {
		return nil, err
	}

	successCount := 0
	failCount := 0
	for _, item := range results {
		if item.Success {
			successCount++
		} else {
			failCount++
		}
	}

	return &BatchURLsResponse{
		BatchID:       req.BatchID,
		Total:         len(req.URLs),
		Success:       successCount,
		Failed:        failCount,
		Results:       results,
		ScreenshotDir: mgr.GetScreenshotDirectory(),
	}, nil
}
