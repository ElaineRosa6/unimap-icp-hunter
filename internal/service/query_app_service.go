package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/unimap/project/internal/adapter"
	"github.com/unimap/project/internal/collection"
	"github.com/unimap/project/internal/core/unimap"
	"github.com/unimap/project/internal/history"
	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/screenshot"
)

// BrowserQueryOutcome 封装浏览器联动查询的结果。
type BrowserQueryOutcome struct {
	Enabled            bool
	OpenedEngines      []string
	CollectedResults   []collection.CollectResult
	Errors             []string
	AutoCaptureEnabled bool
	AutoCaptureQueryID string
	AutoCapturedPaths  map[string]string
	AutoCaptureErrors  []string
}

// BrowserQueryWorkflowOptions configures the synchronous, durable browser
// workflow used by scheduled queries.
type BrowserQueryWorkflowOptions struct {
	Action             string
	QueryID            string
	AutoCaptureEnabled bool
	PreviewURLBuilder  func(string) string
	ScreenshotApp      *ScreenshotAppService
	ScreenshotManager  *screenshot.Manager
	BrowserRouter      BrowserRouter
	RequireComplete    bool
	RequirePersistence bool
}

// QueryAppService 封装查询应用层流程（引擎选择、核心查询、可选浏览器联动）。
type QueryAppService struct {
	unified      *UnifiedService
	orchestrator *adapter.EngineOrchestrator
	historyRepo  *history.Repository
}

const (
	// QueryExecutionTimeout is the server-side guard for one UQL API query.
	QueryExecutionTimeout = 5 * time.Minute

	// BrowserQueryWaitTimeout bounds how long handlers wait for optional browser collection.
	BrowserQueryWaitTimeout = 60 * time.Second

	// BrowserCollectAndCaptureWaitTimeout allows the extension bridge to process
	// several collect+capture tasks even when the browser extension polls serially.
	BrowserCollectAndCaptureWaitTimeout = 150 * time.Second
)

func NewQueryAppService(unified *UnifiedService, orchestrator *adapter.EngineOrchestrator) *QueryAppService {
	return &QueryAppService{unified: unified, orchestrator: orchestrator}
}

// SetHistoryRepository enables server-side persistence for every completed query.
// It is optional so CLI-only and tests can use the service without a database.
func (s *QueryAppService) SetHistoryRepository(repo *history.Repository) {
	s.historyRepo = repo
}

// LoadPushedAssetKeys returns the set of asset fingerprints already pushed for
// a task. Without a history repository (no persistence configured) it returns
// an empty set, so incremental filtering degrades to "everything is new".
func (s *QueryAppService) LoadPushedAssetKeys(taskID string) (map[string]struct{}, error) {
	if s.historyRepo == nil {
		return map[string]struct{}{}, nil
	}
	return s.historyRepo.LoadPushedAssetKeys(taskID)
}

// RecordPushedAssets records asset fingerprints as pushed for a task. Without a
// history repository the call is a no-op.
func (s *QueryAppService) RecordPushedAssets(taskID string, keys []string) error {
	if s.historyRepo == nil {
		return nil
	}
	return s.historyRepo.RecordPushedAssets(taskID, keys)
}

func BrowserQueryWaitTimeoutForAction(action string) time.Duration {
	if strings.EqualFold(strings.TrimSpace(action), "collect_and_capture") {
		return BrowserCollectAndCaptureWaitTimeout
	}
	return BrowserQueryWaitTimeout
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
	startedAt := time.Now()
	resp, err := s.executeQuery(ctx, query, engines, pageSize)
	if persistErr := s.persistQueryHistory(query, engines, pageSize, resp, err, time.Since(startedAt), nil); persistErr != nil {
		logger.Warnf("persist query history: %v", persistErr)
		if resp != nil {
			resp.Persistence = PersistenceStatus{Status: "failed", Warning: "query completed but history persistence failed"}
		}
	} else if resp != nil {
		if s.historyRepo == nil {
			resp.Persistence = PersistenceStatus{Status: "disabled"}
		} else {
			resp.Persistence = PersistenceStatus{Status: "persisted"}
		}
	}
	return resp, err
}

