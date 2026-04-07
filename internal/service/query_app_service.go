package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/screenshot"
)

// BrowserQueryOutcome 封装浏览器联动查询的结果。
type BrowserQueryOutcome struct {
	Enabled            bool
	OpenedEngines      []string
	Errors             []string
	AutoCaptureEnabled bool
	AutoCaptureQueryID string
	AutoCapturedPaths  map[string]string
	AutoCaptureErrors  []string
}

// QueryAppService 封装查询应用层流程（引擎选择、核心查询、可选浏览器联动）。
type QueryAppService struct {
	unified      *UnifiedService
	orchestrator *adapter.EngineOrchestrator
}

func NewQueryAppService(unified *UnifiedService, orchestrator *adapter.EngineOrchestrator) *QueryAppService {
	return &QueryAppService{unified: unified, orchestrator: orchestrator}
}

// ResolveEngines 解析最终要使用的引擎列表。
func (s *QueryAppService) ResolveEngines(engines []string) []string {
	if len(engines) > 0 {
		return engines
	}
	if s.orchestrator == nil {
		return nil
	}
	defaults := s.orchestrator.ListAdapters()
	if len(defaults) == 0 {
		return nil
	}
	return []string{defaults[0]}
}

// ExecuteQuery 执行统一查询。
func (s *QueryAppService) ExecuteQuery(ctx context.Context, query string, engines []string, pageSize int) (*QueryResponse, error) {
	if s.unified == nil {
		return nil, fmt.Errorf("query service not initialized")
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	return s.unified.Query(ctx, QueryRequest{
		Query:       query,
		Engines:     engines,
		PageSize:    pageSize,
		ProcessData: true,
	})
}

// RunBrowserQueryAsync 执行可选浏览器联动（打开结果页、自动截图）。
func (s *QueryAppService) RunBrowserQueryAsync(
	ctx context.Context,
	query string,
	engines []string,
	enabled bool,
	queryID string,
	autoCaptureEnabled bool,
	screenshotApp *ScreenshotAppService,
	screenshotMgr *screenshot.Manager,
	previewURLBuilder func(string) string,
) <-chan BrowserQueryOutcome {
	if !enabled {
		return nil
	}

	resultCh := make(chan BrowserQueryOutcome, 1)
	go func() {
		defer close(resultCh)
		outcome := BrowserQueryOutcome{Enabled: true}

		if autoCaptureEnabled {
			if strings.TrimSpace(queryID) == "" {
				queryID = fmt.Sprintf("query_%d", time.Now().UnixNano())
			}
			outcome.AutoCaptureEnabled = true
			outcome.AutoCaptureQueryID = queryID
			outcome.AutoCapturedPaths = make(map[string]string)
		}

		captureAvailable := screenshotApp != nil && screenshotApp.IsCaptureAvailable(screenshotMgr)
		if outcome.AutoCaptureEnabled && !captureAvailable {
			outcome.AutoCaptureErrors = append(outcome.AutoCaptureErrors, "auto capture unavailable: screenshot engine not initialized")
		}

		for _, engine := range engines {
			if screenshotMgr == nil {
				outcome.Errors = append(outcome.Errors, fmt.Sprintf("browser query open skipped for %s: screenshot manager not initialized", engine))
			} else if _, err := screenshotMgr.OpenSearchEngineResult(ctx, engine, query); err != nil {
				outcome.Errors = append(outcome.Errors, fmt.Sprintf("browser query open failed for %s: %v", engine, err))
			} else {
				outcome.OpenedEngines = append(outcome.OpenedEngines, engine)
			}

			if outcome.AutoCaptureEnabled && captureAvailable {
				path, _, _, _, err := screenshotApp.CaptureSearchEngineResult(ctx, screenshotMgr, engine, query, outcome.AutoCaptureQueryID)
				if err != nil {
					outcome.AutoCaptureErrors = append(outcome.AutoCaptureErrors, fmt.Sprintf("auto capture failed for %s: %v", engine, err))
					continue
				}
				if previewURLBuilder == nil {
					continue
				}
				previewURL := previewURLBuilder(path)
				if previewURL == "" {
					outcome.AutoCaptureErrors = append(outcome.AutoCaptureErrors, fmt.Sprintf("auto capture preview unavailable for %s", engine))
					continue
				}
				outcome.AutoCapturedPaths[engine] = previewURL
			}
		}

		resultCh <- outcome
	}()

	return resultCh
}

func checkCDPStatus(ctx context.Context, baseURL string) (bool, map[string]interface{}, error) {
	baseURL = normalizeCDPBaseURL(baseURL)
	if baseURL == "" {
		return false, nil, fmt.Errorf("cdp url is empty")
	}

	statusURL := strings.TrimRight(baseURL, "/") + "/json/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return false, nil, err
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false, nil, err
	}

	return true, info, nil
}

func normalizeCDPBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	return strings.TrimRight(raw, "/")
}
