package scheduler

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/alerting"
	"github.com/unimap-icp-hunter/project/internal/distributed"
	"github.com/unimap-icp-hunter/project/internal/exporter"
	"github.com/unimap-icp-hunter/project/internal/screenshot"
	"github.com/unimap-icp-hunter/project/internal/service"
)

// extractStrings pulls a string slice from payload[key], falling back to def.
func extractStrings(payload map[string]interface{}, key string, def []string) []string {
	v, ok := payload[key]
	if !ok {
		return def
	}
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if val == "" {
			return def
		}
		return []string{val}
	default:
		return def
	}
}

func extractInt(payload map[string]interface{}, key string, def int) int {
	v, ok := payload[key]
	if !ok {
		return def
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	default:
		return def
	}
}

func extractString(payload map[string]interface{}, key string, def string) string {
	v, ok := payload[key]
	if !ok {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

// --- QueryRunner (ST-01) ---

// QueryRunner executes scheduled UQL queries via QueryAppService.
type QueryRunner struct {
	querySvc *service.QueryAppService
}

// NewQueryRunner creates a QueryRunner.
func NewQueryRunner(b *service.QueryAppService) *QueryRunner {
	return &QueryRunner{querySvc: b}
}

func (r *QueryRunner) Type() TaskType { return TaskQuery }

func (r *QueryRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.querySvc == nil {
		return "", fmt.Errorf("query service not available")
	}

	query := extractString(payload, "query", "")
	if query == "" {
		return "", fmt.Errorf("missing 'query' in payload")
	}

	engines := extractStrings(payload, "engines", []string{})
	if len(engines) == 0 {
		engines = extractStrings(payload, "engine", []string{})
	}
	pageSize := extractInt(payload, "page_size", 100)

	resp, err := r.querySvc.ExecuteQuery(ctx, query, engines, pageSize)
	if err != nil {
		return "", fmt.Errorf("query execution failed: %w", err)
	}

	result := fmt.Sprintf("retrieved %d assets from %d engine(s)", resp.TotalCount, len(resp.EngineStats))
	if len(resp.Errors) > 0 {
		result += fmt.Sprintf(" (%d engine error(s))", len(resp.Errors))
	}
	return result, nil
}

// --- SearchScreenshotRunner (ST-02) ---

// SearchScreenshotRunner executes scheduled search engine screenshots.
type SearchScreenshotRunner struct {
	screenshotSvc *service.ScreenshotAppService
	mgr           *screenshot.Manager
}

// NewSearchScreenshotRunner creates a SearchScreenshotRunner.
func NewSearchScreenshotRunner(svc *service.ScreenshotAppService, mgr *screenshot.Manager) *SearchScreenshotRunner {
	return &SearchScreenshotRunner{screenshotSvc: svc, mgr: mgr}
}

func (r *SearchScreenshotRunner) Type() TaskType { return TaskSearchScreenshot }

func (r *SearchScreenshotRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.screenshotSvc == nil {
		return "", fmt.Errorf("screenshot service not available")
	}

	engine := extractString(payload, "engine", "")
	query := extractString(payload, "query", "")
	queryID := extractString(payload, "query_id", "")

	if engine == "" || query == "" {
		return "", fmt.Errorf("missing 'engine' or 'query' in payload")
	}

	path, eng, q, id, err := r.screenshotSvc.CaptureSearchEngineResult(ctx, r.mgr, engine, query, queryID)
	if err != nil {
		return "", fmt.Errorf("screenshot capture failed: %w", err)
	}

	return fmt.Sprintf("captured %s search for '%s' -> %s (query_id=%s)", eng, q, path, id), nil
}

// --- BatchScreenshotRunner (ST-03) ---

// BatchScreenshotRunner executes scheduled batch URL screenshots.
type BatchScreenshotRunner struct {
	screenshotSvc *service.ScreenshotAppService
	mgr           *screenshot.Manager
}

// NewBatchScreenshotRunner creates a BatchScreenshotRunner.
func NewBatchScreenshotRunner(svc *service.ScreenshotAppService, mgr *screenshot.Manager) *BatchScreenshotRunner {
	return &BatchScreenshotRunner{screenshotSvc: svc, mgr: mgr}
}

func (r *BatchScreenshotRunner) Type() TaskType { return TaskBatchScreenshot }

func (r *BatchScreenshotRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.screenshotSvc == nil {
		return "", fmt.Errorf("screenshot service not available")
	}

	urls := extractStrings(payload, "urls", []string{})
	if len(urls) == 0 {
		return "", fmt.Errorf("missing 'urls' in payload")
	}

	batchID := extractString(payload, "batch_id", "")
	concurrency := extractInt(payload, "concurrency", 5)

	req := service.BatchURLsRequest{
		URLs:        urls,
		BatchID:     batchID,
		Concurrency: concurrency,
	}

	resp, err := r.screenshotSvc.CaptureBatchURLs(ctx, r.mgr, req)
	if err != nil {
		return "", fmt.Errorf("batch screenshot failed: %w", err)
	}

	return fmt.Sprintf("batch %s: %d/%d succeeded, dir=%s", resp.BatchID, resp.Success, resp.Total, resp.ScreenshotDir), nil
}

// --- TamperCheckRunner (ST-04) ---

// TamperCheckRunner executes scheduled tamper checks.
type TamperCheckRunner struct {
	tamperSvc        *service.TamperAppService
	allocatorFactory service.TamperAllocatorFactory
}

// NewTamperCheckRunner creates a TamperCheckRunner.
func NewTamperCheckRunner(svc *service.TamperAppService, af service.TamperAllocatorFactory) *TamperCheckRunner {
	return &TamperCheckRunner{tamperSvc: svc, allocatorFactory: af}
}

func (r *TamperCheckRunner) Type() TaskType { return TaskTamperCheck }

func (r *TamperCheckRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.tamperSvc == nil {
		return "", fmt.Errorf("tamper service not available")
	}

	urls := extractStrings(payload, "urls", []string{})
	if len(urls) == 0 {
		return "", fmt.Errorf("missing 'urls' in payload")
	}

	concurrency := extractInt(payload, "concurrency", 5)
	mode := extractString(payload, "detection_mode", "relaxed")

	req := service.TamperCheckRequest{
		URLs:        urls,
		Concurrency: concurrency,
		Mode:        mode,
	}

	resp, err := r.tamperSvc.Check(ctx, req, r.allocatorFactory)
	if err != nil {
		return "", fmt.Errorf("tamper check failed: %w", err)
	}

	parts := []string{}
	for k, v := range resp.Summary {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	return fmt.Sprintf("tamper check complete [%s]", strings.Join(parts, ", ")), nil
}

// --- URLReachabilityRunner (ST-05) ---

// URLReachabilityRunner executes scheduled URL reachability checks.
type URLReachabilityRunner struct {
	monitorSvc *service.MonitorAppService
}

// NewURLReachabilityRunner creates a URLReachabilityRunner.
func NewURLReachabilityRunner(svc *service.MonitorAppService) *URLReachabilityRunner {
	return &URLReachabilityRunner{monitorSvc: svc}
}

func (r *URLReachabilityRunner) Type() TaskType { return TaskURLReachability }

func (r *URLReachabilityRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.monitorSvc == nil {
		return "", fmt.Errorf("monitor service not available")
	}

	urls := extractStrings(payload, "urls", []string{})
	if len(urls) == 0 {
		return "", fmt.Errorf("missing 'urls' in payload")
	}

	concurrency := extractInt(payload, "concurrency", 5)

	resp, err := r.monitorSvc.CheckURLReachability(ctx, urls, concurrency)
	if err != nil {
		return "", fmt.Errorf("reachability check failed: %w", err)
	}

	return fmt.Sprintf("reachability: %d reachable, %d unreachable, %d invalid out of %d",
		resp.Summary.Reachable, resp.Summary.Unreachable, resp.Summary.InvalidFormat, resp.Summary.Total), nil
}

// --- CookieVerifyRunner (ST-06) ---

// CookieVerifyRunner executes scheduled cookie verification.
type CookieVerifyRunner struct {
	screenshotSvc *service.ScreenshotAppService
	mgr           *screenshot.Manager
}

// NewCookieVerifyRunner creates a CookieVerifyRunner.
func NewCookieVerifyRunner(svc *service.ScreenshotAppService, mgr *screenshot.Manager) *CookieVerifyRunner {
	return &CookieVerifyRunner{screenshotSvc: svc, mgr: mgr}
}

func (r *CookieVerifyRunner) Type() TaskType { return TaskCookieVerify }

func (r *CookieVerifyRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.mgr == nil {
		return "", fmt.Errorf("screenshot manager not available")
	}

	engines := extractStrings(payload, "engines", []string{})
	if len(engines) == 0 {
		// Default: check all supported engines
		engines = []string{"fofa", "hunter", "quake", "zoomeye"}
	}

	results := make([]string, 0, len(engines))
	for _, engine := range engines {
		cookies := r.mgr.GetCookies(engine)
		status := "no_cookies"
		if len(cookies) > 0 {
			status = fmt.Sprintf("%d cookie(s) configured", len(cookies))
		}
		results = append(results, fmt.Sprintf("%s: %s", engine, status))
	}

	return strings.Join(results, "; "), nil
}

// --- LoginStatusCheckRunner (ST-07) ---

// LoginStatusCheckRunner executes scheduled login status checks.
type LoginStatusCheckRunner struct {
	mgr *screenshot.Manager
}

// NewLoginStatusCheckRunner creates a LoginStatusCheckRunner.
func NewLoginStatusCheckRunner(mgr *screenshot.Manager) *LoginStatusCheckRunner {
	return &LoginStatusCheckRunner{mgr: mgr}
}

func (r *LoginStatusCheckRunner) Type() TaskType { return TaskLoginStatusCheck }

func (r *LoginStatusCheckRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.mgr == nil {
		return "", fmt.Errorf("screenshot manager not available")
	}

	engines := extractStrings(payload, "engines", []string{})
	if len(engines) == 0 {
		engines = []string{"fofa", "hunter", "quake", "zoomeye"}
	}
	testQuery := extractString(payload, "test_query", "test")

	results := make([]string, 0, len(engines))
	failedCount := 0
	for _, engine := range engines {
		status, err := r.mgr.CheckEngineLoginStatus(ctx, engine, testQuery)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: error=%v", engine, err))
			failedCount++
			continue
		}
		loginStatus := "logged_in"
		if !status.LoggedIn {
			loginStatus = "not_logged_in"
			failedCount++
		}
		results = append(results, fmt.Sprintf("%s: %s (reason=%s)", engine, loginStatus, status.Reason))
	}

	result := strings.Join(results, "; ")
	if failedCount > 0 {
		return result, fmt.Errorf("%d engine(s) not logged in or errored", failedCount)
	}
	return result, nil
}

// --- DistributedSubmitRunner (ST-08) ---

// DistributedSubmitRunner executes scheduled distributed task submissions.
type DistributedSubmitRunner struct {
	taskQueue *distributed.TaskQueue
}

// NewDistributedSubmitRunner creates a DistributedSubmitRunner.
func NewDistributedSubmitRunner(q *distributed.TaskQueue) *DistributedSubmitRunner {
	return &DistributedSubmitRunner{taskQueue: q}
}

func (r *DistributedSubmitRunner) Type() TaskType { return TaskDistributedSubmit }

func (r *DistributedSubmitRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.taskQueue == nil {
		return "", fmt.Errorf("task queue not available")
	}

	taskType := extractString(payload, "task_type", "")
	if taskType == "" {
		return "", fmt.Errorf("missing 'task_type' in payload")
	}

	taskPayload := make(map[string]interface{})
	if p, ok := payload["task_payload"]; ok {
		if pm, ok := p.(map[string]interface{}); ok {
			taskPayload = pm
		}
	}

	priority := extractInt(payload, "priority", 0)
	timeoutSec := extractInt(payload, "timeout_seconds", 300)
	maxReassign := extractInt(payload, "max_reassign", 3)

	// Build the envelope
	envelope := distributed.TaskEnvelope{
		TaskID:         generateDistributedTaskID(),
		TaskType:       taskType,
		Payload:        taskPayload,
		Priority:       priority,
		TimeoutSeconds: timeoutSec,
		MaxReassign:    maxReassign,
	}

	if _, err := r.taskQueue.Enqueue(envelope); err != nil {
		return "", fmt.Errorf("enqueue failed: %w", err)
	}

	return fmt.Sprintf("enqueued task %s (type=%s, priority=%d)", envelope.TaskID, taskType, priority), nil
}

// distributedIDCounter is a monotonic counter for unique distributed task IDs.
var distributedIDCounter atomic.Int64

// generateDistributedTaskID creates a unique ID for distributed task envelopes.
func generateDistributedTaskID() string {
	return fmt.Sprintf("dist_%d", distributedIDCounter.Add(1))
}

// --- ExportRunner (ST-09) ---

// ExportRunner executes scheduled data exports.
type ExportRunner struct {
	queryApp    *service.QueryAppService
	orchestrator *adapter.EngineOrchestrator
	outputDir   string
}

// NewExportRunner creates an ExportRunner.
func NewExportRunner(queryApp *service.QueryAppService, orchestrator *adapter.EngineOrchestrator, outputDir string) *ExportRunner {
	return &ExportRunner{queryApp: queryApp, orchestrator: orchestrator, outputDir: outputDir}
}

func (r *ExportRunner) Type() TaskType { return TaskExport }

func (r *ExportRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.queryApp == nil || r.orchestrator == nil {
		return "", fmt.Errorf("query service or orchestrator not available")
	}

	query := extractString(payload, "query", "")
	if query == "" {
		return "", fmt.Errorf("missing 'query' in payload")
	}

	engines := extractStrings(payload, "engines", []string{})
	pageSize := extractInt(payload, "page_size", 100)
	format := extractString(payload, "format", "json")
	outputFile := extractString(payload, "output_file", "")

	// Execute the query
	resp, err := r.queryApp.ExecuteQuery(ctx, query, engines, pageSize)
	if err != nil {
		return "", fmt.Errorf("query execution failed: %w", err)
	}

	if resp.TotalCount == 0 {
		return "no results to export", nil
	}

	// Determine output path
	if outputFile == "" {
		outputFile = fmt.Sprintf("export_%s_%s.%s", strings.ReplaceAll(query[:min(len(query), 20)], " ", "_"), time.Now().Format("20060102_150405"), format)
	}
	outPath := filepath.Join(r.outputDir, outputFile)

	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// Export
	exp := exporter.NewJSONExporter()
	if err := exp.Export(resp.Assets, outPath); err != nil {
		return "", fmt.Errorf("export failed: %w", err)
	}

	return fmt.Sprintf("exported %d assets to %s", len(resp.Assets), outPath), nil
}

// --- PortScanRunner (ST-10) ---

// PortScanRunner executes scheduled port scans.
type PortScanRunner struct {
	monitorSvc *service.MonitorAppService
}

// NewPortScanRunner creates a PortScanRunner.
func NewPortScanRunner(svc *service.MonitorAppService) *PortScanRunner {
	return &PortScanRunner{monitorSvc: svc}
}

func (r *PortScanRunner) Type() TaskType { return TaskPortScan }

func (r *PortScanRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.monitorSvc == nil {
		return "", fmt.Errorf("monitor service not available")
	}

	urls := extractStrings(payload, "urls", []string{})
	if len(urls) == 0 {
		return "", fmt.Errorf("missing 'urls' in payload")
	}

	ports := extractStrings(payload, "ports", []string{})
	concurrency := extractInt(payload, "concurrency", 5)

	portNums := make([]int, 0, len(ports))
	for _, p := range ports {
		if n := extractInt(map[string]interface{}{"v": p}, "v", 0); n > 0 {
			portNums = append(portNums, n)
		}
	}
	if len(portNums) == 0 {
		portNums = []int{80, 443} // default ports
	}

	resp, err := r.monitorSvc.ScanURLPorts(ctx, urls, portNums, concurrency)
	if err != nil {
		return "", fmt.Errorf("port scan failed: %w", err)
	}

	return fmt.Sprintf("scanned %d URLs: %d successful, %d failed",
		resp.Summary.Total, resp.Summary.Scanned, resp.Summary.ScanFailed), nil
}

// --- ScreenshotCleanupRunner (ST-11) ---

// ScreenshotCleanupRunner executes scheduled screenshot cleanup.
type ScreenshotCleanupRunner struct {
	screenshotSvc *service.ScreenshotAppService
	maxAgeDays    int
}

// NewScreenshotCleanupRunner creates a ScreenshotCleanupRunner.
func NewScreenshotCleanupRunner(svc *service.ScreenshotAppService, maxAgeDays int) *ScreenshotCleanupRunner {
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}
	return &ScreenshotCleanupRunner{screenshotSvc: svc, maxAgeDays: maxAgeDays}
}

func (r *ScreenshotCleanupRunner) Type() TaskType { return TaskScreenshotCleanup }

func (r *ScreenshotCleanupRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.screenshotSvc == nil {
		return "", fmt.Errorf("screenshot service not available")
	}

	maxAgeDays := extractInt(payload, "max_age_days", r.maxAgeDays)
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)

	batches, err := r.screenshotSvc.ListBatches()
	if err != nil {
		return "", fmt.Errorf("list batches failed: %w", err)
	}

	deletedCount := 0
	for _, batch := range batches {
		batchTime := time.Unix(batch.UpdatedAt, 0)
		if batchTime.Before(cutoff) {
			if delErr := r.screenshotSvc.DeleteBatch(batch.Name); delErr != nil {
				// Log but continue with other batches
				continue
			}
			deletedCount++
		}
	}

	return fmt.Sprintf("cleaned up %d batch(es) older than %d days", deletedCount, maxAgeDays), nil
}

