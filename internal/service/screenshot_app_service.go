package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/screenshot"
)

// ScreenshotAppService 封装截图相关应用层流程。
type ScreenshotAppService struct {
	baseDir       string
	provider      screenshot.Provider
	engine        string
	bridgeService *screenshot.BridgeService
	fallbackToCDP bool
}

func NewScreenshotAppService(baseDir string) *ScreenshotAppService {
	return NewScreenshotAppServiceWithProvider(baseDir, nil)
}

func NewScreenshotAppServiceWithProvider(baseDir string, provider screenshot.Provider) *ScreenshotAppService {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "./screenshots"
	}
	return &ScreenshotAppService{baseDir: baseDir, provider: provider, engine: "cdp"}
}

func (s *ScreenshotAppService) SetEngine(engine string) {
	if s == nil {
		return
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		engine = "cdp"
	}
	s.engine = engine
}

func (s *ScreenshotAppService) SetBridgeService(bridge *screenshot.BridgeService) {
	if s == nil {
		return
	}
	s.bridgeService = bridge
}

func (s *ScreenshotAppService) SetFallbackToCDP(enabled bool) {
	if s == nil {
		return
	}
	s.fallbackToCDP = enabled
}

// IsCaptureAvailable reports whether screenshot capture can run with current dependencies.
func (s *ScreenshotAppService) IsCaptureAvailable(mgr *screenshot.Manager) bool {
	if s != nil && s.provider != nil {
		return true
	}
	return mgr != nil
}

// GetBaseDir 获取截图基础目录
func (s *ScreenshotAppService) GetBaseDir() string {
	return s.baseDir
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
	engine = strings.TrimSpace(engine)
	query = strings.TrimSpace(query)
	if engine == "" || query == "" {
		return "", "", "", "", fmt.Errorf("missing engine or query parameter")
	}
	if strings.TrimSpace(queryID) == "" {
		queryID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	if strings.EqualFold(s.engine, "extension") && s.bridgeService != nil {
		path, bridgeErr := s.captureSearchEngineWithBridge(ctx, mgr, engine, query, queryID)
		if bridgeErr == nil {
			return path, engine, query, queryID, nil
		}
		if !s.fallbackToCDP {
			return "", "", "", "", bridgeErr
		}
	}

	provider, err := s.resolveProvider(mgr)
	if err != nil {
		return "", "", "", "", fmt.Errorf("screenshot manager not initialized")
	}
	path, err := provider.CaptureSearchEngineResult(ctx, engine, query, queryID)
	if err != nil {
		return "", "", "", "", err
	}
	return path, engine, query, queryID, nil
}

func (s *ScreenshotAppService) CaptureTargetWebsite(ctx context.Context, mgr *screenshot.Manager, targetURL, ip, port, protocol, queryID string) (string, string, string, string, string, string, error) {
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

	if strings.EqualFold(s.engine, "extension") && s.bridgeService != nil {
		path, bridgeErr := s.captureTargetWithBridge(ctx, targetURL, ip, port, protocol, queryID)
		if bridgeErr == nil {
			return path, targetURL, ip, port, protocol, queryID, nil
		}
		if !s.fallbackToCDP {
			return "", "", "", "", "", "", bridgeErr
		}
	}

	provider, err := s.resolveProvider(mgr)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("screenshot manager not initialized")
	}

	path, err := provider.CaptureTargetWebsite(ctx, targetURL, ip, port, protocol, queryID)
	if err != nil {
		return "", "", "", "", "", "", err
	}
	return path, targetURL, ip, port, protocol, queryID, nil
}

func (s *ScreenshotAppService) CaptureBatch(ctx context.Context, mgr *screenshot.Manager, req BatchScreenshotRequest) (*BatchScreenshotResponse, error) {
	provider, err := s.resolveProvider(mgr)
	if err != nil {
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
			path, err := provider.CaptureSearchEngineResult(ctx, engineName, query, req.QueryID)
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
			path, err := provider.CaptureTargetWebsite(ctx, url, ip, port, protocol, req.QueryID)
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

	if strings.EqualFold(s.engine, "extension") && s.bridgeService != nil {
		bridgeResp, bridgeErr := s.captureBatchURLsWithBridge(ctx, req)
		if bridgeErr == nil {
			return bridgeResp, nil
		}
		if !s.fallbackToCDP {
			return nil, bridgeErr
		}
	}

	provider, err := s.resolveProvider(mgr)
	if err != nil {
		return nil, fmt.Errorf("screenshot manager not initialized")
	}

	results, err := provider.CaptureBatchURLs(ctx, req.URLs, req.BatchID, req.Concurrency)
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
		ScreenshotDir: provider.GetScreenshotDirectory(),
	}, nil
}

