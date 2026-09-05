package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/unimap/project/internal/alerting"
	"github.com/unimap/project/internal/metrics"
	"github.com/unimap/project/internal/tamper"
	"github.com/unimap/project/internal/utils"
)

// TamperPageLoader renders JavaScript pages through the screenshot module's
// guarded browser seam.
type TamperPageLoader = tamper.BrowserPageLoader

// TamperRuntimeConfig contains the detector settings that may change while the
// process is running.
type TamperRuntimeConfig struct {
	InsecureSkipVerify bool
	PortScanEnabled    bool
	PortScanTimeout    time.Duration
}

// TamperRuntimeConfigProvider returns the latest committed detector settings.
type TamperRuntimeConfigProvider func() TamperRuntimeConfig

// TamperAppService 封装篡改检测应用层流程。
type TamperAppService struct {
	storage               *tamper.HashStorage
	baseDir               string
	alertManager          *alerting.Manager
	runtimeConfigProvider TamperRuntimeConfigProvider
}

func NewTamperAppService(baseDir string, alertManager *alerting.Manager, providers ...TamperRuntimeConfigProvider) *TamperAppService {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = utils.HashStoreDir()
	}
	var provider TamperRuntimeConfigProvider
	if len(providers) > 0 {
		provider = providers[0]
	}
	return &TamperAppService{
		baseDir:               baseDir,
		alertManager:          alertManager,
		runtimeConfigProvider: provider,
	}
}

type TamperCheckRequest struct {
	URLs        []string
	Concurrency int
	Mode        string
}

type TamperCheckResponse struct {
	Mode    string
	Summary map[string]int
	Results []tamper.TamperCheckResult
}

type TamperBaselineRequest struct {
	URLs        []string
	Concurrency int
}

type TamperBaselineResponse struct {
	Summary map[string]int
	Results []tamper.PageHashResult
}

func (s *TamperAppService) Check(ctx context.Context, req TamperCheckRequest, pageLoader TamperPageLoader) (*TamperCheckResponse, error) {
	if len(req.URLs) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 5
	}
	// NormalizeDetectionMode preserves all five supported detection modes
	// (relaxed/strict/security/balanced/precise); previously the service layer
	// silently downgraded security/balanced/precise to relaxed, hiding the
	// detector's richer thresholds from both the UI and scheduled tasks.
	mode := tamper.NormalizeDetectionMode(req.Mode)

	detector := s.newDetector(mode, pageLoader)

	results, err := detector.BatchCheckTampering(ctx, req.URLs, req.Concurrency)
	if err != nil {
		return nil, err
	}

	summary := map[string]int{
		"total":       len(results),
		"tampered":    0,
		"safe":        0,
		"noBaseline":  0,
		"unreachable": 0,
		"failed":      0,
	}

	for i := range results {
		result := &results[i]
		status := strings.ToLower(strings.TrimSpace(result.Status))
		if status == "" {
			if result.CurrentHash == nil {
				status = "failed"
			} else if strings.HasPrefix(strings.ToLower(strings.TrimSpace(result.CurrentHash.Status)), "error") {
				status = "unreachable"
			} else if result.BaselineHash == nil {
				status = "no_baseline"
			} else if result.Tampered {
				status = "tampered"
			} else {
				status = "normal"
			}
			result.Status = status
		}

		switch status {
		case "failed":
			summary["failed"]++
			metrics.IncTamperCheck("failed")
		case "unreachable":
			summary["unreachable"]++
			metrics.IncTamperCheck("unreachable")
		case "no_baseline":
			summary["noBaseline"]++
			metrics.IncTamperCheck("no_baseline")
		case "tampered":
			summary["tampered"]++
			metrics.IncTamperCheck("tampered")
		case "normal":
			summary["safe"]++
			metrics.IncTamperCheck("normal")
		default:
			summary["failed"]++
			metrics.IncTamperCheck("failed")
		}
	}

	return &TamperCheckResponse{Mode: mode, Summary: summary, Results: results}, nil
}