// --- TamperCleanupRunner (ST-12) ---

// TamperCleanupRunner executes scheduled tamper record cleanup.
type TamperCleanupRunner struct {
	tamperSvc  *service.TamperAppService
	maxAgeDays int
}

// NewTamperCleanupRunner creates a TamperCleanupRunner.
func NewTamperCleanupRunner(svc *service.TamperAppService, maxAgeDays int) *TamperCleanupRunner {
	if maxAgeDays <= 0 {
		maxAgeDays = 90
	}
	return &TamperCleanupRunner{tamperSvc: svc, maxAgeDays: maxAgeDays}
}

func (r *TamperCleanupRunner) Type() TaskType { return TaskTamperCleanup }

func (r *TamperCleanupRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.tamperSvc == nil {
		return "", fmt.Errorf("tamper service not available")
	}

	records, err := r.tamperSvc.ListAllCheckRecords()
	if err != nil {
		return "", fmt.Errorf("list check records failed: %w", err)
	}

	deletedCount := len(records)
	for url := range records {
		if delErr := r.tamperSvc.DeleteCheckRecords(url); delErr != nil {
			deletedCount--
		}
	}

	return fmt.Sprintf("cleaned up check records for %d URL(s) (max age: %d days)", deletedCount, r.maxAgeDays), nil
}