func (s *QueryAppService) executeQuery(ctx context.Context, query string, engines []string, pageSize int) (*QueryResponse, error) {
	if s.unified == nil {
		return nil, fmt.Errorf("query service not initialized")
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, QueryExecutionTimeout)
		defer cancel()
	}
	return s.unified.Query(ctx, QueryRequest{
		Query:       query,
		Engines:     engines,
		PageSize:    pageSize,
		ProcessData: true,
	})
}

// deadlineHiddenContext hides only the advertised deadline. Values, Done, Err,
// and cancellation causes remain those of the parent, without a relay goroutine.
type deadlineHiddenContext struct{ context.Context }

func (deadlineHiddenContext) Deadline() (time.Time, bool) { return time.Time{}, false }

// withoutDeadline lets executeQuery install its own timeout while retaining
// parent cancellation, including cancellation caused by the parent's deadline.
func withoutDeadline(parent context.Context) context.Context {
	if parent == nil {
		return context.Background()
	}
	if _, ok := parent.Deadline(); !ok {
		return parent
	}
	return deadlineHiddenContext{Context: parent}
}

// ExecuteQueryWithBrowserWorkflow runs the API query and Bridge collection in
// parallel, merges both result sets, and persists exactly one history record.
// Scheduled callers can require complete collect+capture and durable storage.
func (s *QueryAppService) ExecuteQueryWithBrowserWorkflow(
	ctx context.Context,
	query string,
	engines []string,
	pageSize int,
	opts BrowserQueryWorkflowOptions,
) (*QueryResponse, BrowserQueryOutcome, error) {
	if strings.TrimSpace(opts.Action) == "" {
		opts.Action = "collect_and_capture"
	}
	startedAt := time.Now()
	browserCh := s.RunBrowserQueryAsync(
		ctx, query, engines, true, opts.Action, opts.QueryID, opts.AutoCaptureEnabled,
		opts.ScreenshotApp, opts.ScreenshotManager, browserPreviewURLBuilder(opts.PreviewURLBuilder),
		opts.BrowserRouter, nil,
	)

	// Install the API query timeout independently, but preserve caller
	// cancellation. Hiding Deadline does not detach the parent's Done signal.
	resp, queryErr := s.executeQuery(withoutDeadline(ctx), query, engines, pageSize)
	var browserOutcome BrowserQueryOutcome
	if browserCh != nil {
		browserOutcome = <-browserCh
	}
	merged := mergeBrowserQueryResponse(resp, browserOutcome)

	workflowErr := validateBrowserQueryWorkflow(engines, opts.Action, browserOutcome, opts.RequireComplete)
	if ctx != nil && ctx.Err() != nil {
		workflowErr = fmt.Errorf("browser query workflow context ended: %w", ctx.Err())
	}
	if queryErr != nil {
		if workflowErr == nil && browserOutcomeHasAssets(browserOutcome) {
			merged.Errors = append(merged.Errors, fmt.Sprintf("API query failed; Bridge results used: %v", queryErr))
		} else if workflowErr == nil {
			workflowErr = queryErr
		}
	}

	if opts.RequirePersistence && s.historyRepo == nil {
		return merged, browserOutcome, fmt.Errorf("query history repository not available")
	}
	details := map[string]interface{}{
		"browser_action":      opts.Action,
		"browser_query_id":    opts.QueryID,
		"browser_screenshots": browserOutcome.AutoCapturedPaths,
	}
	if persistErr := s.persistQueryHistory(query, engines, pageSize, merged, workflowErr, time.Since(startedAt), details); persistErr != nil {
		merged.Persistence = PersistenceStatus{Status: "failed", Warning: "query completed but history persistence failed"}
		return merged, browserOutcome, fmt.Errorf("persist combined query results: %w", persistErr)
	}
	if s.historyRepo == nil {
		merged.Persistence = PersistenceStatus{Status: "disabled"}
	} else {
		merged.Persistence = PersistenceStatus{Status: "persisted"}
	}
	if workflowErr != nil {
		return merged, browserOutcome, workflowErr
	}
	return merged, browserOutcome, nil
}

