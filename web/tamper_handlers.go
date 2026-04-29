package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/service"
	"github.com/unimap-icp-hunter/project/internal/tamper"
)

func (s *Server) newTamperDetector(ctx context.Context, mode string) (*tamper.Detector, context.CancelFunc, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:       "./hash_store",
		DetectionMode: mode,
	})

	cleanup := func() {}
	if s.screenshotMgr == nil {
		return detector, cleanup, nil
	}

	allocCtx, allocCancel, err := s.screenshotMgr.NewAllocator(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize browser for tamper detection: %w", err)
	}

	detector.SetAllocator(ctx, allocCtx, allocCancel)
	cleanup = allocCancel
	return detector, cleanup, nil
}

func (s *Server) tamperAllocatorFactory(proxy string) service.TamperAllocatorFactory {
	if s.screenshotMgr == nil {
		return nil
	}
	return func(ctx context.Context) (context.Context, context.CancelFunc, error) {
		if strings.TrimSpace(proxy) != "" {
			return s.screenshotMgr.NewAllocatorWithProxy(ctx, proxy)
		}
		return s.screenshotMgr.NewAllocator(ctx)
	}
}

// handleTamperCheck 处理篡改检测请求
func (s *Server) handleTamperCheck(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		URLs        []string `json:"urls"`
		Concurrency int      `json:"concurrency"`
		Mode        string   `json:"mode"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	// Limit concurrency and URL count to prevent resource exhaustion
	const maxTamperConcurrency = 20
	const maxTamperURLs = 500
	if len(req.URLs) > maxTamperURLs {
		writeAPIError(w, http.StatusBadRequest, "too_many_urls", fmt.Sprintf("maximum %d URLs allowed", maxTamperURLs), nil)
		return
	}
	if req.Concurrency <= 0 || req.Concurrency > maxTamperConcurrency {
		req.Concurrency = maxTamperConcurrency
	}

	proxy := s.selectRequestProxy()
	resp, err := s.tamperApp.Check(r.Context(), service.TamperCheckRequest{
		URLs:        req.URLs,
		Concurrency: req.Concurrency,
		Mode:        req.Mode,
	}, s.tamperAllocatorFactory(proxy))
	if err != nil {
		s.reportRequestProxy(proxy, false)
		if strings.Contains(strings.ToLower(err.Error()), "no urls") {
			writeAPIError(w, http.StatusBadRequest, "no_urls_provided", "no URLs provided", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "tamper_check_failed", "tamper check failed", sanitizeError(err.Error()))
		return
	}
	s.reportRequestProxy(proxy, true)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"mode":    resp.Mode,
		"summary": resp.Summary,
		"results": resp.Results,
	})
}

// handleTamperBaseline 处理基线设置请求
func (s *Server) handleTamperBaseline(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		URLs        []string `json:"urls"`
		Concurrency int      `json:"concurrency"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	// Limit concurrency and URL count to prevent resource exhaustion
	const maxTamperConcurrency = 20
	const maxTamperURLs = 500
	if len(req.URLs) > maxTamperURLs {
		writeAPIError(w, http.StatusBadRequest, "too_many_urls", fmt.Sprintf("maximum %d URLs allowed", maxTamperURLs), nil)
		return
	}
	if req.Concurrency <= 0 || req.Concurrency > maxTamperConcurrency {
		req.Concurrency = maxTamperConcurrency
	}

	proxy := s.selectRequestProxy()
	resp, err := s.tamperApp.SetBaseline(r.Context(), service.TamperBaselineRequest{
		URLs:        req.URLs,
		Concurrency: req.Concurrency,
	}, s.tamperAllocatorFactory(proxy))
	if err != nil {
		s.reportRequestProxy(proxy, false)
		if strings.Contains(strings.ToLower(err.Error()), "no urls") {
			writeAPIError(w, http.StatusBadRequest, "no_urls_provided", "no URLs provided", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "set_baseline_failed", "set baseline failed", sanitizeError(err.Error()))
		return
	}
	s.reportRequestProxy(proxy, true)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"summary": resp.Summary,
		"results": resp.Results,
	})
}

// handleTamperBaselineList 处理基线列表请求
func (s *Server) handleTamperBaselineList(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	urls, err := s.tamperApp.ListBaselines()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_baselines_failed", "list baselines failed", sanitizeError(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"urls":    urls,
		"count":   len(urls),
	})
}

// handleTamperBaselineDelete 处理删除基线请求
func (s *Server) handleTamperBaselineDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	urlValue := strings.TrimSpace(r.URL.Query().Get("url"))
	if urlValue == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_url", "URL is required", nil)
		return
	}

	if err := s.tamperApp.DeleteBaseline(urlValue); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "delete_baseline_failed", "delete baseline failed", sanitizeError(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Baseline for %s deleted", urlValue),
		"url":     urlValue,
	})
}

// handleTamperHistory 处理检测历史记录请求
func (s *Server) handleTamperHistory(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	limit := 200
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		if v, err := strconv.Atoi(rawLimit); err == nil && v > 0 {
			if v > 1000 {
				v = 1000
			}
			limit = v
		}
	}

	filter := service.HistoryFilter{
		URLFilter:   strings.TrimSpace(r.URL.Query().Get("url")),
		TypeFilter:  strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type"))),
		ModeFilter:  strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode"))),
		QueryFilter: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))),
		Limit:       limit,
	}

	result, err := s.tamperApp.QueryHistory(filter)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_history_failed", "list history failed", sanitizeError(err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   result.Count,
		"records": result.Records,
		"urls":    result.URLOptions,
	})
}

// handleTamperHistoryDelete 处理删除指定URL的检测历史请求
func (s *Server) handleTamperHistoryDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	urlValue := strings.TrimSpace(r.URL.Query().Get("url"))
	if urlValue == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_url", "URL is required", nil)
		return
	}

	if err := s.tamperApp.DeleteCheckRecords(urlValue); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "delete_history_failed", "delete history failed", sanitizeError(err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"url":     urlValue,
	})
}