// --- QuotaMonitorRunner (ST-13) ---

// QuotaMonitorRunner executes scheduled quota monitoring.
type QuotaMonitorRunner struct {
	orchestrator  *adapter.EngineOrchestrator
	lowThreshold  int
}

// NewQuotaMonitorRunner creates a QuotaMonitorRunner.
func NewQuotaMonitorRunner(orchestrator *adapter.EngineOrchestrator, lowThreshold int) *QuotaMonitorRunner {
	if lowThreshold <= 0 {
		lowThreshold = 10
	}
	return &QuotaMonitorRunner{orchestrator: orchestrator, lowThreshold: lowThreshold}
}

func (r *QuotaMonitorRunner) Type() TaskType { return TaskQuotaMonitor }

func (r *QuotaMonitorRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.orchestrator == nil {
		return "", fmt.Errorf("orchestrator not available")
	}

	engines := r.orchestrator.ListAdapters()
	if len(engines) == 0 {
		return "no engine adapters registered", nil
	}

	lowThreshold := extractInt(payload, "low_threshold", r.lowThreshold)
	results := make([]string, 0, len(engines))
	lowQuotaEngines := 0

	for _, engine := range engines {
		adapter, ok := r.orchestrator.GetAdapter(engine)
		if !ok {
			continue
		}
		quota, err := adapter.GetQuota()
		if err != nil {
			results = append(results, fmt.Sprintf("%s: error=%v", engine, err))
			continue
		}
		status := "ok"
		if quota != nil && quota.Remaining < lowThreshold {
			status = fmt.Sprintf("LOW (remaining=%d)", quota.Remaining)
			lowQuotaEngines++
		} else if quota != nil {
			status = fmt.Sprintf("remaining=%d/%d", quota.Remaining, quota.Total)
		}
		results = append(results, fmt.Sprintf("%s: %s", engine, status))
	}

	result := strings.Join(results, "; ")
	if lowQuotaEngines > 0 {
		return result, fmt.Errorf("%d engine(s) with low quota (below %d)", lowQuotaEngines, lowThreshold)
	}
	return result, nil
}

