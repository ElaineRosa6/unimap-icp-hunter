package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/unimap/project/internal/config"
	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/metrics"
	"github.com/unimap/project/internal/screenshot"
	"github.com/unimap/project/internal/screenshot/batchdb"
	"github.com/unimap/project/internal/service"
	"github.com/unimap/project/internal/utils"
)

// ============================================================
// Batch Screenshot Job Store (P1-4: async progress tracking)
// ============================================================

type batchJobStatus string

const (
	batchJobRunning   batchJobStatus = "running"
	batchJobCompleted batchJobStatus = "completed"
	batchJobFailed    batchJobStatus = "failed"
)

type batchJob struct {
	ID               string                             `json:"id"`
	Status           batchJobStatus                     `json:"status"`
	Total            int                                `json:"total"`
	Completed        int                                `json:"completed"`
	Success          int                                `json:"success"`
	Failed           int                                `json:"failed"`
	Results          []screenshot.BatchScreenshotResult `json:"results,omitempty"`
	Error            string                             `json:"error,omitempty"`
	PersistenceError string                             `json:"persistence_error,omitempty"`
	StartedAt        time.Time                          `json:"started_at"`
	EndedAt          *time.Time                         `json:"ended_at,omitempty"`
}

type batchJobStore struct {
	mu   sync.RWMutex
	jobs map[string]*batchJob
	repo *batchdb.Repository // optional persistent backend; nil = memory-only
}

func newBatchJobStore() *batchJobStore {
	return &batchJobStore{jobs: make(map[string]*batchJob)}
}

// setRepo attaches a persistent repository and loads prior jobs from disk.
func (s *batchJobStore) setRepo(repo *batchdb.Repository) {
	s.mu.Lock()
	s.repo = repo
	s.mu.Unlock()
	s.loadFromDB()
}

// persistLocked writes the current job snapshot to the DB.
// Must be called with s.mu held (at least read-locked); no-op if repo is nil.
func (s *batchJobStore) persistLocked(job *batchJob) error {
	if s.repo == nil || job == nil {
		return nil
	}
	if err := s.repo.SaveJob(batchJobRecord(job)); err != nil {
		logger.Warnf("screenshot: failed to persist batch job %s: %v", job.ID, err)
		return err
	}
	return nil
}

func batchJobRecord(job *batchJob) *batchdb.BatchJobRecord {
	return &batchdb.BatchJobRecord{
		ID:        job.ID,
		Status:    string(job.Status),
		Total:     job.Total,
		Completed: job.Completed,
		Success:   job.Success,
		Failed:    job.Failed,
		Error:     job.Error,
		Results:   job.Results,
		StartedAt: job.StartedAt,
		EndedAt:   job.EndedAt,
	}
}

// loadFromDB restores completed/failed jobs from the DB into the in-memory map.
// Jobs that were "running" at shutdown are marked "failed" (their goroutine is gone).
func (s *batchJobStore) loadFromDB() {
	if s.repo == nil {
		return
	}
	records, err := s.repo.ListJobs(200)
	if err != nil {
		logger.Warnf("screenshot: failed to load batch jobs from DB: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range records {
		status := batchJobStatus(rec.Status)
		if status == batchJobRunning {
			status = batchJobFailed // goroutine lost on restart
			rec.Status = string(status)
			rec.Error = "interrupted by server restart"
			_ = s.repo.SaveJob(&batchdb.BatchJobRecord{
				ID: rec.ID, Status: rec.Status, Total: rec.Total, Completed: rec.Completed,
				Success: rec.Success, Failed: rec.Failed, Error: rec.Error,
				Results: rec.Results, StartedAt: rec.StartedAt, EndedAt: rec.EndedAt,
			})
		}
		s.jobs[rec.ID] = &batchJob{
			ID:        rec.ID,
			Status:    status,
			Total:     rec.Total,
			Completed: rec.Completed,
			Success:   rec.Success,
			Failed:    rec.Failed,
			Results:   rec.Results,
			Error:     rec.Error,
			StartedAt: rec.StartedAt,
			EndedAt:   rec.EndedAt,
		}
	}
	if len(records) > 0 {
		logger.Infof("screenshot: restored %d batch jobs from DB", len(records))
	}
}

func (s *batchJobStore) create(id string, total int) (*batchJob, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[id]; exists {
		return nil, false, nil
	}
	job := &batchJob{
		ID:        id,
		Status:    batchJobRunning,
		Total:     total,
		StartedAt: time.Now(),
	}
	s.jobs[id] = job
	if s.repo != nil {
		created, err := s.repo.CreateJob(batchJobRecord(job))
		if err != nil {
			delete(s.jobs, id)
			return nil, false, err
		}
		if !created {
			delete(s.jobs, id)
			return nil, false, nil
		}
	}
	return job, true, nil
}

// getSnapshot returns a deep copy of the job to avoid data races when
// the caller serializes it while background goroutines mutate the original.
func (s *batchJobStore) getSnapshot(id string) *batchJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil
	}
	cp := *job
	if job.Results != nil {
		cp.Results = make([]screenshot.BatchScreenshotResult, len(job.Results))
		copy(cp.Results, job.Results)
	}
	if job.EndedAt != nil {
		t := *job.EndedAt
		cp.EndedAt = &t
	}
	return &cp
}