func (s *TamperAppService) SetBaseline(ctx context.Context, req TamperBaselineRequest, pageLoader TamperPageLoader) (*TamperBaselineResponse, error) {
	if len(req.URLs) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 5
	}

	detector := s.newDetector(tamper.DetectionModeRelaxed, pageLoader)

	results, err := detector.BatchSetBaseline(ctx, req.URLs, req.Concurrency)
	if err != nil {
		return nil, err
	}

	summary := map[string]int{
		"total":       len(results),
		"saved":       0,
		"unreachable": 0,
		"failed":      0,
	}

	for _, result := range results {
		status := strings.ToLower(strings.TrimSpace(result.Status))
		if status == "" || status == "success" {
			summary["saved"]++
			continue
		}

		if strings.Contains(status, "failed to initialize browser") || strings.Contains(status, "chrome not found") || strings.Contains(status, "executable file not found") {
			summary["failed"]++
			continue
		}

		if strings.Contains(status, "failed to load page") {
			summary["unreachable"]++
			continue
		}

		summary["failed"]++
	}

	return &TamperBaselineResponse{Summary: summary, Results: results}, nil
}

func (s *TamperAppService) ListBaselines() ([]string, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
		Storage:      s.storage,
		AlertManager: s.alertManager,
	})
	urls, err := detector.ListBaselines()
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	return urls, nil
}

func (s *TamperAppService) DeleteBaseline(targetURL string) error {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
		Storage:      s.storage,
		AlertManager: s.alertManager,
	})
	return detector.DeleteBaseline(targetURL)
}

// LoadCheckRecords 加载指定URL的检测记录
func (s *TamperAppService) LoadCheckRecords(url string, limit int) ([]*tamper.CheckRecord, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
		Storage:      s.storage,
		AlertManager: s.alertManager,
	})
	return detector.LoadCheckRecords(url, limit)
}

// ListAllCheckRecords 列出所有URL的检测记录
func (s *TamperAppService) ListAllCheckRecords() (map[string][]*tamper.CheckRecord, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
		Storage:      s.storage,
		AlertManager: s.alertManager,
	})
	return detector.ListAllCheckRecords()
}

// GetCheckStats 获取检测统计信息
func (s *TamperAppService) GetCheckStats(url string) (tamper.CheckStats, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
		Storage:      s.storage,
		AlertManager: s.alertManager,
	})
	return detector.GetCheckStats(url)
}

// DeleteCheckRecords 删除指定URL的所有检测记录
func (s *TamperAppService) DeleteCheckRecords(url string) error {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
		Storage:      s.storage,
		AlertManager: s.alertManager,
	})
	return detector.DeleteCheckRecords(url)
}

// HistoryFilter 历史记录过滤条件
type HistoryFilter struct {
	URLFilter   string
	TypeFilter  string
	ModeFilter  string
	QueryFilter string
	StartTime   int64
	EndTime     int64
	Limit       int
	Offset      int
}

// HistoryRecord 历史记录
type HistoryRecord struct {
	ID                string   `json:"id"`
	URL               string   `json:"url"`
	CheckType         string   `json:"check_type"`
	DetectionMode     string   `json:"detection_mode,omitempty"`
	Status            string   `json:"status"`
	Tampered          bool     `json:"tampered"`
	TamperedSegments  []string `json:"tampered_segments,omitempty"`
	ChangesCount      int      `json:"changes_count"`
	Timestamp         int64    `json:"timestamp"`
	BaselineTimestamp int64    `json:"baseline_timestamp,omitempty"`
	CurrentFullHash   string   `json:"current_full_hash,omitempty"`
	BaselineFullHash  string   `json:"baseline_full_hash,omitempty"`
}