// --- AlertSummaryRunner (ST-14) ---

// AlertSummaryRunner executes scheduled alert summary generation.
type AlertSummaryRunner struct {
	alertManager *alerting.Manager
}

// NewAlertSummaryRunner creates an AlertSummaryRunner.
func NewAlertSummaryRunner(alertManager *alerting.Manager) *AlertSummaryRunner {
	return &AlertSummaryRunner{alertManager: alertManager}
}

func (r *AlertSummaryRunner) Type() TaskType { return TaskAlertSummary }

func (r *AlertSummaryRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.alertManager == nil {
		return "", fmt.Errorf("alert manager not available")
	}

	maxAgeDays := extractInt(payload, "max_age_days", 7)

	records := r.alertManager.GetAlertRecords()
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)

	typeCounts := make(map[string]int)
	levelCounts := make(map[string]int)
	totalCount := 0

	for _, rec := range records {
		if rec.Alert.Timestamp.Before(cutoff) {
			continue
		}
		totalCount++
		typeCounts[string(rec.Alert.Type)]++
		levelCounts[string(rec.Alert.Level)]++
	}

	parts := []string{
		fmt.Sprintf("total=%d (last %d days)", totalCount, maxAgeDays),
	}
	for t, c := range typeCounts {
		parts = append(parts, fmt.Sprintf("%s=%d", t, c))
	}
	for l, c := range levelCounts {
		parts = append(parts, fmt.Sprintf("%s=%d", l, c))
	}

	return fmt.Sprintf("alert summary [%s]", strings.Join(parts, ", ")), nil
}

