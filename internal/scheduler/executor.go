package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unimap/project/internal/adapter"
	"github.com/unimap/project/internal/alerting"
	"github.com/unimap/project/internal/backup"
	"github.com/unimap/project/internal/distributed"
	"github.com/unimap/project/internal/exporter"
	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/screenshot"
	"github.com/unimap/project/internal/screenshot/batchdb"
	"github.com/unimap/project/internal/service"
	"github.com/unimap/project/internal/tamper"
	"github.com/unimap/project/internal/utils"
)

// --- QueryRunner (ST-01) ---

const (
	defaultQueryNotificationDetailLimit = 50
	maxQueryNotificationDetailLimit     = 100
	// maxQueryNotificationDetailBytes bounds the asset detail body so the whole
	// notification fits under the WeCom markdown limit (4096 bytes after the
	// channel header) while leaving room for the "另有 N 条已持久化" trailer.
	maxQueryNotificationDetailBytes = 3800
	// maxQueryNotificationDetailBytesAllowed caps a per-task override
	// (notification_detail_bytes) so a single misconfigured task cannot build an
	// unbounded notification body. Channels without the WeCom-size constraint
	// (e.g. the smtp-relay email path) can carry the full table, so email tasks
	// may raise the budget well above the WeCom markdown limit.
	maxQueryNotificationDetailBytesAllowed = 40000
)

var notificationWhitespace = regexp.MustCompile(`\s+`)

// QueryRunner executes scheduled UQL queries via QueryAppService.
type QueryRunner struct {
	querySvc      *service.QueryAppService
	screenshotSvc *service.ScreenshotAppService
	mgr           *screenshot.Manager
	browserRouter service.BrowserRouter
	health        *SessionHealthTracker // shared session health tracker for circuit breaking
	exportDir     string                // directory for format=excel workbooks (default AppDataDir("exports"))
}

// NewQueryRunner creates a QueryRunner.
func NewQueryRunner(b *service.QueryAppService) *QueryRunner {
	return &QueryRunner{querySvc: b}
}

// NewQueryRunnerWithBrowser creates a query runner capable of completing the
// durable Bridge collect+capture workflow. An optional SessionHealthTracker
// enables per-engine circuit breaking for browser tasks.
func NewQueryRunnerWithBrowser(b *service.QueryAppService, screenshotSvc *service.ScreenshotAppService, mgr *screenshot.Manager, browserRouter service.BrowserRouter, health ...*SessionHealthTracker) *QueryRunner {
	r := &QueryRunner{querySvc: b, screenshotSvc: screenshotSvc, mgr: mgr, browserRouter: browserRouter}
	if len(health) > 0 && health[0] != nil {
		r.health = health[0]
	}
	return r
}

func (r *QueryRunner) Type() TaskType { return TaskQuery }