// HistoryResult 历史记录查询结果
type HistoryResult struct {
	Records    []HistoryRecord `json:"records"`
	URLOptions []string        `json:"urls"`
	Count      int             `json:"count"`
}

// QueryHistory 查询检测历史记录（带过滤和排序）
func (s *TamperAppService) QueryHistory(filter HistoryFilter) (*HistoryResult, error) {
	storage := s.storage
	if storage == nil {
		storage = tamper.NewHashStorage(s.baseDir)
	}
	if canPageHistoryInStorage(filter) {
		page, err := storage.ListCheckRecords(tamper.CheckRecordQuery{
			URL:       strings.TrimSpace(filter.URLFilter),
			StartTime: filter.StartTime,
			EndTime:   filter.EndTime,
			Limit:     filter.Limit,
			Offset:    filter.Offset,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list history page: %w", err)
		}
		records := make([]HistoryRecord, 0, len(page.Records))
		for _, rec := range page.Records {
			if rec == nil || strings.TrimSpace(rec.URL) == "" {
				continue
			}
			recordURL := strings.TrimSpace(rec.URL)
			records = append(records, buildHistoryRecord(rec, recordURL, computeTamperStatus(rec), resolveDetectionMode(rec.DetectionMode)))
		}
		return &HistoryResult{Records: records, URLOptions: page.URLs, Count: page.Total}, nil
	}

	allRecords, err := storage.ListAllCheckRecords()
	if err != nil {
		return nil, fmt.Errorf("failed to list history: %w", err)
	}

	records := make([]HistoryRecord, 0)
	urlSet := make(map[string]struct{})

	for _, list := range allRecords {
		for _, rec := range list {
			if rec == nil || strings.TrimSpace(rec.URL) == "" {
				continue
			}
			recordURL := strings.TrimSpace(rec.URL)
			status := computeTamperStatus(rec)
			recordMode := resolveDetectionMode(rec.DetectionMode)

			if !matchesHistoryFilter(filter, recordURL, rec.CheckType, status, recordMode, rec.Timestamp) {
				continue
			}

			item := buildHistoryRecord(rec, recordURL, status, recordMode)
			records = append(records, item)
			urlSet[recordURL] = struct{}{}
		}
	}

	sort.Slice(records, func(i, j int) bool { return records[i].Timestamp > records[j].Timestamp })
	total := len(records)
	records = limitHistoryRecords(records, filter.Limit, filter.Offset)

	urlOptions := make([]string, 0, len(urlSet))
	for u := range urlSet {
		urlOptions = append(urlOptions, u)
	}
	sort.Strings(urlOptions)

	return &HistoryResult{Records: records, URLOptions: urlOptions, Count: total}, nil
}

func canPageHistoryInStorage(filter HistoryFilter) bool {
	return strings.TrimSpace(filter.TypeFilter) == "" &&
		strings.TrimSpace(filter.ModeFilter) == "" &&
		strings.TrimSpace(filter.QueryFilter) == ""
}

// computeTamperStatus 计算检查记录的状态
func computeTamperStatus(rec *tamper.CheckRecord) string {
	switch {
	case rec.CheckType == "first_check":
		return "first_check"
	case rec.Tampered:
		return "tampered"
	case rec.BaselineHash == nil:
		return "no_baseline"
	default:
		return "normal"
	}
}

// resolveDetectionMode 解析检测模式，默认 relaxed
func resolveDetectionMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "" {
		return tamper.DetectionModeRelaxed
	}
	return m
}