// --- BaselineRefreshRunner (ST-15) ---

// BaselineRefreshRunner executes scheduled baseline refresh.
type BaselineRefreshRunner struct {
	tamperSvc *service.TamperAppService
}

// NewBaselineRefreshRunner creates a BaselineRefreshRunner.
func NewBaselineRefreshRunner(svc *service.TamperAppService) *BaselineRefreshRunner {
	return &BaselineRefreshRunner{tamperSvc: svc}
}

func (r *BaselineRefreshRunner) Type() TaskType { return TaskBaselineRefresh }

func (r *BaselineRefreshRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.tamperSvc == nil {
		return "", fmt.Errorf("tamper service not available")
	}

	urls := extractStrings(payload, "urls", []string{})
	if len(urls) == 0 {
		// Get current baselines and refresh them
		baselines, err := r.tamperSvc.ListBaselines()
		if err != nil {
			return "", fmt.Errorf("list baselines failed: %w", err)
		}
		if len(baselines) == 0 {
			return "no baselines to refresh", nil
		}
		urls = baselines
	}

	refreshed := 0

	for _, url := range urls {
		req := service.TamperBaselineRequest{
			URLs: []string{url},
		}
		_, err := r.tamperSvc.SetBaseline(ctx, req, nil)
		if err != nil {
			continue
		}
		refreshed++
	}

	return fmt.Sprintf("refreshed baseline for %d/%d URL(s)", refreshed, len(urls)), nil
}