func (s *ScreenshotAppService) captureBatchURLsWithBridge(ctx context.Context, req BatchURLsRequest) (*BatchURLsResponse, error) {
	if s.bridgeService == nil {
		return nil, fmt.Errorf("bridge service not initialized")
	}

	results := make([]screenshot.BatchScreenshotResult, len(req.URLs))
	sem := make(chan struct{}, req.Concurrency)
	var wg sync.WaitGroup

	for i, rawURL := range req.URLs {
		wg.Add(1)
		go func(idx int, inputURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			normalizedURL := normalizeBridgeTargetURL(inputURL)
			result := screenshot.BatchScreenshotResult{URL: inputURL, Timestamp: time.Now().Unix()}
			if normalizedURL == "" {
				result.Success = false
				result.Error = "invalid URL"
				results[idx] = result
				return
			}

			task := screenshot.BridgeTask{
				RequestID:    fmt.Sprintf("bridge_%d_%d", time.Now().UnixNano(), idx),
				URL:          normalizedURL,
				BatchID:      req.BatchID,
				WaitStrategy: "load",
			}
			bridgeResult, err := s.bridgeService.Submit(ctx, task)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
				results[idx] = result
				return
			}

			result.Success = bridgeResult.Success
			result.FilePath = bridgeResult.ImagePath
			if !bridgeResult.Success {
				if strings.TrimSpace(bridgeResult.Error) != "" {
					result.Error = bridgeResult.Error
				} else {
					result.Error = bridgeResult.ErrorCode
				}
			}
			results[idx] = result
		}(i, rawURL)
	}

	wg.Wait()

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
		ScreenshotDir: s.baseDir,
	}, nil
}

func normalizeBridgeTargetURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		trimmed = "http://" + trimmed
	}
	return trimmed
}

func buildSearchEngineResultURL(engine, query string) string {
	b64Query := base64.StdEncoding.EncodeToString([]byte(query))
	encodedB64 := url.QueryEscape(b64Query)
	encodedQuery := url.QueryEscape(query)

	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "fofa":
		return fmt.Sprintf("https://fofa.info/result?qbase64=%s", encodedB64)
	case "hunter":
		return fmt.Sprintf("https://hunter.qianxin.com/list?search=%s", encodedB64)
	case "quake":
		return fmt.Sprintf("https://quake.360.net/quake/#/searchResult?search=%s", encodedQuery)
	case "zoomeye":
		return fmt.Sprintf("https://www.zoomeye.hk/searchResult?q=%s", encodedQuery)
	default:
		return ""
	}
}

func (s *ScreenshotAppService) captureSearchEngineWithBridge(ctx context.Context, mgr *screenshot.Manager, engine, query, queryID string) (string, error) {
	if s == nil || s.bridgeService == nil {
		return "", fmt.Errorf("bridge service not initialized")
	}

	searchURL := ""
	if mgr != nil {
		searchURL = strings.TrimSpace(mgr.BuildSearchEngineURL(engine, query))
	}
	if searchURL == "" {
		searchURL = buildSearchEngineResultURL(engine, query)
	}
	if searchURL == "" {
		return "", fmt.Errorf("unsupported engine: %s", engine)
	}

	task := screenshot.BridgeTask{
		RequestID:    fmt.Sprintf("bridge_search_%d", time.Now().UnixNano()),
		URL:          searchURL,
		BatchID:      queryID,
		WaitStrategy: "load",
	}
	result, err := s.bridgeService.Submit(ctx, task)
	if err != nil {
		return "", err
	}
	if !result.Success {
		if strings.TrimSpace(result.Error) != "" {
			return "", fmt.Errorf("bridge capture failed: %s", strings.TrimSpace(result.Error))
		}
		if strings.TrimSpace(result.ErrorCode) != "" {
			return "", fmt.Errorf("bridge capture failed: %s", strings.TrimSpace(result.ErrorCode))
		}
		return "", fmt.Errorf("bridge capture failed")
	}
	if strings.TrimSpace(result.ImagePath) == "" {
		return "", fmt.Errorf("bridge capture missing image path")
	}
	return strings.TrimSpace(result.ImagePath), nil
}

