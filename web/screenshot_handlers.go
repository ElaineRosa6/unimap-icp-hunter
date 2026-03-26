package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/metrics"
	"github.com/unimap-icp-hunter/project/internal/service"
)

func (s *Server) resolveScreenshotBaseDir() string {
	baseDir := "./screenshots"
	if s.config != nil && strings.TrimSpace(s.config.Screenshot.BaseDir) != "" {
		baseDir = s.config.Screenshot.BaseDir
	}
	if filepath.IsAbs(baseDir) {
		return filepath.Clean(baseDir)
	}
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return filepath.Clean(baseDir)
	}
	return absBaseDir
}

func (s *Server) screenshotPathToPreviewURL(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}

	absPath := filepath.Clean(path)
	if !filepath.IsAbs(absPath) {
		var err error
		absPath, err = filepath.Abs(absPath)
		if err != nil {
			return ""
		}
	}

	baseDir := s.resolveScreenshotBaseDir()
	relPath, err := filepath.Rel(baseDir, absPath)
	if err != nil {
		return ""
	}
	if relPath == "." || strings.HasPrefix(relPath, "..") {
		return ""
	}

	segments := strings.Split(filepath.ToSlash(relPath), "/")
	for idx, segment := range segments {
		segments[idx] = url.PathEscape(segment)
	}

	return "/screenshots/" + strings.Join(segments, "/")
}

func (s *Server) handleScreenshotFile(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	origin := r.Header.Get("Origin")
	referer := r.Header.Get("Referer")
	allowedOrigins := allowedOriginsFromConfig(s.config)
	if !isOriginAllowed(origin, r.Host, allowedOrigins) && !isOriginAllowed(referer, r.Host, allowedOrigins) {
		writeAPIError(w, http.StatusForbidden, "forbidden_origin", "origin not allowed", nil)
		return
	}

	relPath := strings.TrimPrefix(r.URL.Path, "/screenshots/")
	relPath = strings.TrimSpace(relPath)
	if relPath == "" || strings.HasSuffix(r.URL.Path, "/") {
		http.NotFound(w, r)
		return
	}

	cleanRelPath := filepath.Clean(filepath.FromSlash(relPath))
	if cleanRelPath == "." || strings.HasPrefix(cleanRelPath, "..") {
		writeAPIError(w, http.StatusBadRequest, "invalid_path", "invalid path", nil)
		return
	}

	ext := strings.ToLower(filepath.Ext(cleanRelPath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
	default:
		writeAPIError(w, http.StatusForbidden, "unsupported_file_type", "unsupported file type", nil)
		return
	}

	baseDir := s.resolveScreenshotBaseDir()
	fullPath := filepath.Join(baseDir, cleanRelPath)
	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_path", "invalid path", nil)
		return
	}

	relToBase, err := filepath.Rel(baseDir, absFullPath)
	if err != nil || relToBase == "." || strings.HasPrefix(relToBase, "..") {
		writeAPIError(w, http.StatusBadRequest, "invalid_path", "invalid path", nil)
		return
	}

	if _, err := os.Stat(absFullPath); err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, absFullPath)
}

// handleScreenshot 处理截图请求
func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requireTrustedRequest(w, r, allowedOriginsFromConfig(s.config)) {
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_url", "missing url parameter", nil)
		return
	}

	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeAPIError(w, http.StatusBadRequest, "invalid_url", "invalid url", nil)
		return
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(1365, 768),
	)

	if chromePath := strings.TrimSpace(os.Getenv("UNIMAP_CHROME_PATH")); chromePath != "" {
		st, statErr := os.Stat(chromePath)
		if statErr != nil || st.IsDir() {
			writeAPIError(w, http.StatusInternalServerError, "invalid_chrome_path", "invalid UNIMAP_CHROME_PATH", "file not found or not a file")
			return
		}
		opts = append(opts, chromedp.ExecPath(chromePath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var buf []byte
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.CaptureScreenshot(&buf),
	); err != nil {
		if strings.TrimSpace(os.Getenv("UNIMAP_CHROME_PATH")) == "" {
			writeAPIError(w, http.StatusInternalServerError, "screenshot_failed", "screenshot failed", map[string]string{
				"error": err.Error(),
				"hint":  "set UNIMAP_CHROME_PATH to your Chrome/Chromium executable path",
			})
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "screenshot_failed", "screenshot failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(buf)
}

// handleSearchEngineScreenshot 处理搜索引擎结果页面截图请求
func (s *Server) handleSearchEngineScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	if s.screenshotMgr == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "screenshot_manager_unavailable", "screenshot manager not initialized", nil)
		return
	}

	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	queryID := strings.TrimSpace(r.URL.Query().Get("query_id"))

	if engine == "" || query == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_parameters", "missing engine or query parameter", nil)
		return
	}

	if queryID == "" {
		queryID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	startTime := time.Now()
	screenshotPath, engine, query, queryID, err := s.screenshotApp.CaptureSearchEngineResult(r.Context(), s.screenshotMgr, engine, query, queryID)
	if err != nil {
		logger.Errorf("Failed to capture search engine screenshot: %v", err)
		metrics.IncScreenshotRequest("search_engine", "error")
		metrics.ObserveScreenshotDuration("search_engine", time.Since(startTime))
		writeAPIError(w, http.StatusInternalServerError, "screenshot_failed", "screenshot failed", err.Error())
		return
	}

	metrics.IncScreenshotRequest("search_engine", "success")
	metrics.ObserveScreenshotDuration("search_engine", time.Since(startTime))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"path":     screenshotPath,
		"engine":   engine,
		"query":    query,
		"query_id": queryID,
	})
}