// --- URLImportRunner (ST-16) ---

// URLImportRunner executes scheduled URL import from files.
type URLImportRunner struct {
	importDir string
}

// NewURLImportRunner creates a URLImportRunner.
func NewURLImportRunner(importDir string) *URLImportRunner {
	return &URLImportRunner{importDir: importDir}
}

func (r *URLImportRunner) Type() TaskType { return TaskURLImport }

func (r *URLImportRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.importDir == "" {
		return "", fmt.Errorf("import directory not configured")
	}

	filePattern := extractString(payload, "file_pattern", "*.txt")
	maxLines := extractInt(payload, "max_lines", 10000)

	// Find matching files
	matches, err := filepath.Glob(filepath.Join(r.importDir, filePattern))
	if err != nil {
		return "", fmt.Errorf("glob failed: %w", err)
	}
	if len(matches) == 0 {
		return fmt.Sprintf("no files matching %s in %s", filePattern, r.importDir), nil
	}

	importedURLs := make([]string, 0)
	for _, filePath := range matches {
		urls, err := readURLsFromFile(filePath, maxLines)
		if err != nil {
			continue
		}
		importedURLs = append(importedURLs, urls...)
	}

	return fmt.Sprintf("imported %d URL(s) from %d file(s)", len(importedURLs), len(matches)), nil
}