func browserPreviewURLBuilder(builder func(string) string) func(string) string {
	if builder != nil {
		return builder
	}
	return func(path string) string { return path }
}

// browserOutcomeHasAssets reports whether the browser channel produced usable
// structured assets. A collection envelope without assets must not mask an API
// failure, login wall, selector miss, or empty result page.
func browserOutcomeHasAssets(outcome BrowserQueryOutcome) bool {
	for _, result := range outcome.CollectedResults {
		if len(result.Assets) > 0 {
			return true
		}
	}
	return false
}

func mergeBrowserQueryResponse(resp *QueryResponse, outcome BrowserQueryOutcome) *QueryResponse {
	merged := &QueryResponse{EngineStats: make(map[string]int)}
	if resp != nil {
		merged.Assets = append(merged.Assets, resp.Assets...)
		merged.TotalCount = resp.TotalCount
		merged.Errors = append(merged.Errors, resp.Errors...)
		for engine, count := range resp.EngineStats {
			merged.EngineStats[engine] = count
		}
	}
	for _, collected := range outcome.CollectedResults {
		collection.NormalizeAssets(collected.Engine, collected.Assets)
		merged.Assets = append(merged.Assets, collected.Assets...)
		if collected.Total > 0 {
			merged.TotalCount += collected.Total
		} else {
			merged.TotalCount += len(collected.Assets)
		}
		merged.EngineStats[collected.Engine] += len(collected.Assets)
	}
	merged.Errors = append(merged.Errors, outcome.Errors...)
	merged.Errors = append(merged.Errors, outcome.AutoCaptureErrors...)
	return merged
}

func validateBrowserQueryWorkflow(engines []string, action string, outcome BrowserQueryOutcome, requireComplete bool) error {
	if !requireComplete {
		return nil
	}
	if action != "collect_and_capture" {
		return fmt.Errorf("browser workflow requires action collect_and_capture, got %q", action)
	}
	if len(outcome.Errors) > 0 || len(outcome.AutoCaptureErrors) > 0 {
		return fmt.Errorf("browser workflow failed: %s", strings.Join(append(append([]string{}, outcome.Errors...), outcome.AutoCaptureErrors...), "; "))
	}
	collected := make(map[string]int, len(outcome.CollectedResults))
	for _, result := range outcome.CollectedResults {
		key := strings.ToLower(strings.TrimSpace(result.Engine))
		collected[key] += len(result.Assets)
	}
	for _, engine := range engines {
		key := strings.ToLower(strings.TrimSpace(engine))
		if collected[key] == 0 {
			return fmt.Errorf("browser workflow returned no structured assets for %s", engine)
		}
		if strings.TrimSpace(outcome.AutoCapturedPaths[engine]) == "" && strings.TrimSpace(outcome.AutoCapturedPaths[key]) == "" {
			return fmt.Errorf("browser workflow returned no screenshot for %s", engine)
		}
		path := strings.TrimSpace(outcome.AutoCapturedPaths[engine])
		if path == "" {
			path = strings.TrimSpace(outcome.AutoCapturedPaths[key])
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("browser workflow screenshot unavailable for %s: %w", engine, err)
		}
	}
	return nil
}

func (s *QueryAppService) persistQueryHistory(query string, engines []string, pageSize int, resp *QueryResponse, queryErr error, duration time.Duration, details map[string]interface{}) error {
	if s.historyRepo == nil {
		return nil
	}
	input, err := json.Marshal(map[string]interface{}{"query": query, "engines": engines, "page_size": pageSize})
	if err != nil {
		return fmt.Errorf("marshal query history input: %w", err)
	}
	status := "success"
	total := 0
	summary := map[string]interface{}{}
	if resp != nil {
		total = resp.TotalCount
		summary["engine_stats"] = resp.EngineStats
		summary["errors"] = resp.Errors
	}
	if queryErr != nil {
		status = "error"
		summary["error"] = queryErr.Error()
	} else if resp != nil && len(resp.Errors) > 0 {
		status = "partial"
	}
	for key, value := range details {
		summary[key] = value
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal query history summary: %w", err)
	}
	historyRecord := &history.OperationHistory{
		OperationType: history.OpTypeQuery,
		Input:         string(input),
		Status:        status,
		TotalCount:    total,
		Summary:       string(summaryJSON),
		DurationMS:    duration.Milliseconds(),
	}
	resultCapacity := 0
	if resp != nil {
		resultCapacity = len(resp.Assets)
	}
	results := make([]history.OperationResult, 0, resultCapacity)
	if resp != nil {
		for _, asset := range resp.Assets {
			data, marshalErr := json.Marshal(asset)
			if marshalErr != nil {
				logger.Warnf("marshal query history result: %v", marshalErr)
				continue
			}
			results = append(results, history.OperationResult{Data: string(data)})
		}
	}
	_, err = s.historyRepo.CreateHistoryWithResults(historyRecord, results)
	return err
}