func (s *batchJobStore) recordResult(id string, result screenshot.BatchScreenshotResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Results = append(job.Results, result)
		job.Completed = len(job.Results)
		if result.Success {
			job.Success++
		} else {
			job.Failed++
		}
		if err := s.persistLocked(job); err != nil {
			job.PersistenceError = err.Error()
		}
	}
}

func (s *batchJobStore) complete(id string, results []screenshot.BatchScreenshotResult, success, failed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = batchJobCompleted
		job.Results = results
		job.Success = success
		job.Failed = failed
		job.Completed = len(results)
		now := time.Now()
		job.EndedAt = &now
		if err := s.persistLocked(job); err != nil {
			job.PersistenceError = err.Error()
		}
	}
}

func (s *batchJobStore) fail(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Status = batchJobFailed
		job.Error = err.Error()
		now := time.Now()
		job.EndedAt = &now
		if persistErr := s.persistLocked(job); persistErr != nil {
			job.PersistenceError = persistErr.Error()
		}
	}
}

// cleanup removes jobs older than maxAge.
func (s *batchJobStore) cleanup(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for id, job := range s.jobs {
		if job.EndedAt != nil && job.EndedAt.Before(cutoff) {
			delete(s.jobs, id)
		}
	}
}

func (s *Server) resolveScreenshotBaseDir() string {
	baseDir := utils.ScreenshotsDir()
	if cfg := s.currentConfig(); cfg != nil && strings.TrimSpace(cfg.Screenshot.BaseDir) != "" {
		baseDir = cfg.Screenshot.BaseDir
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
	allowedOrigins := s.allowedOrigins()
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

	// Resolve every component relative to an open root, including symlinks.
	// Lexical filepath.Rel checks alone do not constrain filesystem targets.
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	file, err := root.Open(cleanRelPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	header := make([]byte, 512)
	n, _ := file.Read(header)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.NotFound(w, r)
		return
	}
	if n > 0 {
		w.Header().Set("Content-Type", http.DetectContentType(header[:n]))
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	// Serve the same validated handle; ServeFile would reopen the pathname.
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

// handleScreenshot 处理截图请求
func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
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

	if isPrivateOrInternalIP(parsed.Hostname()) {
		writeAPIError(w, http.StatusForbidden, "blocked_url", "url resolves to private/internal address", nil)
		return
	}

	if s.screenshotRouter == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "screenshot_router_unavailable", "screenshot router not initialized", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	queryID := fmt.Sprintf("single_%d", time.Now().UnixNano())
	screenshotPath, err := s.screenshotRouter.CaptureTargetWebsite(ctx, targetURL, "", "", "", queryID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "screenshot_failed", "screenshot failed", sanitizeError(err.Error()))
		return
	}

	imgData, err := os.ReadFile(screenshotPath)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "screenshot_read_failed", "failed to read screenshot file", sanitizeError(err.Error()))
		return
	}

	contentType := http.DetectContentType(imgData)
	if contentType != "image/png" {
		writeAPIError(w, http.StatusInternalServerError, "screenshot_format_invalid", "screenshot provider returned a non-PNG image", nil)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(imgData)
}