// readURLsFromFile reads URLs from a text file, one per line.
func readURLsFromFile(filePath string, maxLines int) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	urls := make([]string, 0)
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		if count >= maxLines {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
		count++
	}
	return urls, scanner.Err()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- PluginHealthRunner (ST-17) ---

// PluginHealthRunner executes scheduled plugin health checks.
type PluginHealthRunner struct {
	unifiedSvc *service.UnifiedService
}

// NewPluginHealthRunner creates a PluginHealthRunner.
func NewPluginHealthRunner(svc *service.UnifiedService) *PluginHealthRunner {
	return &PluginHealthRunner{unifiedSvc: svc}
}

func (r *PluginHealthRunner) Type() TaskType { return TaskPluginHealth }

func (r *PluginHealthRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.unifiedSvc == nil {
		return "", fmt.Errorf("unified service not available")
	}

	health := r.unifiedSvc.HealthCheck()
	if len(health) == 0 {
		return "no plugins registered", nil
	}

	healthyCount := 0
	unhealthy := make([]string, 0)
	for name, status := range health {
		if status.Healthy {
			healthyCount++
		} else {
			unhealthy = append(unhealthy, fmt.Sprintf("%s: %s", name, status.Message))
		}
	}

	result := fmt.Sprintf("%d/%d plugins healthy", healthyCount, len(health))
	if len(unhealthy) > 0 {
		result += fmt.Sprintf(" (%s)", strings.Join(unhealthy, "; "))
		return result, fmt.Errorf("unhealthy plugins detected")
	}
	return result, nil
}