func (s *QueryAppService) translateBrowserQuery(query, engine string) (string, error) {
	if s.orchestrator == nil {
		return query, nil
	}
	var engineAdapter adapter.EngineAdapter
	engineAdapter, _ = s.orchestrator.GetAdapter(engine)
	if engineAdapter == nil {
		engineAdapter = browserOnlyTranslationAdapter(engine)
	}
	if engineAdapter == nil {
		return "", fmt.Errorf("browser query engine %s is unsupported", engine)
	}
	ast, err := unimap.NewUQLParser().Parse(query)
	if err != nil {
		return "", fmt.Errorf("parse browser query for %s: %w", engine, err)
	}
	translated, err := engineAdapter.Translate(ast)
	if err != nil {
		return "", fmt.Errorf("translate browser query for %s: %w", engine, err)
	}
	if strings.TrimSpace(translated) == "" {
		return "", fmt.Errorf("translate browser query for %s returned empty query", engine)
	}
	return translated, nil
}

// browserOnlyTranslationAdapter supplies query translation without requiring
// an API key or a registered API adapter. The returned adapters perform no
// network access here; only their Translate method is used.
func browserOnlyTranslationAdapter(engine string) adapter.EngineAdapter {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "fofa":
		return adapter.NewFofaAdapterWebOnly()
	case "hunter":
		return adapter.NewHunterAdapterWebOnly()
	case "quake":
		return adapter.NewQuakeAdapterWebOnly()
	case "zoomeye":
		return adapter.NewZoomEyeAdapterWebOnly()
	case "shodan":
		return adapter.NewShodanAdapterWebOnly()
	case "censys":
		return adapter.NewCensysAdapterWebOnly()
	case "daydaymap":
		return adapter.NewDayDayMapAdapterWebOnly()
	default:
		return nil
	}
}