func buildTargetCaptureURL(targetURL, ip, port, protocol string) (string, error) {
	resolvedURL := strings.TrimSpace(targetURL)
	resolvedIP := strings.TrimSpace(ip)
	resolvedPort := strings.TrimSpace(port)
	resolvedProto := strings.TrimSpace(protocol)

	if resolvedURL == "" {
		if resolvedIP == "" {
			return "", fmt.Errorf("target URL or IP is required")
		}
		proto := "http"
		if resolvedProto != "" {
			proto = strings.ToLower(resolvedProto)
		} else if resolvedPort == "443" {
			proto = "https"
		}

		if resolvedPort != "" && resolvedPort != "80" && resolvedPort != "443" {
			resolvedURL = fmt.Sprintf("%s://%s:%s", proto, resolvedIP, resolvedPort)
		} else {
			resolvedURL = fmt.Sprintf("%s://%s", proto, resolvedIP)
		}
	}

	if !strings.HasPrefix(resolvedURL, "http://") && !strings.HasPrefix(resolvedURL, "https://") {
		resolvedURL = "http://" + resolvedURL
	}

	return resolvedURL, nil
}

func (s *ScreenshotAppService) captureTargetWithBridge(ctx context.Context, targetURL, ip, port, protocol, queryID string) (string, error) {
	if s == nil || s.bridgeService == nil {
		return "", fmt.Errorf("bridge service not initialized")
	}

	resolvedURL, err := buildTargetCaptureURL(targetURL, ip, port, protocol)
	if err != nil {
		return "", err
	}

	task := screenshot.BridgeTask{
		RequestID:    fmt.Sprintf("bridge_target_%d", time.Now().UnixNano()),
		URL:          resolvedURL,
		BatchID:      queryID,
		WaitStrategy: "load",
	}
	result, err := s.bridgeService.Submit(ctx, task)
	if err != nil {
		return "", err
	}
	if !result.Success {
		if strings.TrimSpace(result.Error) != "" {
			return "", fmt.Errorf("bridge capture failed: %s", strings.TrimSpace(result.Error))
		}
		if strings.TrimSpace(result.ErrorCode) != "" {
			return "", fmt.Errorf("bridge capture failed: %s", strings.TrimSpace(result.ErrorCode))
		}
		return "", fmt.Errorf("bridge capture failed")
	}
	if strings.TrimSpace(result.ImagePath) == "" {
		return "", fmt.Errorf("bridge capture missing image path")
	}
	return strings.TrimSpace(result.ImagePath), nil
}

func (s *ScreenshotAppService) resolveProvider(mgr *screenshot.Manager) (screenshot.Provider, error) {
	if s != nil && s.provider != nil {
		return s.provider, nil
	}
	if mgr != nil {
		return screenshot.NewCDPProvider(mgr), nil
	}
	return nil, fmt.Errorf("screenshot manager not initialized")
}

// BatchInfo 批次信息
type BatchInfo struct {
	Name      string `json:"name"`
	FileCount int    `json:"file_count"`
	UpdatedAt int64  `json:"updated_at"`
}

// FileInfo 文件信息
type FileInfo struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	UpdatedAt  int64  `json:"updated_at"`
	PreviewURL string `json:"preview_url,omitempty"`
}

// ListBatches 列出所有截图批次
func (s *ScreenshotAppService) ListBatches() ([]BatchInfo, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BatchInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read screenshot directory: %w", err)
	}

	batches := make([]BatchInfo, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}

		fileCount := 0
		children, childErr := os.ReadDir(filepath.Join(s.baseDir, entry.Name()))
		if childErr == nil {
			for _, child := range children {
				if !child.IsDir() {
					fileCount++
				}
			}
		}

		batches = append(batches, BatchInfo{
			Name:      entry.Name(),
			FileCount: fileCount,
			UpdatedAt: info.ModTime().Unix(),
		})
	}

	sort.Slice(batches, func(i, j int) bool {
		return batches[i].UpdatedAt > batches[j].UpdatedAt
	})

	return batches, nil
}