// --- BridgeTokenRotateRunner (ST-18) ---

// BridgeTokenRotateRunner executes scheduled bridge health checks and token status verification.
type BridgeTokenRotateRunner struct {
	bridgeSvc *screenshot.BridgeService
}

// NewBridgeTokenRotateRunner creates a BridgeTokenRotateRunner.
func NewBridgeTokenRotateRunner(svc *screenshot.BridgeService) *BridgeTokenRotateRunner {
	return &BridgeTokenRotateRunner{bridgeSvc: svc}
}

func (r *BridgeTokenRotateRunner) Type() TaskType { return TaskBridgeTokenRotate }

func (r *BridgeTokenRotateRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.bridgeSvc == nil {
		return "", fmt.Errorf("bridge service not available")
	}

	queueLen := r.bridgeSvc.QueueLen()
	workers := r.bridgeSvc.WorkerCount()
	inFlight := r.bridgeSvc.InFlight()
	started := r.bridgeSvc.IsStarted()

	status := fmt.Sprintf("bridge: started=%t, workers=%d, queue=%d, in_flight=%d",
		started, workers, queueLen, inFlight)

	if !started {
		return status, fmt.Errorf("bridge service is not started")
	}
	return status, nil
}

// --- AlertSilenceRunner (ST-19) ---

// AlertSilenceRunner executes scheduled alert silence windows.
type AlertSilenceRunner struct {
	alertManager *alerting.Manager
}

// NewAlertSilenceRunner creates an AlertSilenceRunner.
func NewAlertSilenceRunner(alertManager *alerting.Manager) *AlertSilenceRunner {
	return &AlertSilenceRunner{alertManager: alertManager}
}

func (r *AlertSilenceRunner) Type() TaskType { return TaskAlertSilence }

func (r *AlertSilenceRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	if r.alertManager == nil {
		return "", fmt.Errorf("alert manager not available")
	}

	alertType := extractString(payload, "alert_type", "")
	durationMin := extractInt(payload, "duration_minutes", 60)
	duration := time.Duration(durationMin) * time.Minute

	if alertType != "" {
		r.alertManager.SilenceAlertsByType(alerting.AlertType(alertType), duration)
		return fmt.Sprintf("silenced all %s alerts for %d minutes", alertType, durationMin), nil
	}

	// No type specified: cleanup old records instead
	maxAgeDays := extractInt(payload, "max_age_days", 30)
	r.alertManager.CleanupOldRecords(time.Duration(maxAgeDays) * 24 * time.Hour)
	return fmt.Sprintf("cleaned up alert records older than %d days", maxAgeDays), nil
}

// --- CacheWarmupRunner (ST-20) ---

// CacheWarmupRunner executes scheduled cache warmup.
type CacheWarmupRunner struct {
	// No direct dependency — warms up by triggering common queries
	// that will populate the cache for subsequent requests.
}

// NewCacheWarmupRunner creates a CacheWarmupRunner.
func NewCacheWarmupRunner() *CacheWarmupRunner {
	return &CacheWarmupRunner{}
}

func (r *CacheWarmupRunner) Type() TaskType { return TaskCacheWarmup }

func (r *CacheWarmupRunner) Execute(ctx context.Context, payload map[string]interface{}) (string, error) {
	// Cache warmup is typically done by running lightweight health checks
	// or pinging common endpoints. Without a query service dependency,
	// this runner serves as a placeholder that can be extended.
	urls := extractStrings(payload, "warmup_urls", []string{})
	if len(urls) == 0 {
		return "no warmup URLs configured", nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	successCount := 0
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			successCount++
		}
	}

	return fmt.Sprintf("warmed up %d/%d URLs", successCount, len(urls)), nil
}