// handleSearchEngineScreenshot 处理搜索引擎结果页面截图请求
func (s *Server) handleSearchEngineScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
		return
	}

	if s.screenshotApp == nil || !s.screenshotApp.IsCaptureAvailable(s.screenshotMgr) {
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
	if err := screenshot.ValidateIdentifier(queryID); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_query_id", "query_id is invalid", nil)
		return
	}

	startTime := time.Now()

	var screenshotPath string
	var err error

	proxy := ""
	if s.screenshotRouter == nil || s.screenshotRouter.SupportsRequestProxy() {
		proxy = s.selectRequestProxy()
	}
	screenshotPath, _, _, _, err = s.screenshotApp.CaptureSearchEngineResultWithProxy(r.Context(), s.screenshotMgr, engine, query, queryID, proxy)
	if proxy != "" {
		s.reportRequestProxy(proxy, err == nil)
	}

	if err != nil {
		logger.Errorf("Failed to capture search engine screenshot: %v", err)
		metrics.IncScreenshotRequest("search_engine", "error")
		metrics.ObserveScreenshotDuration("search_engine", time.Since(startTime))
		writeAPIError(w, http.StatusInternalServerError, "screenshot_failed", "screenshot failed", sanitizeError(err.Error()))
		return
	}

	metrics.IncScreenshotRequest("search_engine", "success")
	metrics.ObserveScreenshotDuration("search_engine", time.Since(startTime))

	type screenshotResponse struct {
		Success bool   `json:"success"`
		Path    string `json:"path"`
		Engine  string `json:"engine"`
		Query   string `json:"query"`
		QueryID string `json:"query_id"`
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(screenshotResponse{
		Success: true, Path: screenshotPath, Engine: engine, Query: query, QueryID: queryID,
	})
}

