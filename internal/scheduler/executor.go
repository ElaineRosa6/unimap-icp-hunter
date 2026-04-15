package scheduler

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/unimap-icp-hunter/project/internal/distributed"
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