func (r *QueryRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.querySvc == nil {
		return "", fmt.Errorf("query service not available")
	}

	query := extractString(payload, "query", "")
	if query == "" {
		return "", fmt.Errorf("%s runner: missing 'query' in payload", r.Type())
	}

	engines := extractStrings(payload, "engines", nil)
	if len(engines) == 0 {
		engines = extractStrings(payload, "engine", []string{})
	}
	engines = r.querySvc.ResolveEngines(engines)
	if len(engines) == 0 {
		return "", fmt.Errorf("no query engines available")
	}
	pageSize := payload.PageSize
	if pageSize == 0 {
		pageSize = 100
	}

	var resp *service.QueryResponse
	var browserOutcome service.BrowserQueryOutcome
	var err error
	if payload.BrowserQuery {
		action := strings.TrimSpace(payload.BrowserAction)
		if action == "" {
			action = "collect_and_capture"
		}
		if action != "collect_and_capture" {
			return "", fmt.Errorf("scheduled browser query requires browser_action=collect_and_capture")
		}
		if r.browserRouter == nil || r.screenshotSvc == nil {
			return "", fmt.Errorf("scheduled browser query requires an available Bridge screenshot provider")
		}
		// Circuit breaker: skip engines whose session health is tripped.
		if r.health != nil {
			var allowed []string
			var blocked []string
			for _, eng := range engines {
				if r.health.AllowBrowserTask(eng) {
					allowed = append(allowed, eng)
				} else {
					blocked = append(blocked, eng)
				}
			}
			if len(blocked) > 0 {
				logger.Warnf("browser query: skipping circuit-open engines: %s", strings.Join(blocked, ", "))
			}
			if len(allowed) == 0 {
				return "", fmt.Errorf("all engines circuit-open, browser query skipped (blocked: %s)", strings.Join(blocked, ", "))
			}
			engines = allowed
		}
		queryID := strings.TrimSpace(payload.QueryID)
		if queryID == "" {
			queryID = fmt.Sprintf("scheduled_query_%d", time.Now().UnixNano())
		}
		resp, browserOutcome, err = r.querySvc.ExecuteQueryWithBrowserWorkflow(ctx, query, engines, pageSize, service.BrowserQueryWorkflowOptions{
			Action:             action,
			QueryID:            queryID,
			AutoCaptureEnabled: true,
			ScreenshotApp:      r.screenshotSvc,
			ScreenshotManager:  r.mgr,
			BrowserRouter:      r.browserRouter,
			RequireComplete:    true,
			RequirePersistence: true,
		})
	} else {
		resp, err = r.querySvc.ExecuteQuery(ctx, query, engines, pageSize)
	}
	if err != nil {
		return "", fmt.Errorf("query execution failed: %w", err)
	}

	// Incremental push: drop assets already pushed for this task, keeping only
	// the new fingerprints. The task name scopes the dedup set (see
	// taskNameFromContext); a re-created task with the same name keeps its
	// pushed history. Assets without an IP key cannot be deduplicated and are
	// always delivered, but never recorded.
	var onlyNewTaskID string
	var onlyNewKeys []string
	var pushed map[string]struct{}
	if payload.OnlyNew {
		onlyNewTaskID = taskNameFromContext(ctx)
		if onlyNewTaskID == "" {
			return "", fmt.Errorf("only_new query task requires a task name")
		}
		pushed, err = r.querySvc.LoadPushedAssetKeys(onlyNewTaskID)
		if err != nil {
			return "", fmt.Errorf("load pushed asset keys for %s: %w", onlyNewTaskID, err)
		}
		fresh := make([]model.UnifiedAsset, 0, len(resp.Assets))
		freshKeys := make([]string, 0, len(resp.Assets))
		for _, asset := range resp.Assets {
			key := asset.Key()
			if key == "" {
				fresh = append(fresh, asset)
				continue
			}
			if _, seen := pushed[key]; seen {
				continue
			}
			fresh = append(fresh, asset)
			freshKeys = append(freshKeys, key)
		}
		resp.Assets = fresh
		resp.TotalCount = len(fresh)
		onlyNewKeys = freshKeys
	}

	// Optional Excel export: when format is excel/xlsx and assets remain after
	// the incremental filter, export the (deduplicated) assets to a workbook and
	// embed its path so the notification layer delivers it as a file message.
	// No assets means no workbook is produced (the "no new assets" message still
	// goes out as text).
	var excelPath string
	if strings.EqualFold(payload.Format, "excel") || strings.EqualFold(payload.Format, "xlsx") {
		if len(resp.Assets) > 0 {
			if excelPath, err = r.exportAssetsExcel(ctx, resp.Assets); err != nil {
				return "", fmt.Errorf("export query assets to excel: %w", err)
			}
		}
	}

	var b strings.Builder
	// Compact header: engine + total. The raw UQL is intentionally not replayed
	// here — the scheduled task name already identifies the query, and dumping a
	// 40-clause query made the push unusable while crowding asset rows out of
	// the notification byte budget.
	if payload.OnlyNew && resp.TotalCount > 0 {
		fmt.Fprintf(&b, "**查询完成｜引擎: %s ｜新增 %d 条（去重后）**\n", strings.Join(engines, "+"), resp.TotalCount)
	} else if payload.OnlyNew {
		fmt.Fprintf(&b, "**查询完成｜引擎: %s ｜无新增资产（已全部推送过）**\n", strings.Join(engines, "+"))
	} else {
		fmt.Fprintf(&b, "**查询完成｜引擎: %s ｜返回 %d 条**\n", strings.Join(engines, "+"), resp.TotalCount)
	}
	if len(resp.EngineStats) > 1 {
		for eng, count := range resp.EngineStats {
			fmt.Fprintf(&b, "✅ %s: %d 条  ", eng, count)
		}
		fmt.Fprintf(&b, "\n")
	}
	for _, e := range resp.Errors {
		fmt.Fprintf(&b, "❌ %s\n", e)
	}
	appendQueryAssetDetails(&b, resp.Assets, queryNotificationDetailLimit(payload), queryNotificationDetailBytes(payload))
	if payload.BrowserQuery {
		enginesWithScreenshots := make([]string, 0, len(browserOutcome.AutoCapturedPaths))
		for engine := range browserOutcome.AutoCapturedPaths {
			enginesWithScreenshots = append(enginesWithScreenshots, engine)
		}
		sort.Strings(enginesWithScreenshots)
		for _, engine := range enginesWithScreenshots {
			path := browserOutcome.AutoCapturedPaths[engine]
			if _, statErr := os.Stat(path); statErr != nil {
				return "", fmt.Errorf("Bridge screenshot unavailable for %s: %w", engine, statErr)
			}
			fmt.Fprintf(&b, "✅ %s Bridge 截图保存: %s\n", engine, path)
		}
		fmt.Fprintf(&b, "✅ Bridge 采集结果已合并并持久化\n")
	}
	if payload.OnlyNew && len(onlyNewKeys) > 0 {
		if err := r.querySvc.RecordPushedAssets(onlyNewTaskID, onlyNewKeys); err != nil {
			return "", fmt.Errorf("record pushed asset keys for %s: %w", onlyNewTaskID, err)
		}
	}
	if excelPath != "" {
		fmt.Fprintf(&b, "✅ Excel 文件保存: %s\n", excelPath)
	}
	return sanitizeUTF8(b.String()), nil
}

// WithExportDir sets the directory where format=excel query tasks write their
// workbooks. When empty, the default AppDataDir("exports") is used.
func (r *QueryRunner) WithExportDir(dir string) *QueryRunner {
	r.exportDir = dir
	return r
}