// handleTargetScreenshot 处理目标网站截图请求（保存到文件）
func (s *Server) handleTargetScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	if s.screenshotApp == nil || !s.screenshotApp.IsCaptureAvailable(s.screenshotMgr) {
		writeAPIError(w, http.StatusServiceUnavailable, "screenshot_manager_unavailable", "screenshot manager not initialized", nil)
		return
	}
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
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
	if req.QueryID != "" {
		if err := screenshot.ValidateIdentifier(req.QueryID); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_query_id", "query_id is invalid", nil)
			return
		}
	}

	if req.URL != "" {
		if parsed, err := url.Parse(req.URL); err == nil && isPrivateOrInternalIP(parsed.Hostname()) {
			writeAPIError(w, http.StatusForbidden, "blocked_url", "url resolves to private/internal address", nil)
			return
		}
	}
	// SSRF: also validate IP when provided without URL
	if req.URL == "" && req.IP != "" {
		if isPrivateOrInternalIP(req.IP) {
			writeAPIError(w, http.StatusForbidden, "blocked_url", "ip resolves to private/internal address", nil)
			return
		}
	}

	startTime := time.Now()

	var screenshotPath, targetURL, ip, port, protocol, queryID string
	var err error

	proxy := ""
	if s.screenshotRouter == nil || s.screenshotRouter.SupportsRequestProxy() {
		proxy = s.selectRequestProxy()
	}
	screenshotPath, targetURL, ip, port, protocol, queryID, err = s.screenshotApp.CaptureTargetWebsiteWithProxy(
		r.Context(), s.screenshotMgr, req.URL, req.IP, req.Port, req.Protocol, req.QueryID, proxy,
	)
	if proxy != "" {
		s.reportRequestProxy(proxy, err == nil)
	}
	if err != nil {
		logger.Errorf("Failed to capture target screenshot: %v", err)
		metrics.IncScreenshotRequest("target", "error")
		metrics.ObserveScreenshotDuration("target", time.Since(startTime))
		if strings.Contains(strings.ToLower(err.Error()), "missing url or ip") {
			writeAPIError(w, http.StatusBadRequest, "missing_parameters", "missing url or ip parameter", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "screenshot_failed", "screenshot failed", sanitizeError(err.Error()))
		return
	}

	metrics.IncScreenshotRequest("target", "success")
	metrics.ObserveScreenshotDuration("target", time.Since(startTime))

	type targetScreenshotResponse struct {
		Success  bool   `json:"success"`
		Path     string `json:"path"`
		URL      string `json:"url"`
		IP       string `json:"ip"`
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
		QueryID  string `json:"query_id"`
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(targetScreenshotResponse{
		Success: true, Path: screenshotPath, URL: targetURL, IP: ip,
		Port: port, Protocol: protocol, QueryID: queryID,
	})
}

// handleBatchScreenshot 处理批量截图请求
func (s *Server) handleBatchScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	if s.screenshotApp == nil || !s.screenshotApp.IsCaptureAvailable(s.screenshotMgr) {
		writeAPIError(w, http.StatusServiceUnavailable, "screenshot_manager_unavailable", "screenshot manager not initialized", nil)
		return
	}
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
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
	if req.QueryID != "" {
		if err := screenshot.ValidateIdentifier(req.QueryID); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_query_id", "query_id is invalid", nil)
			return
		}
	}

	// SSRF: validate target URLs
	for _, t := range req.Targets {
		if t.URL != "" {
			if parsed, err := url.Parse(t.URL); err == nil && isPrivateOrInternalIP(parsed.Hostname()) {
				writeAPIError(w, http.StatusForbidden, "blocked_url", "target url resolves to private/internal address", nil)
				return
			}
		} else if t.IP != "" {
			if isPrivateOrInternalIP(t.IP) {
				writeAPIError(w, http.StatusForbidden, "blocked_url", "target ip resolves to private/internal address", nil)
				return
			}
		}
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
		writeAPIError(w, http.StatusInternalServerError, "batch_screenshot_failed", "batch screenshot failed", sanitizeError(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

// handleBatchURLsScreenshot 处理批量URL截图请求（P1-4: 异步执行，返回 job ID）
func (s *Server) handleBatchURLsScreenshot(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if s.screenshotApp == nil || !s.screenshotApp.IsCaptureAvailable(s.screenshotMgr) {
		writeAPIError(w, http.StatusServiceUnavailable, "screenshot_manager_unavailable", "screenshot manager not initialized", nil)
		return
	}
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
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
	if len(req.URLs) == 0 {
		writeAPIError(w, http.StatusBadRequest, "no_urls", "no URLs provided", nil)
		return
	}
	validItems, invalidResults := classifyBatchURLs(req.URLs)
	validURLs := make([]string, 0, len(validItems))
	for _, item := range validItems {
		validURLs = append(validURLs, item.URL)
	}

	metrics.IncBatchOperation("screenshot")
	metrics.ObserveBatchOperationSize("screenshot", len(req.URLs))

	// 生成 job ID，创建 job，立即返回
	jobID := req.BatchID
	if jobID == "" {
		jobID = fmt.Sprintf("batch_%d", time.Now().UnixNano())
	}
	if err := screenshot.ValidateIdentifier(jobID); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_batch_id", "batch_id is invalid", nil)
		return
	}
	if _, created, err := s.batchJobs.create(jobID, len(req.URLs)); err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "batch_persistence_unavailable", "failed to persist batch job", nil)
		return
	} else if !created {
		writeAPIError(w, http.StatusConflict, "batch_id_conflict", "batch_id already exists", nil)
		return
	}
	for _, item := range invalidResults {
		s.batchJobs.recordResult(jobID, item.Result)
	}

	if len(validURLs) == 0 {
		finalResults, successCount, failedCount := mergeBatchURLResults(len(req.URLs), validItems, invalidResults, nil)
		s.batchJobs.complete(jobID, finalResults, successCount, failedCount)
		type batchResponse struct {
			JobID  string `json:"job_id"`
			Total  int    `json:"total"`
			Status string `json:"status"`
		}
		writeJSON(w, http.StatusAccepted, batchResponse{JobID: jobID, Total: len(req.URLs), Status: "completed"})
		return
	}
	req.URLs = validURLs

	// 后台异步执行 — 使用 shutdownCtx 确保服务器关闭时取消
	go func() {
		ctx, cancel := context.WithCancel(s.shutdownCtx)
		defer cancel()
		results, err := s.executeBatchURLScreenshot(ctx, &req, func(result screenshot.BatchScreenshotResult) {
			s.batchJobs.recordResult(jobID, result)
		})
		if err != nil {
			s.batchJobs.fail(jobID, err)
			return
		}
		if results != nil {
			finalResults, successCount, failedCount := mergeBatchURLResults(len(req.URLs)+len(invalidResults), validItems, invalidResults, results.Results)
			if failedCount > 0 {
				metrics.IncScreenshotRequest("batch", "partial")
			} else {
				metrics.IncScreenshotRequest("batch", "success")
			}
			metrics.ObserveScreenshotBatchSize(successCount)
			s.batchJobs.complete(jobID, finalResults, successCount, failedCount)
		} else {
			finalResults, successCount, failedCount := mergeBatchURLResults(len(req.URLs)+len(invalidResults), validItems, invalidResults, nil)
			s.batchJobs.complete(jobID, finalResults, successCount, failedCount)
		}
	}()

	// 立即返回 job ID
	type batchStartResponse struct {
		JobID  string `json:"job_id"`
		Total  int    `json:"total"`
		Status string `json:"status"`
	}
	writeJSON(w, http.StatusAccepted, batchStartResponse{
		JobID: jobID, Total: len(validURLs) + len(invalidResults), Status: "running",
	})
}

