package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

func (s *Server) tamperAllocatorFactory() service.TamperAllocatorFactory {
	if s.screenshotMgr == nil {
		return nil
	}
	return func(ctx context.Context) (context.Context, context.CancelFunc, error) {
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

	resp, err := s.tamperApp.Check(r.Context(), service.TamperCheckRequest{
		URLs:        req.URLs,
		Concurrency: req.Concurrency,
		Mode:        req.Mode,
	}, s.tamperAllocatorFactory())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no urls") {
			writeAPIError(w, http.StatusBadRequest, "no_urls_provided", "no URLs provided", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "tamper_check_failed", "tamper check failed", err.Error())
		return
	}

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

	resp, err := s.tamperApp.SetBaseline(r.Context(), service.TamperBaselineRequest{
		URLs:        req.URLs,
		Concurrency: req.Concurrency,
	}, s.tamperAllocatorFactory())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no urls") {
			writeAPIError(w, http.StatusBadRequest, "no_urls_provided", "no URLs provided", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "set_baseline_failed", "set baseline failed", err.Error())
		return
	}

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
		writeAPIError(w, http.StatusInternalServerError, "list_baselines_failed", "list baselines failed", err.Error())
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
		writeAPIError(w, http.StatusInternalServerError, "delete_baseline_failed", "delete baseline failed", err.Error())
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

	urlFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("url")))
	typeFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	modeFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	queryFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	storage := tamper.NewHashStorage("./hash_store")
	allRecords, err := storage.ListAllCheckRecords()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "list_history_failed", "list history failed", err.Error())
		return
	}

	type historyRecord struct {
		ID               string   `json:"id"`
		URL              string   `json:"url"`
		CheckType        string   `json:"check_type"`
		DetectionMode    string   `json:"detection_mode,omitempty"`
		Status           string   `json:"status"`
		Tampered         bool     `json:"tampered"`
		TamperedSegments []string `json:"tampered_segments,omitempty"`
		ChangesCount     int      `json:"changes_count"`
		Timestamp        int64    `json:"timestamp"`
		CurrentFullHash  string   `json:"current_full_hash,omitempty"`
		BaselineFullHash string   `json:"baseline_full_hash,omitempty"`
	}

	records := make([]historyRecord, 0)
	urlSet := make(map[string]struct{})

	for _, list := range allRecords {
		for _, rec := range list {
			if rec == nil {
				continue
			}
			recordURL := strings.TrimSpace(rec.URL)
			if recordURL == "" {
				continue
			}

			status := "normal"
			switch {
			case rec.CheckType == "first_check":
				status = "first_check"
			case rec.Tampered:
				status = "tampered"
			case rec.BaselineHash == nil:
				status = "no_baseline"
			default:
				status = "normal"
			}

			urlLower := strings.ToLower(recordURL)
			if urlFilter != "" && urlLower != urlFilter {
				continue
			}
			if typeFilter != "" {
				if strings.ToLower(rec.CheckType) != typeFilter && status != typeFilter {
					continue
				}
			}

			recordMode := strings.ToLower(strings.TrimSpace(rec.DetectionMode))
			if recordMode == "" {
				recordMode = tamper.DetectionModeRelaxed
			}
			if modeFilter != "" && modeFilter != recordMode {
				continue
			}

			if queryFilter != "" {
				if !strings.Contains(urlLower, queryFilter) &&
					!strings.Contains(strings.ToLower(rec.CheckType), queryFilter) &&
					!strings.Contains(status, queryFilter) &&
					!strings.Contains(recordMode, queryFilter) {
					continue
				}
			}

			item := historyRecord{
				ID:               rec.ID,
				URL:              recordURL,
				CheckType:        rec.CheckType,
				DetectionMode:    recordMode,
				Status:           status,
				Tampered:         rec.Tampered,
				TamperedSegments: rec.TamperedSegments,
				ChangesCount:     len(rec.Changes),
				Timestamp:        rec.Timestamp,
			}
			if rec.CurrentHash != nil {
				item.CurrentFullHash = rec.CurrentHash.FullHash
			}
			if rec.BaselineHash != nil {
				item.BaselineFullHash = rec.BaselineHash.FullHash
			}

			records = append(records, item)
			urlSet[recordURL] = struct{}{}
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp > records[j].Timestamp
	})
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}

	urlOptions := make([]string, 0, len(urlSet))
	for u := range urlSet {
		urlOptions = append(urlOptions, u)
	}
	sort.Strings(urlOptions)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   len(records),
		"records": records,
		"urls":    urlOptions,
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

	storage := tamper.NewHashStorage("./hash_store")
	if err := storage.DeleteCheckRecords(urlValue); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "delete_history_failed", "delete history failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"url":     urlValue,
	})
}