// ListBatchFiles 列出指定批次的文件
func (s *ScreenshotAppService) ListBatchFiles(batch string, previewURLBuilder func(string) string) ([]FileInfo, error) {
	batchToken := s.normalizePathToken(batch)
	if batchToken == "" {
		return nil, fmt.Errorf("invalid batch name")
	}

	batchDir := filepath.Join(s.baseDir, batchToken)
	absBatchDir, err := filepath.Abs(batchDir)
	if err != nil {
		return nil, fmt.Errorf("invalid batch path")
	}

	// 安全检查：确保目录在 baseDir 内
	absBaseDir, _ := filepath.Abs(s.baseDir)
	rel, err := filepath.Rel(absBaseDir, absBatchDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("invalid batch path")
	}

	entries, err := os.ReadDir(absBatchDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("batch not found")
		}
		return nil, fmt.Errorf("failed to read batch directory: %w", err)
	}

	files := make([]FileInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}

		absPath := filepath.Join(absBatchDir, entry.Name())
		previewURL := ""
		if previewURLBuilder != nil {
			previewURL = previewURLBuilder(absPath)
		}

		files = append(files, FileInfo{
			Name:       entry.Name(),
			Size:       info.Size(),
			UpdatedAt:  info.ModTime().Unix(),
			PreviewURL: previewURL,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].UpdatedAt > files[j].UpdatedAt
	})

	return files, nil
}

// DeleteBatch 删除指定批次
func (s *ScreenshotAppService) DeleteBatch(batch string) error {
	batchToken := s.normalizePathToken(batch)
	if batchToken == "" {
		return fmt.Errorf("invalid batch name")
	}

	batchDir := filepath.Join(s.baseDir, batchToken)
	absBatchDir, err := filepath.Abs(batchDir)
	if err != nil {
		return fmt.Errorf("invalid batch path")
	}

	// 安全检查
	absBaseDir, _ := filepath.Abs(s.baseDir)
	rel, err := filepath.Rel(absBaseDir, absBatchDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("invalid batch path")
	}

	if _, err := os.Stat(absBatchDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("batch not found")
		}
		return fmt.Errorf("failed to access batch: %w", err)
	}

	return os.RemoveAll(absBatchDir)
}

// DeleteFile 删除指定批次中的文件
func (s *ScreenshotAppService) DeleteFile(batch, fileName string) error {
	batchToken := s.normalizePathToken(batch)
	if batchToken == "" {
		return fmt.Errorf("invalid batch name")
	}

	fileToken := s.normalizePathToken(fileName)
	if fileToken == "" {
		return fmt.Errorf("invalid file name")
	}

	batchDir := filepath.Join(s.baseDir, batchToken)
	absBatchDir, err := filepath.Abs(batchDir)
	if err != nil {
		return fmt.Errorf("invalid batch path")
	}

	// 安全检查
	absBaseDir, _ := filepath.Abs(s.baseDir)
	rel, err := filepath.Rel(absBaseDir, absBatchDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("invalid batch path")
	}

	targetFile := filepath.Join(absBatchDir, fileToken)
	absTarget, err := filepath.Abs(targetFile)
	if err != nil {
		return fmt.Errorf("invalid file path")
	}

	// 安全检查：确保文件在批次目录内
	relFile, err := filepath.Rel(absBatchDir, absTarget)
	if err != nil || relFile == "." || strings.HasPrefix(relFile, "..") {
		return fmt.Errorf("invalid file path")
	}

	info, err := os.Stat(absTarget)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found")
		}
		return fmt.Errorf("failed to access file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("file name points to a directory")
	}

	return os.Remove(absTarget)
}

// normalizePathToken 规范化路径令牌，防止路径穿越
func (s *ScreenshotAppService) normalizePathToken(raw string) string {
	token := strings.TrimSpace(raw)
	if token == "" || token == "." || token == ".." {
		return ""
	}
	if strings.Contains(token, "/") || strings.Contains(token, "\\") {
		return ""
	}
	if filepath.Base(token) != token {
		return ""
	}
	return token
}