// handleBatchScreenshotProgress 处理批量截图进度查询 (P1-4)
func (s *Server) handleBatchScreenshotProgress(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_job_id", "job_id query parameter is required", nil)
		return
	}

	if s.batchJobs == nil {
		writeAPIError(w, http.StatusNotFound, "job_not_found", "batch job not found", nil)
		return
	}
	job := s.batchJobs.getSnapshot(jobID)
	if job == nil {
		writeAPIError(w, http.StatusNotFound, "job_not_found", "batch job not found", nil)
		return
	}

	writeJSON(w, http.StatusOK, job)
}

type batchURLItem struct {
	Index int
	URL   string
}

type batchURLResult struct {
	Index  int
	Result screenshot.BatchScreenshotResult
}

func classifyBatchURLs(urls []string) ([]batchURLItem, []batchURLResult) {
	valid := make([]batchURLItem, 0, len(urls))
	invalid := make([]batchURLResult, 0)
	for i, u := range urls {
		result := screenshot.BatchScreenshotResult{
			URL:       u,
			Timestamp: time.Now().Unix(),
		}
		parsed, err := url.Parse(u)
		if err != nil {
			result.Error = "invalid URL"
			invalid = append(invalid, batchURLResult{Index: i, Result: result})
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			result.Error = "only http/https schemes allowed"
			invalid = append(invalid, batchURLResult{Index: i, Result: result})
			continue
		}
		if isPrivateOrInternalIP(parsed.Hostname()) {
			result.Error = "url resolves to private/internal address"
			invalid = append(invalid, batchURLResult{Index: i, Result: result})
			continue
		}
		valid = append(valid, batchURLItem{Index: i, URL: u})
	}
	return valid, invalid
}

func mergeBatchURLResults(total int, validItems []batchURLItem, invalidResults []batchURLResult, captured []screenshot.BatchScreenshotResult) ([]screenshot.BatchScreenshotResult, int, int) {
	merged := make([]screenshot.BatchScreenshotResult, total)
	for _, item := range invalidResults {
		if item.Index >= 0 && item.Index < len(merged) {
			merged[item.Index] = item.Result
		}
	}
	for i, item := range validItems {
		if i < len(captured) && item.Index >= 0 && item.Index < len(merged) {
			merged[item.Index] = captured[i]
		}
	}
	success, failed := 0, 0
	for i := range merged {
		if merged[i].URL == "" {
			continue
		}
		if merged[i].Success {
			success++
		} else {
			failed++
		}
	}
	return merged, success, failed
}

// executeBatchURLScreenshot runs the batch screenshot via router or app service.
func (s *Server) executeBatchURLScreenshot(ctx context.Context, req *struct {
	URLs        []string `json:"urls"`
	BatchID     string   `json:"batch_id"`
	Concurrency int      `json:"concurrency"`
}, onResult func(screenshot.BatchScreenshotResult)) (*service.BatchURLsResponse, error) {
	if s.screenshotRouter != nil {
		return s.executeBatchURLScreenshotViaRouter(ctx, req, onResult)
	}
	return s.executeBatchURLScreenshotViaApp(ctx, req)
}

// executeBatchURLScreenshotViaRouter executes batch screenshot using the router.
func (s *Server) executeBatchURLScreenshotViaRouter(ctx context.Context, req *struct {
	URLs        []string `json:"urls"`
	BatchID     string   `json:"batch_id"`
	Concurrency int      `json:"concurrency"`
}, onResult func(screenshot.BatchScreenshotResult)) (*service.BatchURLsResponse, error) {
	routerResults, err := s.screenshotRouter.CaptureBatchURLsWithProgress(ctx, req.URLs, req.BatchID, req.Concurrency, onResult)
	if err != nil {
		return nil, err
	}
	successCount, failCount := 0, 0
	for _, item := range routerResults {
		if item.Success {
			successCount++
		} else {
			failCount++
		}
	}
	return &service.BatchURLsResponse{
		BatchID: req.BatchID, Total: len(req.URLs),
		Success: successCount, Failed: failCount,
		Results: routerResults, ScreenshotDir: s.screenshotRouter.GetScreenshotDirectory(),
	}, nil
}

