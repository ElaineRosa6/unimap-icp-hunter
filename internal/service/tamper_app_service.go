package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/metrics"
	"github.com/unimap-icp-hunter/project/internal/tamper"
)

// TamperAllocatorFactory 用于注入浏览器 allocator，便于复用 screenshot 的 CDP/本地启动策略。
type TamperAllocatorFactory func(ctx context.Context) (context.Context, context.CancelFunc, error)

// TamperAppService 封装篡改检测应用层流程。
type TamperAppService struct {
	baseDir string
}

func NewTamperAppService(baseDir string) *TamperAppService {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "./hash_store"
	}
	return &TamperAppService{baseDir: baseDir}
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
	detector := tamper.NewDetector(tamper.DetectorConfig{BaseDir: s.baseDir})
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
	detector := tamper.NewDetector(tamper.DetectorConfig{BaseDir: s.baseDir})
	return detector.DeleteBaseline(targetURL)
}

func (s *TamperAppService) newDetector(ctx context.Context, mode string, allocatorFactory TamperAllocatorFactory) (*tamper.Detector, context.CancelFunc, error) {
	detector := tamper.NewDetector(tamper.DetectorConfig{BaseDir: s.baseDir, DetectionMode: mode})
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