// exportAssetsExcel exports the given assets to a timestamped xlsx workbook
// named after the task (task names are scheduler identifiers; sanitized for use
// as a file name). Returns the written path.
func (r *QueryRunner) exportAssetsExcel(ctx context.Context, assets []model.UnifiedAsset) (string, error) {
	dir := strings.TrimSpace(r.exportDir)
	if dir == "" {
		dir = utils.AppDataDir("exports")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := "query"
	if taskName := taskNameFromContext(ctx); taskName != "" {
		name = safeFileName(taskName)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s_%d.xlsx", name, time.Now().Unix()))
	if err := exporter.NewExcelExporter().ExportFull(assets, path); err != nil {
		return "", err
	}
	return path, nil
}

// safeFileName strips characters unsafe for a file name component from a task
// name (path separators, whitespace, punctuation).
func safeFileName(name string) string {
	return regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(strings.TrimSpace(name), "_")
}

func queryNotificationDetailLimit(payload *model.TaskPayload) int {
	limit := extractInt(payload, "notification_detail_limit", defaultQueryNotificationDetailLimit)
	if limit < 1 {
		return defaultQueryNotificationDetailLimit
	}
	if limit > maxQueryNotificationDetailLimit {
		return maxQueryNotificationDetailLimit
	}
	return limit
}

// queryNotificationDetailBytes returns the per-task byte budget for the asset
// detail body. Defaults to the WeCom-markdown-safe 3800 bytes; tasks whose
// notifications go to channels without that size limit (e.g. email) may raise
// it via the payload field notification_detail_bytes so the full table fits.
func queryNotificationDetailBytes(payload *model.TaskPayload) int {
	bytes := extractInt(payload, "notification_detail_bytes", maxQueryNotificationDetailBytes)
	if bytes < 1 {
		return maxQueryNotificationDetailBytes
	}
	if bytes > maxQueryNotificationDetailBytesAllowed {
		return maxQueryNotificationDetailBytesAllowed
	}
	return bytes
}

// queryNotificationTableHeader opens the markdown pipe-table for asset rows.
// DingTalk/Feishu and the WeCom client render pipe tables; cells never contain
// '|' because queryAssetRow escapes them.
const queryNotificationTableHeader = "\n| 资产 | 标题 | 状态 |\n| --- | --- | --- |\n"

func appendQueryAssetDetails(b *strings.Builder, assets []model.UnifiedAsset, limit int, byteBudget int) {
	if len(assets) == 0 {
		return
	}
	// Reserve room for the trailer line so "另有 N 条已持久化" survives the byte
	// budget instead of being silently dropped by the truncation.
	const trailerReserve = 96
	b.WriteString(queryNotificationTableHeader)
	shown := 0
	for shown < min(len(assets), limit) {
		row := queryAssetRow(assets[shown])
		if b.Len()+len(row) > byteBudget-trailerReserve {
			break
		}
		b.WriteString(row)
		shown++
	}
	if remaining := len(assets) - shown; remaining > 0 {
		fmt.Fprintf(b, "… 另有 %d 条结果已持久化，通知中未展开。\n", remaining)
	}
}

// queryAssetRow renders one asset as a compact single table row. Columns are
// 资产 (host:port), 标题, 状态 — the fields that matter most for triage while
// keeping each row small enough to show far more assets than the old verbose
// multi-line bullet format.
func queryAssetRow(asset model.UnifiedAsset) string {
	target := assetEndpoint(asset)
	if target == "" {
		target = firstNonEmpty(asset.Host, asset.IP, asset.URL, "未知")
	}
	return fmt.Sprintf("| %s | %s | %s |\n",
		tableCell(target, 22),
		tableCell(notificationField(asset.Title), 22),
		tableCell(assetProtocolStatus(asset), 16),
	)
}

// tableCell sanitizes a value for a markdown table cell: escapes pipes and
// newlines (which would break the row layout) and truncates to maxRunes.
func tableCell(v string, maxRunes int) string {
	v = strings.ReplaceAll(v, "|", "｜")
	v = strings.ReplaceAll(v, "\n", " ")
	return truncateRunes(v, maxRunes)
}

// truncateRunes truncates s to at most maxRunes runes (CJK-safe) and appends
// an ellipsis when text was cut.
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes == 1 {
		return string(runes[:1])
	}
	return string(runes[:maxRunes-1]) + "…"
}

func assetEndpoint(asset model.UnifiedAsset) string {
	return asset.Key()
}

func assetProtocolStatus(asset model.UnifiedAsset) string {
	parts := make([]string, 0, 2)
	if asset.Protocol != "" {
		parts = append(parts, asset.Protocol)
	}
	if asset.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", asset.StatusCode))
	}
	return strings.Join(parts, " / ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "未知资产"
}

func notificationField(value string) string {
	value = notificationWhitespace.ReplaceAllString(strings.TrimSpace(value), " ")
	value = strings.NewReplacer("<", "‹", ">", "›", "`", "'", "[", "［", "]", "］").Replace(value)
	runes := []rune(value)
	if len(runes) > 160 {
		value = string(runes[:157]) + "..."
	}
	return value
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

func (r *SearchScreenshotRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.screenshotSvc == nil {
		return "", fmt.Errorf("screenshot service not available")
	}

	engine := extractString(payload, "engine", "")
	query := extractString(payload, "query", "")
	queryID := extractString(payload, "query_id", "")

	if engine == "" || query == "" {
		return "", fmt.Errorf("%s runner: missing 'engine' or 'query' in payload", r.Type())
	}

	path, eng, q, id, err := r.screenshotSvc.CaptureSearchEngineResult(ctx, r.mgr, engine, query, queryID)
	if err != nil {
		return "", fmt.Errorf("screenshot capture failed: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "搜索引擎截图完成\n\n")
	fmt.Fprintf(&b, "✅ 引擎: %s\n", eng)
	fmt.Fprintf(&b, "✅ 查询: %s\n", q)
	fmt.Fprintf(&b, "✅ 保存: %s\n", path)
	if id != "" {
		fmt.Fprintf(&b, "✅ 查询ID: %s\n", id)
	}
	return sanitizeUTF8(b.String()), nil
}

// --- BatchScreenshotRunner (ST-03) ---

// BatchScreenshotRunner executes scheduled batch URL screenshots.
type BatchScreenshotRunner struct {
	screenshotSvc *service.ScreenshotAppService
	mgr           *screenshot.Manager
	repo          *batchdb.Repository
}

// NewBatchScreenshotRunner creates a BatchScreenshotRunner.
func NewBatchScreenshotRunner(svc *service.ScreenshotAppService, mgr *screenshot.Manager, repos ...*batchdb.Repository) *BatchScreenshotRunner {
	r := &BatchScreenshotRunner{screenshotSvc: svc, mgr: mgr}
	if len(repos) > 0 {
		r.repo = repos[0]
	}
	return r
}

func (r *BatchScreenshotRunner) Type() TaskType { return TaskBatchScreenshot }

func (r *BatchScreenshotRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.screenshotSvc == nil {
		return "", fmt.Errorf("screenshot service not available")
	}

	urls := extractStrings(payload, "urls", []string{})
	if len(urls) == 0 {
		return "", fmt.Errorf("%s runner: missing 'urls' in payload", r.Type())
	}

	batchID := extractString(payload, "batch_id", "")
	if batchID == "" {
		batchID = fmt.Sprintf("scheduled_%d", time.Now().UnixNano())
	}
	concurrency := extractInt(payload, "concurrency", 5)

	req := service.BatchURLsRequest{
		URLs:        urls,
		BatchID:     batchID,
		Concurrency: concurrency,
	}

	startedAt := time.Now()
	if r.repo != nil {
		if err := r.repo.SaveJob(&batchdb.BatchJobRecord{ID: batchID, Status: "running", Total: len(urls), StartedAt: startedAt}); err != nil {
			return "", fmt.Errorf("persist batch screenshot start: %w", err)
		}
	}
	resp, err := r.screenshotSvc.CaptureBatchURLs(ctx, r.mgr, req)
	if err != nil {
		if r.repo != nil {
			endedAt := time.Now()
			if saveErr := r.repo.SaveJob(&batchdb.BatchJobRecord{ID: batchID, Status: "failed", Total: len(urls), Error: err.Error(), StartedAt: startedAt, EndedAt: &endedAt}); saveErr != nil {
				return "", fmt.Errorf("batch screenshot failed: %v; persist failure: %w", err, saveErr)
			}
		}
		return "", fmt.Errorf("batch screenshot failed: %w", err)
	}
	if r.repo != nil {
		endedAt := time.Now()
		if err := r.repo.SaveJob(&batchdb.BatchJobRecord{ID: batchID, Status: "completed", Total: resp.Total, Completed: len(resp.Results), Success: resp.Success, Failed: resp.Failed, Results: resp.Results, StartedAt: startedAt, EndedAt: &endedAt}); err != nil {
			return "", fmt.Errorf("persist completed batch screenshot: %w", err)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "批量截图完成：%d/%d 成功\n\n", resp.Success, resp.Total)
	for _, r := range resp.Results {
		if r.Success {
			fmt.Fprintf(&b, "✅ %s → %s\n", r.URL, r.FilePath)
		} else {
			fmt.Fprintf(&b, "❌ %s — %s\n", r.URL, r.Error)
		}
	}
	fmt.Fprintf(&b, "\n📁 截图目录: %s", resp.ScreenshotDir)
	return sanitizeUTF8(b.String()), nil
}

// --- TamperCheckRunner (ST-04) ---

// TamperCheckRunner executes scheduled tamper checks.
type TamperCheckRunner struct {
	tamperSvc       *service.TamperAppService
	pageLoader      service.TamperPageLoader
	evidenceCapture tamperEvidenceCapturer
	evidenceManager *screenshot.Manager
	evidenceEnabled func() bool
}

// NewTamperCheckRunner creates a TamperCheckRunner.
func NewTamperCheckRunner(svc *service.TamperAppService, loader service.TamperPageLoader) *TamperCheckRunner {
	return &TamperCheckRunner{tamperSvc: svc, pageLoader: loader}
}

type tamperEvidenceCapturer interface {
	CaptureTargetWebsite(
		ctx context.Context,
		mgr *screenshot.Manager,
		targetURL, ip, port, protocol, queryID string,
	) (path, normalizedURL, normalizedIP, normalizedPort, normalizedProtocol, normalizedQueryID string, err error)
}

// NewTamperCheckRunnerWithEvidence creates a tamper runner with optional
// evidence capture. Production keeps enabled=false until controlled cloud
// page-change and browser SSRF acceptance have both passed.
func NewTamperCheckRunnerWithEvidence(
	svc *service.TamperAppService,
	loader service.TamperPageLoader,
	capturer tamperEvidenceCapturer,
	mgr *screenshot.Manager,
	enabled func() bool,
) *TamperCheckRunner {
	return &TamperCheckRunner{
		tamperSvc:       svc,
		pageLoader:      loader,
		evidenceCapture: capturer,
		evidenceManager: mgr,
		evidenceEnabled: enabled,
	}
}

func (r *TamperCheckRunner) Type() TaskType { return TaskTamperCheck }

func (r *TamperCheckRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.tamperSvc == nil {
		return "", fmt.Errorf("tamper service not available")
	}

	urls := extractStrings(payload, "urls", []string{})
	if len(urls) == 0 {
		return "", fmt.Errorf("%s runner: missing 'urls' in payload", r.Type())
	}

	concurrency := extractInt(payload, "concurrency", 5)
	mode := extractString(payload, "detection_mode", "relaxed")

	req := service.TamperCheckRequest{
		URLs:        urls,
		Concurrency: concurrency,
		Mode:        mode,
	}

	resp, err := r.tamperSvc.Check(ctx, req, r.pageLoader)
	if err != nil {
		return "", fmt.Errorf("tamper check failed: %w", err)
	}
	evidencePaths, err := r.captureEvidence(ctx, resp.Results)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "篡改检测完成（模式: %s）：共 %d 个 URL\n\n", mode, len(resp.Results))
	for i, r := range resp.Results {
		switch r.Status {
		case "tampered":
			fmt.Fprintf(&b, "⚠️ 已篡改 %s", r.URL)
			if len(r.TamperedSegments) > 0 {
				fmt.Fprintf(&b, " — 变更区域: %s", strings.Join(r.TamperedSegments, ", "))
			}
			b.WriteString("\n")
			if path := evidencePaths[i]; path != "" {
				fmt.Fprintf(&b, "  📷 证据截图保存: %s\n", path)
			}
		case "no_baseline":
			fmt.Fprintf(&b, "🆕 首次检测 %s — 已建立基线\n", r.URL)
		case "unreachable":
			fmt.Fprintf(&b, "❌ 不可达 %s", r.URL)
			if r.ErrorMessage != "" {
				fmt.Fprintf(&b, " — %s", r.ErrorMessage)
			}
			b.WriteString("\n")
		case "normal":
			fmt.Fprintf(&b, "✅ 正常 %s\n", r.URL)
		default:
			fmt.Fprintf(&b, "❓ %s %s\n", r.Status, r.URL)
		}
	}
	return sanitizeUTF8(b.String()), nil
}

func (r *TamperCheckRunner) captureEvidence(ctx context.Context, results []tamper.TamperCheckResult) (map[int]string, error) {
	if r.evidenceEnabled == nil || !r.evidenceEnabled() {
		return nil, nil
	}
	if r.evidenceCapture == nil {
		return nil, fmt.Errorf("tamper evidence capture is enabled but screenshot provider is unavailable")
	}

	paths := make(map[int]string)
	for i := range results {
		result := &results[i]
		if !result.Tampered && !strings.EqualFold(strings.TrimSpace(result.Status), "tampered") {
			continue
		}
		queryID := fmt.Sprintf("tamper_evidence_%d_%d", time.Now().UnixNano(), i)
		path, _, _, _, _, _, err := r.evidenceCapture.CaptureTargetWebsite(
			ctx,
			r.evidenceManager,
			result.URL,
			"",
			"",
			"",
			queryID,
		)
		if err != nil {
			return nil, fmt.Errorf("capture tamper evidence for %s: %w", result.URL, err)
		}
		path = strings.TrimSpace(path)
		if path == "" {
			return nil, fmt.Errorf("capture tamper evidence for %s: screenshot path is empty", result.URL)
		}
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("capture tamper evidence for %s: verify screenshot: %w", result.URL, err)
		}
		paths[i] = path
	}
	return paths, nil
}

// --- URLReachabilityRunner (ST-05) ---

// URLReachabilityRunner executes scheduled URL reachability checks.
type URLReachabilityRunner struct {
	monitorSvc *service.MonitorAppService
	alerts     *alerting.Manager
}

// NewURLReachabilityRunner creates a URLReachabilityRunner.
func NewURLReachabilityRunner(svc *service.MonitorAppService, alerts ...*alerting.Manager) *URLReachabilityRunner {
	r := &URLReachabilityRunner{monitorSvc: svc}
	if len(alerts) > 0 {
		r.alerts = alerts[0]
	}
	return r
}

func (r *URLReachabilityRunner) Type() TaskType { return TaskURLReachability }

func (r *URLReachabilityRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.monitorSvc == nil {
		return "", fmt.Errorf("monitor service not available")
	}

	urls := extractStrings(payload, "urls", []string{})
	if len(urls) == 0 {
		return "", fmt.Errorf("%s runner: missing 'urls' in payload", r.Type())
	}

	concurrency := extractInt(payload, "concurrency", 5)

	resp, err := r.monitorSvc.CheckURLReachability(ctx, urls, concurrency)
	if err != nil {
		return "", fmt.Errorf("reachability check failed: %w", err)
	}
	if r.alerts != nil {
		for _, item := range resp.Results {
			if !item.Reachable {
				r.alerts.SendWarning(alerting.AlertTypeReachability, "URL 不可达", item.Reason, item, "scheduler", item.Input)
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "URL 可达性检测完成：%d 个 URL\n\n", resp.Summary.Total)
	for _, r := range resp.Results {
		if r.Reachable {
			detail := r.Input
			if r.HTTPStatus > 0 {
				detail = fmt.Sprintf("%s (HTTP %d)", r.Input, r.HTTPStatus)
			}
			fmt.Fprintf(&b, "✅ 可达 %s\n", detail)
		} else {
			detail := r.Input
			if r.Reason != "" {
				detail += " — " + r.Reason
			}
			fmt.Fprintf(&b, "❌ 不可达 %s\n", detail)
		}
	}
	fmt.Fprintf(&b, "\n📊 可达: %d，不可达: %d", resp.Summary.Reachable, resp.Summary.Unreachable)
	return sanitizeUTF8(b.String()), nil
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

func (r *CookieVerifyRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.mgr == nil {
		return "", fmt.Errorf("screenshot manager not available")
	}

	engines := extractStrings(payload, "engines", []string{})
	if len(engines) == 0 {
		// Default: check all supported engines
		engines = []string{"fofa", "hunter", "quake", "zoomeye", "shodan", "censys", "daydaymap"}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Cookie 验证完成：%d 个引擎\n\n", len(engines))
	for _, engine := range engines {
		cookies := r.mgr.GetCookies(engine)
		if len(cookies) > 0 {
			fmt.Fprintf(&b, "✅ %s: %d 个 Cookie 已配置\n", engine, len(cookies))
		} else {
			fmt.Fprintf(&b, "⚠️ %s: 未配置 Cookie\n", engine)
		}
	}
	return sanitizeUTF8(b.String()), nil
}

// --- LoginStatusCheckRunner (ST-07) ---

// LoginStatusCheckRunner executes scheduled login status checks.
type LoginStatusCheckRunner struct {
	mgr    *screenshot.Manager
	health *SessionHealthTracker
}

// NewLoginStatusCheckRunner creates a LoginStatusCheckRunner. An optional
// external SessionHealthTracker can be provided to share state with other
// runners (e.g. QueryRunner circuit breaking); if omitted a new tracker is
// created internally.
func NewLoginStatusCheckRunner(mgr *screenshot.Manager, health ...*SessionHealthTracker) *LoginStatusCheckRunner {
	h := NewSessionHealthTracker()
	if len(health) > 0 && health[0] != nil {
		h = health[0]
	}
	return &LoginStatusCheckRunner{mgr: mgr, health: h}
}

// Health returns the session health tracker for external inspection/notification.
func (r *LoginStatusCheckRunner) Health() *SessionHealthTracker { return r.health }

func (r *LoginStatusCheckRunner) Type() TaskType { return TaskLoginStatusCheck }

func (r *LoginStatusCheckRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.mgr == nil {
		return "", fmt.Errorf("screenshot manager not available")
	}

	engines := extractStrings(payload, "engines", []string{})
	if len(engines) == 0 {
		engines = []string{"fofa", "hunter", "quake", "zoomeye", "shodan", "censys", "daydaymap"}
	}
	testQuery := extractString(payload, "test_query", "test")

	var b strings.Builder
	fmt.Fprintf(&b, "登录状态检查完成：%d 个引擎\n\n", len(engines))
	failedCount := 0
	for _, engine := range engines {
		// Skip check if circuit is open (cooldown not elapsed)
		if !r.health.AllowCheck(engine) {
			h := r.health.GetHealth(engine)
			fmt.Fprintf(&b, "⏸️ %s: 熔断中（连续失败 %d 次，%s后重试）\n", engine, h.ConsecutiveFails, h.LastFailure)
			continue
		}

		status, err := r.mgr.CheckEngineLoginStatus(ctx, engine, testQuery)
		if err != nil {
			category := ClassifyFailureReason("", err.Error())
			r.health.RecordFailure(engine, category, err.Error())
			fmt.Fprintf(&b, "❌ %s: 检查失败 [%s] — %v\n", engine, category, err)
			fmt.Fprintf(&b, "   💡 %s\n", RecoveryHint(category))
			failedCount++
			continue
		}
		if status.LoggedIn {
			r.health.RecordSuccess(engine)
			fmt.Fprintf(&b, "✅ %s: 已登录", engine)
		} else {
			category := ClassifyFailureReason(status.Reason, status.Error)
			r.health.RecordFailure(engine, category, status.Reason)
			fmt.Fprintf(&b, "❌ %s: 未登录 [%s]", engine, category)
			failedCount++
			fmt.Fprintf(&b, "\n   💡 %s", RecoveryHint(category))
		}
		if status.Reason != "" {
			fmt.Fprintf(&b, " (%s)", status.Reason)
		}
		b.WriteString("\n")
	}

	// Append circuit breaker summary if any engine is tripped
	summary := r.health.Summary()
	if summary != "" && summary != "no engine health data" {
		fmt.Fprintf(&b, "\n--- 会话健康 ---\n%s", summary)
	}

	result := sanitizeUTF8(b.String())
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

func (r *DistributedSubmitRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.taskQueue == nil {
		return "", fmt.Errorf("task queue not available")
	}

	taskType := extractString(payload, "task_type", "")
	if taskType == "" {
		return "", fmt.Errorf("%s runner: missing 'task_type' in payload", r.Type())
	}

	taskPayload := make(map[string]any)
	if payload.Extra != nil {
		if p, ok := payload.Extra["task_payload"]; ok {
			if pm, ok := p.(map[string]any); ok {
				taskPayload = pm
			}
		}
	}

	priority := extractInt(payload, "priority", 0)
	timeoutSec := extractInt(payload, "timeout_seconds", 300)
	maxReassign := extractInt(payload, "max_reassign", 3)

	// Convert taskPayload map to typed struct
	var typedPayload *model.TaskPayload
	if len(taskPayload) > 0 {
		raw, err := json.Marshal(taskPayload)
		if err == nil {
			var p model.TaskPayload
			_ = json.Unmarshal(raw, &p)
			typedPayload = &p
		}
	}

	// Build the envelope
	envelope := distributed.TaskEnvelope{
		TaskID:         generateDistributedTaskID(),
		TaskType:       taskType,
		Payload:        typedPayload,
		Priority:       priority,
		TimeoutSeconds: timeoutSec,
		MaxReassign:    maxReassign,
	}

	if _, err := r.taskQueue.Enqueue(envelope); err != nil {
		return "", fmt.Errorf("enqueue failed: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "分布式任务已提交\n\n")
	fmt.Fprintf(&b, "✅ 任务ID: %s\n", envelope.TaskID)
	fmt.Fprintf(&b, "✅ 任务类型: %s\n", taskType)
	fmt.Fprintf(&b, "✅ 优先级: %d\n", priority)
	fmt.Fprintf(&b, "✅ 超时: %ds\n", timeoutSec)
	fmt.Fprintf(&b, "✅ 最大重分配: %d\n", maxReassign)
	return sanitizeUTF8(b.String()), nil
}

// distributedIDCounter is a monotonic counter for unique distributed task IDs.
var distributedIDCounter atomic.Int64

// BackupRunner executes scheduled archives of selected application data.
type BackupRunner struct{ snapshotter backup.SQLiteSnapshotter }

func NewBackupRunner(snapshotters ...backup.SQLiteSnapshotter) *BackupRunner {
	snapshotter := backup.SQLiteSnapshotterFor()
	if len(snapshotters) > 0 && snapshotters[0] != nil {
		snapshotter = snapshotters[0]
	}
	return &BackupRunner{snapshotter: snapshotter}
}
func (r *BackupRunner) Type() TaskType { return TaskBackup }
func (r *BackupRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	sources := extractStrings(payload, "sources", []string{})
	if len(sources) == 0 {
		return "", fmt.Errorf("%s runner: missing 'sources' in payload", r.Type())
	}
	snapshotter := r.snapshotter
	if snapshotter == nil {
		snapshotter = backup.SQLiteSnapshotterFor()
	}
	result, err := backup.BackupContext(ctx, backup.BackupConfig{
		SQLiteSnapshotter: snapshotter,
		Sources:           sources, OutputDir: extractString(payload, "output_dir", ""),
		Prefix: extractString(payload, "prefix", "unimap"), MaxBackups: extractInt(payload, "max_backups", 7),
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("备份完成：%s（%d 字节）", result.Path, result.Size), nil
}

// generateDistributedTaskID creates a unique ID for distributed task envelopes.
func generateDistributedTaskID() string {
	return fmt.Sprintf("dist_%d", distributedIDCounter.Add(1))
}

// --- ExportRunner (ST-09) ---

// ExportRunner executes scheduled data exports.
type ExportRunner struct {
	queryApp     *service.QueryAppService
	orchestrator *adapter.EngineOrchestrator
	outputDir    string
}

// NewExportRunner creates an ExportRunner.
func NewExportRunner(queryApp *service.QueryAppService, orchestrator *adapter.EngineOrchestrator, outputDir string) *ExportRunner {
	return &ExportRunner{queryApp: queryApp, orchestrator: orchestrator, outputDir: outputDir}
}

func (r *ExportRunner) Type() TaskType { return TaskExport }

func (r *ExportRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.queryApp == nil || r.orchestrator == nil {
		return "", fmt.Errorf("query service or orchestrator not available")
	}

	query := extractString(payload, "query", "")
	if query == "" {
		return "", fmt.Errorf("%s runner: missing 'query' in payload", r.Type())
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
		var b strings.Builder
		fmt.Fprintf(&b, "数据导出完成\n\n")
		fmt.Fprintf(&b, "⚠️ 查询: %s\n", query)
		fmt.Fprintf(&b, "⚠️ 引擎: %s\n", strings.Join(engines, ","))
		fmt.Fprintf(&b, "⚠️ 结果: 无数据可导出\n")
		return sanitizeUTF8(b.String()), nil
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
	var exp exporter.Exporter
	switch format {
	case "excel", "xlsx":
		exp = exporter.NewExcelExporter()
	default:
		exp = exporter.NewJSONExporter()
	}
	if err := exp.Export(resp.Assets, outPath); err != nil {
		return "", fmt.Errorf("export failed: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "数据导出完成\n\n")
	fmt.Fprintf(&b, "✅ 查询: %s\n", query)
	fmt.Fprintf(&b, "✅ 引擎: %s\n", strings.Join(engines, ","))
	fmt.Fprintf(&b, "✅ 格式: %s\n", format)
	fmt.Fprintf(&b, "✅ 资产数: %d\n", len(resp.Assets))
	fmt.Fprintf(&b, "✅ 保存: %s\n", outPath)
	return sanitizeUTF8(b.String()), nil
}

// --- PortScanRunner (ST-10) ---

// PortScanRunner executes scheduled port scans.
type PortScanRunner struct {
	monitorSvc *service.MonitorAppService
	alerts     *alerting.Manager
}

// NewPortScanRunner creates a PortScanRunner.
func NewPortScanRunner(svc *service.MonitorAppService, alerts ...*alerting.Manager) *PortScanRunner {
	r := &PortScanRunner{monitorSvc: svc}
	if len(alerts) > 0 {
		r.alerts = alerts[0]
	}
	return r
}

func (r *PortScanRunner) Type() TaskType { return TaskPortScan }

func (r *PortScanRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.monitorSvc == nil {
		return "", fmt.Errorf("monitor service not available")
	}

	urls := extractStrings(payload, "urls", []string{})
	if len(urls) == 0 {
		return "", fmt.Errorf("%s runner: missing 'urls' in payload", r.Type())
	}

	concurrency := extractInt(payload, "concurrency", 5)
	portSpec := strings.TrimSpace(extractString(payload, "port_spec", ""))
	if strings.EqualFold(strings.TrimSpace(extractString(payload, "scan_mode", "")), "full") {
		portSpec = "all"
	}
	if portSpec == "" {
		portSpec = strings.Join(extractStrings(payload, "ports", []string{}), ",")
	}
	portNums, err := service.ParsePortSpec(portSpec)
	if err != nil {
		return "", fmt.Errorf("invalid port specification: %w", err)
	}
	probeMethodNames := extractStrings(payload, "probe_methods", nil)
	if vErr := service.ValidatePortScanMethods(probeMethodNames); vErr != nil {
		return "", fmt.Errorf("invalid port probe methods: %w", vErr)
	}
	probeMethods := make([]service.PortScanMethod, len(probeMethodNames))
	for i := range probeMethodNames {
		probeMethods[i] = service.PortScanMethod(probeMethodNames[i])
	}

	resp, err := r.monitorSvc.ScanURLPortsWithOptions(ctx, urls, portNums, service.PortScanOptions{
		TargetConcurrency: concurrency,
		PortConcurrency:   extractInt(payload, "port_concurrency", 256),
		ConnectTimeout:    time.Duration(extractInt(payload, "connect_timeout_ms", 800)) * time.Millisecond,
		ScanTimeout:       time.Duration(extractInt(payload, "scan_timeout_seconds", 0)) * time.Second,
		AuthorizedTargets: extractStrings(payload, "authorized_targets", nil),
		ProbeMethods:      probeMethods,
		JitterMin:         time.Duration(extractInt(payload, "jitter_min_ms", 0)) * time.Millisecond,
		JitterMax:         time.Duration(extractInt(payload, "jitter_max_ms", 0)) * time.Millisecond,
	})
	if err != nil {
		return "", fmt.Errorf("port scan failed: %w", err)
	}
	if r.alerts != nil {
		for _, item := range resp.Results {
			if item.Status == "resolve_failed" || item.Status == "scan_failed" || item.Status == "not_authorized" {
				r.alerts.SendWarning(alerting.AlertTypeReachability, "端口巡检失败", item.Reason, item, "scheduler", item.Input)
			}
		}
	}

	var b strings.Builder
	portDescription := fmt.Sprintf("端口 %v", resp.Ports)
	if resp.PortCount > 1024 {
		portDescription = fmt.Sprintf("全端口 1-65535（%d 个）", resp.PortCount)
	}
	fmt.Fprintf(&b, "端口扫描完成：%d 个目标，%d 个唯一 IP，%s，连接 %d/%d，耗时 %.1f 秒\n\n",
		resp.Summary.Total, resp.UniqueIPCount, portDescription, resp.AttemptedConnections, resp.PlannedConnections, float64(resp.DurationMS)/1000)
	for _, r := range resp.Results {
		switch r.Status {
		case "scanned":
			var portDetails []string
			for ip, ports := range r.OpenPorts {
				if len(ports) > 0 {
					portDetails = append(portDetails, fmt.Sprintf("%s: %v", ip, ports))
				}
			}
			sort.Strings(portDetails)
			if len(portDetails) > 0 {
				fmt.Fprintf(&b, "✅ %s — 开放端口 %s\n", r.Input, strings.Join(portDetails, "; "))
			} else {
				fmt.Fprintf(&b, "✅ %s — 无开放端口\n", r.Input)
			}
		case "resolve_failed":
			fmt.Fprintf(&b, "❌ %s — DNS 解析失败", r.Input)
			if r.Reason != "" {
				fmt.Fprintf(&b, " (%s)", r.Reason)
			}
			b.WriteString("\n")
		case "cdn_excluded":
			fmt.Fprintf(&b, "⚠️ %s — CDN 已排除\n", r.Input)
		case "not_authorized":
			fmt.Fprintf(&b, "⛔ %s — 超出授权 IP/CIDR 范围 (%s)\n", r.Input, r.Reason)
		default:
			fmt.Fprintf(&b, "❓ %s — %s", r.Input, r.Status)
			if r.Reason != "" {
				fmt.Fprintf(&b, " (%s)", r.Reason)
			}
			b.WriteString("\n")
		}
	}
	return sanitizeUTF8(b.String()), nil
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

func (r *ScreenshotCleanupRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
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
	skippedCount := 0
	for _, batch := range batches {
		batchTime := time.Unix(batch.UpdatedAt, 0)
		if batchTime.Before(cutoff) {
			if delErr := r.screenshotSvc.DeleteBatch(batch.Name); delErr != nil {
				continue
			}
			deletedCount++
		} else {
			skippedCount++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "截图清理完成（保留 %d 天）\n\n", maxAgeDays)
	fmt.Fprintf(&b, "🗑️ 已删除: %d 个批次\n", deletedCount)
	fmt.Fprintf(&b, "📁 保留: %d 个批次\n", skippedCount)
	return sanitizeUTF8(b.String()), nil
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

func (r *TamperCleanupRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.tamperSvc == nil {
		return "", fmt.Errorf("tamper service not available")
	}

	records, err := r.tamperSvc.ListAllCheckRecords()
	if err != nil {
		return "", fmt.Errorf("list check records failed: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -r.maxAgeDays).Unix()
	deletedCount := 0
	skippedCount := 0

	for url, urlRecords := range records {
		onlyExpired := true
		hasExpired := false
		zeroTimestampFound := false

		for _, record := range urlRecords {
			if record == nil {
				continue
			}
			if record.Timestamp == 0 {
				zeroTimestampFound = true
				continue
			}
			if record.Timestamp >= cutoff {
				onlyExpired = false
				break
			}
			hasExpired = true
		}

		if !onlyExpired || !hasExpired {
			skippedCount += len(urlRecords)
			continue
		}

		if zeroTimestampFound {
			logger.Warnf("tamper check record for %q has zero timestamp(s), skipping deletion to prevent data loss", url)
			skippedCount += len(urlRecords)
			continue
		}

		if delErr := r.tamperSvc.DeleteCheckRecords(url); delErr != nil {
			logger.Warnf("failed to delete expired tamper records for %q: %v", url, delErr)
		} else {
			deletedCount += len(urlRecords)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "篡改记录清理完成（保留 %d 天）\n\n", r.maxAgeDays)
	fmt.Fprintf(&b, "🗑️ 已删除: %d 条过期记录\n", deletedCount)
	fmt.Fprintf(&b, "📁 保留: %d 条有效记录\n", skippedCount)
	return sanitizeUTF8(b.String()), nil
}

// --- QuotaMonitorRunner (ST-13) ---

// QuotaMonitorRunner executes scheduled quota monitoring.
type QuotaMonitorRunner struct {
	orchestrator *adapter.EngineOrchestrator
	lowThreshold int
}

// NewQuotaMonitorRunner creates a QuotaMonitorRunner.
func NewQuotaMonitorRunner(orchestrator *adapter.EngineOrchestrator, lowThreshold int) *QuotaMonitorRunner {
	if lowThreshold <= 0 {
		lowThreshold = 10
	}
	return &QuotaMonitorRunner{orchestrator: orchestrator, lowThreshold: lowThreshold}
}

func (r *QuotaMonitorRunner) Type() TaskType { return TaskQuotaMonitor }

func (r *QuotaMonitorRunner) Execute(ctx context.Context, payload *model.TaskPayload) (string, error) {
	if r.orchestrator == nil {
		return "", fmt.Errorf("orchestrator not available")
	}

	engines := r.orchestrator.ListAdapters()
	if len(engines) == 0 {
		return "no engine adapters registered", nil
	}

	lowThreshold := extractInt(payload, "low_threshold", r.lowThreshold)
	var b strings.Builder
	fmt.Fprintf(&b, "引擎配额监控完成：%d 个引擎\n\n", len(engines))
	lowQuotaEngines := 0

	for _, engine := range engines {
		adapter, ok := r.orchestrator.GetAdapter(engine)
		if !ok {
			continue
		}
		quota, err := adapter.GetQuota()
		if err != nil {
			fmt.Fprintf(&b, "❌ %s: 查询失败 — %v\n", engine, err)
			continue
		}
		if quota != nil && quota.Remaining < lowThreshold {
			fmt.Fprintf(&b, "⚠️ %s: 配额不足 (剩余 %d/%d)\n", engine, quota.Remaining, quota.Total)
			lowQuotaEngines++
		} else if quota != nil {
			fmt.Fprintf(&b, "✅ %s: 配额充足 (剩余 %d/%d)\n", engine, quota.Remaining, quota.Total)
		} else {
			fmt.Fprintf(&b, "✅ %s: 配额信息不可用\n", engine)
		}
	}

	result := sanitizeUTF8(b.String())
	if lowQuotaEngines > 0 {
		return result, fmt.Errorf("%d engine(s) with low quota (below %d)", lowQuotaEngines, lowThreshold)
	}
	return result, nil
}