// executeBatchURLScreenshotViaApp executes batch screenshot using the app service.
// NOTE: Unlike the router path, the app service does not support per-URL progress
// callbacks (onResult). Progress granularity differs: router path reports each URL
// as it completes; app path only reports final results.
func (s *Server) executeBatchURLScreenshotViaApp(ctx context.Context, req *struct {
	URLs        []string `json:"urls"`
	BatchID     string   `json:"batch_id"`
	Concurrency int      `json:"concurrency"`
}) (*service.BatchURLsResponse, error) {
	results, err := s.screenshotApp.CaptureBatchURLs(ctx, s.screenshotMgr, service.BatchURLsRequest{
		URLs: req.URLs, BatchID: req.BatchID, Concurrency: req.Concurrency,
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// writeBatchScreenshotError writes the appropriate error response for batch screenshot failures.
func writeBatchScreenshotError(w http.ResponseWriter, err error) {
	errText := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errText, "no urls"):
		writeAPIError(w, http.StatusBadRequest, "no_urls_provided", "no URLs provided", nil)
	case strings.Contains(errText, "too many"):
		writeAPIError(w, http.StatusBadRequest, "too_many_urls", "too many URLs", map[string]int{"max": 100})
	default:
		writeAPIError(w, http.StatusInternalServerError, "batch_screenshot_failed", "batch screenshot failed", sanitizeError(err.Error()))
	}
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
		writeAPIError(w, http.StatusInternalServerError, "list_batches_failed", "list screenshot batches failed", sanitizeError(err.Error()))
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
			writeAPIError(w, http.StatusInternalServerError, "list_batch_files_failed", "list batch files failed", sanitizeError(err.Error()))
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
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
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
			writeAPIError(w, http.StatusInternalServerError, "delete_batch_failed", "delete batch failed", sanitizeError(err.Error()))
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
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
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
			writeAPIError(w, http.StatusInternalServerError, "delete_file_failed", "delete file failed", sanitizeError(err.Error()))
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"batch":   batch,
		"file":    fileName,
	})
}

// handleScreenshotRouterStatus returns the current screenshot router status.
func (s *Server) handleScreenshotRouterStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	if s.screenshotRouter == nil {
		mode := "cdp"
		if cfg := s.currentConfig(); cfg != nil {
			mode = strings.ToLower(strings.TrimSpace(cfg.Screenshot.Engine))
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"router_enabled": false,
			"mode":           mode,
		})
		return
	}

	cdpHealthy, extHealthy := s.screenshotRouter.HealthStatus()
	cfg := s.screenshotRouter.Config()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"router_enabled":  true,
		"configured_mode": string(s.screenshotRouter.ConfiguredMode()),
		"current_mode":    string(s.screenshotRouter.ActiveMode()),
		"ready":           s.screenshotRouter.Ready(),
		"cdp_healthy":     cdpHealthy,
		"ext_healthy":     extHealthy,
		"priority":        string(cfg.Priority),
		"fallback":        cfg.Fallback,
	})
}

// handleSetScreenshotMode changes the screenshot execution mode at runtime.
func (s *Server) handleSetScreenshotMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !requireTrustedRequest(w, r, s.allowedOrigins()) {
		return
	}

	var req struct {
		Mode string `json:"mode"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "cdp" && mode != "extension" && mode != "auto" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be one of: cdp, extension, auto"})
		return
	}

	if _, err := s.updateConfig(func(cfg *config.Config) error {
		cfg.Screenshot.Mode = mode
		return nil
	}); err != nil {
		logger.Warnf("failed to persist screenshot mode %q: %v", mode, err)
		writeAPIError(w, http.StatusInternalServerError, "save_failed", "failed to persist screenshot mode", nil)
		return
	}

	// Apply runtime state only after the configuration commit succeeds.
	if s.screenshotRouter != nil {
		s.screenshotRouter.SetMode(screenshot.ScreenshotMode(mode))
	}
	if s.screenshotApp != nil {
		s.screenshotApp.SetMode(mode)
	}

	routerMode := mode
	if s.screenshotRouter != nil {
		routerMode = string(s.screenshotRouter.CurrentMode())
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mode":        mode,
		"router_mode": routerMode,
	})
}