// matchesHistoryFilter 检查记录是否匹配过滤条件
func matchesHistoryFilter(filter HistoryFilter, recordURL, checkType, status, mode string, timestamp int64) bool {
	if filter.StartTime > 0 && timestamp < filter.StartTime {
		return false
	}
	if filter.EndTime > 0 && timestamp > filter.EndTime {
		return false
	}
	urlLower := strings.ToLower(recordURL)
	if filter.URLFilter != "" && urlLower != strings.ToLower(filter.URLFilter) {
		return false
	}
	if filter.TypeFilter != "" {
		tf := strings.ToLower(filter.TypeFilter)
		if strings.ToLower(checkType) != tf && strings.ToLower(status) != tf {
			return false
		}
	}
	if filter.ModeFilter != "" && strings.ToLower(filter.ModeFilter) != mode {
		return false
	}
	if filter.QueryFilter != "" {
		ql := strings.ToLower(filter.QueryFilter)
		if !strings.Contains(urlLower, ql) &&
			!strings.Contains(strings.ToLower(checkType), ql) &&
			!strings.Contains(status, ql) &&
			!strings.Contains(mode, ql) {
			return false
		}
	}
	return true
}

// buildHistoryRecord 从 CheckRecord 构建 HistoryRecord
func buildHistoryRecord(rec *tamper.CheckRecord, recordURL, status, mode string) HistoryRecord {
	item := HistoryRecord{
		ID: rec.ID, URL: recordURL, CheckType: rec.CheckType, DetectionMode: mode,
		Status: status, Tampered: rec.Tampered, TamperedSegments: rec.TamperedSegments,
		ChangesCount: len(rec.Changes), Timestamp: rec.Timestamp,
	}
	if rec.CurrentHash != nil {
		item.CurrentFullHash = rec.CurrentHash.FullHash
	}
	if rec.BaselineHash != nil {
		item.BaselineFullHash = rec.BaselineHash.FullHash
		item.BaselineTimestamp = rec.BaselineHash.Timestamp
	}
	return item
}

// limitHistoryRecords 限制历史记录数量
func limitHistoryRecords(records []HistoryRecord, limit, offset int) []HistoryRecord {
	if limit <= 0 {
		limit = 200
	}
	if limit > 10000 {
		limit = 10000
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(records) {
		return []HistoryRecord{}
	}
	end := offset + limit
	if end > len(records) {
		end = len(records)
	}
	return records[offset:end]
}

func (s *TamperAppService) newDetector(mode string, pageLoader TamperPageLoader) *tamper.Detector {
	detector := tamper.NewDetector(s.detectorConfig(mode))
	if pageLoader != nil {
		detector.SetBrowserPageLoader(pageLoader)
	}
	return detector
}

func (s *TamperAppService) detectorConfig(mode string) tamper.DetectorConfig {
	cfg := tamper.DetectorConfig{
		BaseDir:       s.baseDir,
		Storage:       s.storage,
		DetectionMode: mode,
		AlertManager:  s.alertManager,
	}
	if s.runtimeConfigProvider != nil {
		runtimeCfg := s.runtimeConfigProvider()
		cfg.InsecureSkipVerify = runtimeCfg.InsecureSkipVerify
		cfg.PortScanEnabled = runtimeCfg.PortScanEnabled
		cfg.PortScanTimeout = runtimeCfg.PortScanTimeout
	}
	return cfg
}

// NewPersistentTamperAppService opens a service-owned history database once.
// The caller must Close it after requests and scheduled work have stopped.
func NewPersistentTamperAppService(baseDir string, alerts *alerting.Manager, providers ...TamperRuntimeConfigProvider) (*TamperAppService, error) {
	service := NewTamperAppService(baseDir, alerts, providers...)
	storage, err := tamper.NewPersistentHashStorage(service.baseDir)
	if err != nil {
		return nil, err
	}
	service.storage = storage
	return service, nil
}

func (s *TamperAppService) Close() error {
	if s.storage == nil {
		return nil
	}
	return s.storage.Close()
}

// HistoryDatabase returns a borrowed service-owned connection and its identity.
func (s *TamperAppService) HistoryDatabase() (*sql.DB, os.FileInfo, error) {
	if s.storage == nil {
		return nil, nil, fmt.Errorf("tamper history is not service-owned")
	}
	return s.storage.HistoryDatabase()
}