// handleTargetScreenshot 处理目标网站截图请求（保存到文件）
func (s *Server) handleTargetScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	if s.screenshotMgr == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "screenshot_manager_unavailable", "screenshot manager not initialized", nil)
		return
	}
	if !requireTrustedRequest(w, r, allowedOriginsFromConfig(s.config)) {
		return
	}

	var req struct {
		URL      string `json:"url"`
		IP       string `json:"ip"`
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
		QueryID  string `json:"query_id"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	startTime := time.Now()
	screenshotPath, targetURL, ip, port, protocol, queryID, err := s.screenshotApp.CaptureTargetWebsite(
		r.Context(),
		s.screenshotMgr,
		req.URL,
		req.IP,
		req.Port,
		req.Protocol,
		req.QueryID,
	)
	if err != nil {
		logger.Errorf("Failed to capture target screenshot: %v", err)
		metrics.IncScreenshotRequest("target", "error")
		metrics.ObserveScreenshotDuration("target", time.Since(startTime))
		if strings.Contains(strings.ToLower(err.Error()), "missing url or ip") {
			writeAPIError(w, http.StatusBadRequest, "missing_parameters", "missing url or ip parameter", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "screenshot_failed", "screenshot failed", err.Error())
		return
	}

	metrics.IncScreenshotRequest("target", "success")
	metrics.ObserveScreenshotDuration("target", time.Since(startTime))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"path":     screenshotPath,
		"url":      targetURL,
		"ip":       ip,
		"port":     port,
		"protocol": protocol,
		"query_id": queryID,
	})
}

// handleBatchScreenshot 处理批量截图请求
func (s *Server) handleBatchScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	if s.screenshotMgr == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "screenshot_manager_unavailable", "screenshot manager not initialized", nil)
		return
	}
	if !requireTrustedRequest(w, r, allowedOriginsFromConfig(s.config)) {
		return
	}

	var req struct {
		QueryID string `json:"query_id"`
		Engines []struct {
			Engine string `json:"engine"`
			Query  string `json:"query"`
		} `json:"engines"`
		Targets []struct {
			URL      string `json:"url"`
			IP       string `json:"ip"`
			Port     string `json:"port"`
			Protocol string `json:"protocol"`
		} `json:"targets"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	appReq := service.BatchScreenshotRequest{QueryID: req.QueryID}
	for _, item := range req.Engines {
		appReq.Engines = append(appReq.Engines, struct {
			Engine string
			Query  string
		}{
			Engine: item.Engine,
			Query:  item.Query,
		})
	}
	for _, item := range req.Targets {
		appReq.Targets = append(appReq.Targets, struct {
			URL      string
			IP       string
			Port     string
			Protocol string
		}{
			URL:      item.URL,
			IP:       item.IP,
			Port:     item.Port,
			Protocol: item.Protocol,
		})
	}
	results, err := s.screenshotApp.CaptureBatch(r.Context(), s.screenshotMgr, appReq)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "batch_screenshot_failed", "batch screenshot failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// handleBatchURLsScreenshot 处理批量URL截图请求
func (s *Server) handleBatchURLsScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	if s.screenshotMgr == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "screenshot_manager_unavailable", "screenshot manager not initialized", nil)
		return
	}
	if !requireTrustedRequest(w, r, allowedOriginsFromConfig(s.config)) {
		return
	}

	var req struct {
		URLs        []string `json:"urls"`
		BatchID     string   `json:"batch_id"`
		Concurrency int      `json:"concurrency"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	metrics.IncBatchOperation("screenshot")
	metrics.ObserveBatchOperationSize("screenshot", len(req.URLs))

	results, err := s.screenshotApp.CaptureBatchURLs(r.Context(), s.screenshotMgr, service.BatchURLsRequest{
		URLs:        req.URLs,
		BatchID:     req.BatchID,
		Concurrency: req.Concurrency,
	})
	if err != nil {
		errText := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errText, "no urls"):
			writeAPIError(w, http.StatusBadRequest, "no_urls_provided", "no URLs provided", nil)
		case strings.Contains(errText, "too many"):
			writeAPIError(w, http.StatusBadRequest, "too_many_urls", "too many URLs", map[string]int{"max": 100})
		default:
			writeAPIError(w, http.StatusInternalServerError, "batch_screenshot_failed", "batch screenshot failed", err.Error())
		}
		return
	}

	// 记录批量截图成功/失败统计
	if results != nil {
		metrics.IncScreenshotRequest("batch", "success")
		metrics.ObserveScreenshotBatchSize(results.Success)
		if results.Failed > 0 {
			metrics.IncScreenshotRequest("batch", "partial")
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// handleBatchScreenshotPage 处理批量截图页面
func (s *Server) handleBatchScreenshotPage(w http.ResponseWriter, r *http.Request) {
	s.templates.ExecuteTemplate(w, "batch-screenshot.html", map[string]interface{}{
		"staticVersion": s.staticVersion,
	})
}

func normalizeScreenshotPathToken(raw string) (string, bool) {
	token := strings.TrimSpace(raw)
	if token == "" || token == "." || token == ".." {
		return "", false
	}
	if strings.Contains(token, "/") || strings.Contains(token, "\\") {
		return "", false
	}
	if filepath.Base(token) != token {
		return "", false
	}
	return token, true
}

func (s *Server) resolveScreenshotBatchDir(batch string) (string, bool) {
	batchToken, ok := normalizeScreenshotPathToken(batch)
	if !ok {
		return "", false
	}

	baseDir := s.resolveScreenshotBaseDir()
	target := filepath.Join(baseDir, batchToken)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(baseDir, absTarget)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return absTarget, true
}

func (s *Server) handleScreenshotBatches(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	batches, err := s.screenshotApp.ListBatches()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_batches_failed", "list screenshot batches failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"count":   len(batches),
		"batches": batches,
	})
}

func (s *Server) handleScreenshotBatchFiles(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	batch := strings.TrimSpace(r.URL.Query().Get("batch"))
	if batch == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_batch", "batch parameter is required", nil)
		return
	}

	files, err := s.screenshotApp.ListBatchFiles(batch, s.screenshotPathToPreviewURL)
	if err != nil {
		errText := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errText, "invalid batch"):
			writeAPIError(w, http.StatusBadRequest, "invalid_batch", "invalid batch name", nil)
		case strings.Contains(errText, "not found"):
			writeAPIError(w, http.StatusNotFound, "batch_not_found", "batch not found", nil)
		default:
			writeAPIError(w, http.StatusInternalServerError, "list_batch_files_failed", "list batch files failed", err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"batch":   batch,
		"count":   len(files),
		"files":   files,
	})
}

func (s *Server) handleScreenshotBatchDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	batch := strings.TrimSpace(r.URL.Query().Get("batch"))
	if batch == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_batch", "batch parameter is required", nil)
		return
	}

	if err := s.screenshotApp.DeleteBatch(batch); err != nil {
		errText := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errText, "invalid"):
			writeAPIError(w, http.StatusBadRequest, "invalid_batch", "invalid batch name", nil)
		case strings.Contains(errText, "not found"):
			writeAPIError(w, http.StatusNotFound, "batch_not_found", "batch not found", nil)
		default:
			writeAPIError(w, http.StatusInternalServerError, "delete_batch_failed", "delete batch failed", err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"batch":   batch,
	})
}

func (s *Server) handleScreenshotFileDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	batch := strings.TrimSpace(r.URL.Query().Get("batch"))
	fileName := strings.TrimSpace(r.URL.Query().Get("file"))

	if batch == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_batch", "batch parameter is required", nil)
		return
	}
	if fileName == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_file", "file parameter is required", nil)
		return
	}

	if err := s.screenshotApp.DeleteFile(batch, fileName); err != nil {
		errText := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errText, "invalid batch"):
			writeAPIError(w, http.StatusBadRequest, "invalid_batch", "invalid batch name", nil)
		case strings.Contains(errText, "invalid file"):
			writeAPIError(w, http.StatusBadRequest, "invalid_file", "invalid file name", nil)
		case strings.Contains(errText, "not found"):
			writeAPIError(w, http.StatusNotFound, "file_not_found", "file not found", nil)
		default:
			writeAPIError(w, http.StatusInternalServerError, "delete_file_failed", "delete file failed", err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"batch":   batch,
		"file":    fileName,
	})
}