// RunBrowserQueryAsync 执行可选浏览器联动（打开结果页、截图、采集结构化结果）。
// progressCallback 在每个引擎阶段推进时被调用（progress 范围 0~100），可为 nil。
func (s *QueryAppService) RunBrowserQueryAsync(
	ctx context.Context,
	query string,
	engines []string,
	enabled bool,
	action string,
	queryID string,
	autoCaptureEnabled bool,
	screenshotApp *ScreenshotAppService,
	screenshotMgr *screenshot.Manager,
	previewURLBuilder func(string) string,
	browserRouter BrowserRouter,
	progress func(done, total int, engine string, err error),
) <-chan BrowserQueryOutcome {
	if !enabled {
		return nil
	}

	// Anti-corruption: old clients send browser_query=true without browser_action;
	// fallback to "collect" semantics (the previous default behavior).
	if action == "" {
		action = "collect"
	}

	// Backward compatibility: map old action names to the canonical ones.
	//   - Old "capture" (was: collect-only) → "collect"
	//   - Old "collect" with screenshot context (was: collect+截图) → "collect_and_capture"
	// Heuristic: autoCaptureEnabled serves as the "screenshot context" signal.
	switch action {
	case "capture":
		logger.CtxInfof(ctx, "legacy browser_action 'capture' mapped to 'collect'")
		action = "collect"
	case "collect":
		if autoCaptureEnabled {
			logger.CtxInfof(ctx, "legacy browser_action 'collect' with screenshot context mapped to 'collect_and_capture'")
			action = "collect_and_capture"
		}
	}

	resultCh := make(chan BrowserQueryOutcome, 1)
	go func() {
		defer close(resultCh)
		outcome := BrowserQueryOutcome{Enabled: true}

		if autoCaptureEnabled && (action == "collect" || action == "collect_and_capture") {
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

		total := len(engines)
		completed := 0
		var mu sync.Mutex // protects outcome fields and completed counter
		var wg sync.WaitGroup
		for _, engine := range engines {
			wg.Add(1)
			go func(engine string) {
				var engineErr error
				defer func() {
					mu.Lock()
					completed++
					ce := completed
					if progress != nil {
						progress(ce, total, engine, engineErr)
					}
					mu.Unlock()
					wg.Done()
				}()

				browserQuery, err := s.translateBrowserQuery(query, engine)
				if err != nil {
					engineErr = err
					mu.Lock()
					outcome.Errors = append(outcome.Errors, err.Error())
					mu.Unlock()
					return
				}

				var combined CombinedBrowserRouter
				combinedAvailable := false
				if action == "collect_and_capture" && captureAvailable && browserRouter != nil {
					combined, combinedAvailable = browserRouter.(CombinedBrowserRouter)
				}

				// Open search engine result page only for "open" action.
				// For "collect" and "collect_and_capture", the collect step already
				// navigates to the page, so opening here would cause duplicate navigation.
				if action == "open" && !combinedAvailable {
					if browserRouter != nil {
						if _, err := browserRouter.OpenSearchEngineResult(ctx, engine, browserQuery); err != nil {
							engineErr = err
							mu.Lock()
							outcome.Errors = append(outcome.Errors, fmt.Sprintf("browser query open failed for %s: %v", engine, err))
							mu.Unlock()
							return
						}
						mu.Lock()
						outcome.OpenedEngines = append(outcome.OpenedEngines, engine)
						mu.Unlock()
					} else if screenshotMgr == nil {
						mu.Lock()
						outcome.Errors = append(outcome.Errors, fmt.Sprintf("browser query open skipped for %s: no browser provider", engine))
						mu.Unlock()
						engineErr = fmt.Errorf("no browser provider")
						return
					} else if _, err := screenshotMgr.OpenSearchEngineResult(ctx, engine, browserQuery); err != nil {
						engineErr = err
						mu.Lock()
						outcome.Errors = append(outcome.Errors, fmt.Sprintf("browser query open failed for %s: %v", engine, err))
						mu.Unlock()
						return
					} else {
						mu.Lock()
						outcome.OpenedEngines = append(outcome.OpenedEngines, engine)
						mu.Unlock()
					}
				}

				// Action-specific follow-up
				switch action {
				case "open":
					// Already opened above — nothing more to do.

				case "collect":
					// Collect structured asset data from DOM (no screenshot).
					if browserRouter != nil {
						collected, err := browserRouter.CollectSearchEngineResult(ctx, engine, browserQuery, queryID)
						if err != nil {
							engineErr = err
							mu.Lock()
							outcome.Errors = append(outcome.Errors, fmt.Sprintf("browser collect failed for %s: %v", engine, err))
							mu.Unlock()
						} else {
							tagBrowserAssets(collected)
							mu.Lock()
							outcome.CollectedResults = append(outcome.CollectedResults, collected...)
							mu.Unlock()
						}
					}

				case "collect_and_capture":
					// Collect structured data + take evidence screenshot.
					// 优先使用合并方法（单次导航），降级为分步调用。
					if combinedAvailable {
						captureQueryID := queryID
						if captureQueryID == "" {
							captureQueryID = fmt.Sprintf("query_%d", time.Now().UnixNano())
						}
						collected, path, err := combined.CollectAndCaptureSearchEngineResult(ctx, engine, browserQuery, captureQueryID)
						if err != nil {
							engineErr = err
							mu.Lock()
							outcome.Errors = append(outcome.Errors, fmt.Sprintf("browser collect+capture failed for %s: %v", engine, err))
							mu.Unlock()
						} else {
							tagBrowserAssets(collected)
							mu.Lock()
							outcome.OpenedEngines = append(outcome.OpenedEngines, engine)
							outcome.CollectedResults = append(outcome.CollectedResults, collected...)
							if path != "" && previewURLBuilder != nil {
								if outcome.AutoCapturedPaths == nil {
									outcome.AutoCapturedPaths = make(map[string]string)
								}
								if previewURL := previewURLBuilder(path); previewURL != "" {
									outcome.AutoCapturedPaths[engine] = previewURL
								}
							}
							mu.Unlock()
						}
					} else {
						// 降级：分步调用
						if browserRouter != nil {
							collected, err := browserRouter.CollectSearchEngineResult(ctx, engine, browserQuery, queryID)
							if err != nil {
								engineErr = err
								mu.Lock()
								outcome.Errors = append(outcome.Errors, fmt.Sprintf("browser collect failed for %s: %v", engine, err))
								mu.Unlock()
							} else {
								tagBrowserAssets(collected)
								mu.Lock()
								outcome.CollectedResults = append(outcome.CollectedResults, collected...)
								mu.Unlock()
							}
						}
						if captureAvailable {
							captureQueryID := queryID
							if captureQueryID == "" {
								captureQueryID = fmt.Sprintf("query_%d", time.Now().UnixNano())
							}
							path, _, _, _, err := screenshotApp.CaptureSearchEngineResult(ctx, screenshotMgr, engine, browserQuery, captureQueryID)
							if err != nil {
								mu.Lock()
								outcome.AutoCaptureErrors = append(outcome.AutoCaptureErrors, fmt.Sprintf("screenshot failed for %s: %v", engine, err))
								mu.Unlock()
							} else if previewURLBuilder != nil {
								mu.Lock()
								if outcome.AutoCapturedPaths == nil {
									outcome.AutoCapturedPaths = make(map[string]string)
								}
								if previewURL := previewURLBuilder(path); previewURL != "" {
									outcome.AutoCapturedPaths[engine] = previewURL
								}
								mu.Unlock()
							}
						}
					}
				}
			}(engine)
		}
		wg.Wait()
		// Validate collection diagnostics after all producers have completed.
		// Keep the original envelope and screenshot for troubleshooting, and do
		// not mistake successful API assets for a successful DOM extraction.
		for _, collected := range outcome.CollectedResults {
			if collected.RowsFound > 0 && len(collected.Assets) == 0 {
				outcome.Errors = append(outcome.Errors, fmt.Sprintf("browser collection for %s reported %d DOM rows but returned no structured assets", collected.Engine, collected.RowsFound))
			}
		}
		resultCh <- outcome
	}()

	return resultCh
}

// tagBrowserAssets marks every asset inside collected results as browser-sourced.
func tagBrowserAssets(collected []collection.CollectResult) {
	for i := range collected {
		for j := range collected[i].Assets {
			a := &collected[i].Assets[j]
			if a.Extra == nil {
				a.Extra = make(map[string]interface{})
			}
			a.Extra["collection_method"] = "browser"
		}
	}
}

// BrowserRouter is the minimal interface needed for browser query operations.
type BrowserRouter interface {
	OpenSearchEngineResult(ctx context.Context, engine, query string) (string, error)
	CollectSearchEngineResult(ctx context.Context, engine, query, queryID string) ([]collection.CollectResult, error)
}

// CombinedBrowserRouter extends BrowserRouter with a combined collect+capture operation.
type CombinedBrowserRouter interface {
	BrowserRouter
	CollectAndCaptureSearchEngineResult(ctx context.Context, engine, query, queryID string) ([]collection.CollectResult, string, error)
}

// CDPStatusInfo is the typed response from Chrome DevTools Protocol /json/version.
type CDPStatusInfo struct {
	Browser              string `json:"Browser"`
	ProtocolVersion      string `json:"Protocol-Version"`
	UserAgent            string `json:"User-Agent"`
	V8Version            string `json:"V8-Version"`
	WebKitVersion        string `json:"WebKit-Version"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// nolint:unused
func checkCDPStatus(ctx context.Context, baseURL string) (bool, *CDPStatusInfo, error) {
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

	var info CDPStatusInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false, nil, err
	}

	return true, &info, nil
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
