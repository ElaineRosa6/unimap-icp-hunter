package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/alerting"
	"github.com/unimap-icp-hunter/project/internal/metrics"
	"github.com/unimap-icp-hunter/project/internal/tamper"
)

// TamperAllocatorFactory 用于注入浏览器 allocator，便于复用 screenshot 的 CDP/本地启动策略。
type TamperAllocatorFactory func(ctx context.Context) (context.Context, context.CancelFunc, error)

// TamperAppService 封装篡改检测应用层流程。
type TamperAppService struct {
	baseDir      string
	alertManager *alerting.Manager
}

func NewTamperAppService(baseDir string, alertManager *alerting.Manager) *TamperAppService {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "./hash_store"
	}
	return &TamperAppService{
		baseDir:      baseDir,
		alertManager: alertManager,
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

func (s *TamperAppService) Check(ctx context.Context, req TamperCheckRequest, allocatorFactory TamperAllocatorFactory) (*TamperCheckResponse, error) {
	if len(req.URLs) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 5
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != tamper.DetectionModeStrict {
		mode = tamper.DetectionModeRelaxed
	}

	detector, cleanup, err := s.newDetector(ctx, mode, allocatorFactory)
	if err != nil {
		return nil, err
	}
	defer cleanup()

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

func (s *TamperAppService) SetBaseline(ctx context.Context, req TamperBaselineRequest, allocatorFactory TamperAllocatorFactory) (*TamperBaselineResponse, error) {
	if len(req.URLs) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 5
	}

	detector, cleanup, err := s.newDetector(ctx, tamper.DetectionModeRelaxed, allocatorFactory)
	if err != nil {
		return nil, err
	}
	defer cleanup()

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
		AlertManager: s.alertManager,
	})
	return detector.DeleteBaseline(targetURL)
}

// LoadCheckRecords 加载指定URL的检测记录
func (s *TamperAppService) LoadCheckRecords(url string, limit int) ([]*tamper.CheckRecord, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
		AlertManager: s.alertManager,
	})
	return detector.LoadCheckRecords(url, limit)
}

// ListAllCheckRecords 列出所有URL的检测记录
func (s *TamperAppService) ListAllCheckRecords() (map[string][]*tamper.CheckRecord, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
		AlertManager: s.alertManager,
	})
	return detector.ListAllCheckRecords()
}

// GetCheckStats 获取检测统计信息
func (s *TamperAppService) GetCheckStats(url string) (map[string]interface{}, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
		AlertManager: s.alertManager,
	})
	return detector.GetCheckStats(url)
}

// DeleteCheckRecords 删除指定URL的所有检测记录
func (s *TamperAppService) DeleteCheckRecords(url string) error {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:      s.baseDir,
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
	Limit       int
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
	storage := tamper.NewHashStorage(s.baseDir)
	allRecords, err := storage.ListAllCheckRecords()
	if err != nil {
		return nil, fmt.Errorf("failed to list history: %w", err)
	}

	records := make([]HistoryRecord, 0)
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

			// 计算状态
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

			// URL 过滤
			urlLower := strings.ToLower(recordURL)
			if filter.URLFilter != "" && urlLower != strings.ToLower(filter.URLFilter) {
				continue
			}

			// 类型过滤
			if filter.TypeFilter != "" {
				if strings.ToLower(rec.CheckType) != strings.ToLower(filter.TypeFilter) &&
					strings.ToLower(status) != strings.ToLower(filter.TypeFilter) {
					continue
				}
			}

			// 模式过滤
			recordMode := strings.ToLower(strings.TrimSpace(rec.DetectionMode))
			if recordMode == "" {
				recordMode = tamper.DetectionModeRelaxed
			}
			if filter.ModeFilter != "" && strings.ToLower(filter.ModeFilter) != recordMode {
				continue
			}

			// 查询过滤
			if filter.QueryFilter != "" {
				queryLower := strings.ToLower(filter.QueryFilter)
				if !strings.Contains(urlLower, queryLower) &&
					!strings.Contains(strings.ToLower(rec.CheckType), queryLower) &&
					!strings.Contains(status, queryLower) &&
					!strings.Contains(recordMode, queryLower) {
					continue
				}
			}

			item := HistoryRecord{
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
				item.BaselineTimestamp = rec.BaselineHash.Timestamp
			}

			records = append(records, item)
			urlSet[recordURL] = struct{}{}
		}
	}

	// 按时间戳降序排序
	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp > records[j].Timestamp
	})

	// 限制数量
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if len(records) > limit {
		records = records[:limit]
	}

	// URL 选项列表
	urlOptions := make([]string, 0, len(urlSet))
	for u := range urlSet {
		urlOptions = append(urlOptions, u)
	}
	sort.Strings(urlOptions)

	return &HistoryResult{
		Records:    records,
		URLOptions: urlOptions,
		Count:      len(records),
	}, nil
}

func (s *TamperAppService) newDetector(ctx context.Context, mode string, allocatorFactory TamperAllocatorFactory) (*tamper.Detector, context.CancelFunc, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{
		BaseDir:       s.baseDir,
		DetectionMode: mode,
		AlertManager:  s.alertManager,
	})
	cleanup := func() {}

	if allocatorFactory == nil {
		return detector, cleanup, nil
	}

	allocCtx, allocCancel, err := allocatorFactory(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize browser for tamper detection: %w", err)
	}
	detector.SetAllocator(ctx, allocCtx, allocCancel)
	cleanup = allocCancel

	return detector, cleanup, nil
}
