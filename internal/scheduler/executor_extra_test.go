package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/alerting"
	"github.com/unimap-icp-hunter/project/internal/distributed"
	"github.com/unimap-icp-hunter/project/internal/screenshot"
	"github.com/unimap-icp-hunter/project/internal/service"
)

// ===== Bridge client mock for scheduler tests =====

type mockBridgeSchedulerClient struct {
	awaitResult screenshot.BridgeResult
	awaitErr    error
}

func (m *mockBridgeSchedulerClient) SubmitTask(ctx context.Context, task screenshot.BridgeTask) error {
	return nil
}
func (m *mockBridgeSchedulerClient) AwaitResult(ctx context.Context, requestID string) (screenshot.BridgeResult, error) {
	if m.awaitErr != nil {
		return screenshot.BridgeResult{}, m.awaitErr
	}
	return m.awaitResult, nil
}

// ===== QueryRunner Execute tests =====

func TestQueryRunner_Execute_NilService(t *testing.T) {
	r := NewQueryRunner(nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{"query": "test"})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error should mention 'not available': %v", err)
	}
}

func TestQueryRunner_Execute_MissingQuery(t *testing.T) {
	r := NewQueryRunner(service.NewQueryAppService(nil, nil))
	_, err := r.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error should mention 'missing': %v", err)
	}
}

// ===== SearchScreenshotRunner Execute tests =====

func TestSearchScreenshotRunner_Execute_NilService(t *testing.T) {
	r := NewSearchScreenshotRunner(nil, nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{"engine": "fofa", "query": "test"})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestSearchScreenshotRunner_Execute_MissingParams(t *testing.T) {
	svc := service.NewScreenshotAppService("./screenshots")
	r := NewSearchScreenshotRunner(svc, nil)

	_, err := r.Execute(context.Background(), map[string]interface{}{"query": "test"})
	if err == nil {
		t.Fatal("expected error for missing engine")
	}

	_, err = r.Execute(context.Background(), map[string]interface{}{"engine": "fofa"})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

// ===== BatchScreenshotRunner Execute tests =====

func TestBatchScreenshotRunner_Execute_NilService(t *testing.T) {
	r := NewBatchScreenshotRunner(nil, nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{"urls": []string{"http://example.com"}})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestBatchScreenshotRunner_Execute_MissingURLs(t *testing.T) {
	svc := service.NewScreenshotAppService("./screenshots")
	r := NewBatchScreenshotRunner(svc, nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing urls")
	}
}

// ===== TamperCheckRunner Execute tests =====

func TestTamperCheckRunner_Execute_NilService(t *testing.T) {
	r := NewTamperCheckRunner(nil, nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{"urls": []string{"http://example.com"}})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestTamperCheckRunner_Execute_MissingURLs(t *testing.T) {
	svc := service.NewTamperAppService("", nil)
	r := NewTamperCheckRunner(svc, nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing urls")
	}
}

// ===== URLReachabilityRunner Execute tests =====

func TestURLReachabilityRunner_Execute_NilService(t *testing.T) {
	r := NewURLReachabilityRunner(nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{"urls": []string{"http://example.com"}})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestURLReachabilityRunner_Execute_MissingURLs(t *testing.T) {
	r := NewURLReachabilityRunner(service.NewMonitorAppService(nil))
	_, err := r.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing urls")
	}
}

// ===== CookieVerifyRunner Execute tests =====

func TestCookieVerifyRunner_Execute_NilMgr(t *testing.T) {
	r := NewCookieVerifyRunner(nil, nil)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestCookieVerifyRunner_Execute_DefaultEngines(t *testing.T) {
	mgr := &screenshot.Manager{}
	r := NewCookieVerifyRunner(nil, mgr)
	result, err := r.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "fofa") {
		t.Errorf("result should mention fofa: %s", result)
	}
	if !strings.Contains(result, "no_cookies") {
		t.Errorf("result should mention no_cookies: %s", result)
	}
}

func TestCookieVerifyRunner_Execute_SpecificEngines(t *testing.T) {
	mgr := &screenshot.Manager{}
	r := NewCookieVerifyRunner(nil, mgr)
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"engines": []string{"fofa", "custom"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "fofa") {
		t.Errorf("result should mention fofa: %s", result)
	}
	if !strings.Contains(result, "custom") {
		t.Errorf("result should mention custom: %s", result)
	}
}

// ===== LoginStatusCheckRunner Execute tests =====

func TestLoginStatusCheckRunner_Execute_NilMgr(t *testing.T) {
	r := NewLoginStatusCheckRunner(nil)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

// ===== DistributedSubmitRunner Execute tests =====

func TestDistributedSubmitRunner_Execute_NilQueue(t *testing.T) {
	r := NewDistributedSubmitRunner(nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{"task_type": "scan"})
	if err == nil {
		t.Fatal("expected error for nil queue")
	}
}

func TestDistributedSubmitRunner_Execute_MissingTaskType(t *testing.T) {
	q := distributed.NewTaskQueue()
	r := NewDistributedSubmitRunner(q)
	_, err := r.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing task_type")
	}
}

func TestDistributedSubmitRunner_Execute_Success(t *testing.T) {
	q := distributed.NewTaskQueue()
	r := NewDistributedSubmitRunner(q)
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"task_type":       "port_scan",
		"task_payload":    map[string]interface{}{"target": "example.com"},
		"priority":        5,
		"timeout_seconds": 60,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "enqueued") {
		t.Errorf("result should mention 'enqueued': %s", result)
	}
	if !strings.Contains(result, "port_scan") {
		t.Errorf("result should mention task type: %s", result)
	}
}

// ===== ExportRunner Execute tests =====

func TestExportRunner_Execute_NilDeps(t *testing.T) {
	r := NewExportRunner(nil, nil, "/tmp")
	_, err := r.Execute(context.Background(), map[string]interface{}{"query": "test"})
	if err == nil {
		t.Fatal("expected error for nil deps")
	}
}

func TestExportRunner_Execute_MissingQuery(t *testing.T) {
	r := NewExportRunner(service.NewQueryAppService(nil, nil), adapter.NewEngineOrchestrator(), "/tmp")
	_, err := r.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

// ===== PortScanRunner Execute tests =====

func TestPortScanRunner_Execute_NilService(t *testing.T) {
	r := NewPortScanRunner(nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{"urls": []string{"http://example.com"}})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestPortScanRunner_Execute_MissingURLs(t *testing.T) {
	r := NewPortScanRunner(service.NewMonitorAppService(nil))
	_, err := r.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing urls")
	}
}

// ===== ScreenshotCleanupRunner Execute tests =====

func TestScreenshotCleanupRunner_Execute_NilService(t *testing.T) {
	r := NewScreenshotCleanupRunner(nil, 30)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

// ===== TamperCleanupRunner Execute tests =====

func TestTamperCleanupRunner_Execute_NilService(t *testing.T) {
	r := NewTamperCleanupRunner(nil, 90)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

// ===== QuotaMonitorRunner Execute tests =====

func TestQuotaMonitorRunner_Execute_NilOrchestrator(t *testing.T) {
	r := NewQuotaMonitorRunner(nil, 10)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil orchestrator")
	}
}

func TestQuotaMonitorRunner_Execute_NoAdapters(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	r := NewQuotaMonitorRunner(orch, 10)
	result, err := r.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "no engine adapters") {
		t.Errorf("result should mention no adapters: %s", result)
	}
}

// ===== AlertSummaryRunner Execute tests =====

func TestAlertSummaryRunner_Execute_NilManager(t *testing.T) {
	r := NewAlertSummaryRunner(nil)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestAlertSummaryRunner_Execute_EmptyRecords(t *testing.T) {
	m := alerting.NewManager()
	r := NewAlertSummaryRunner(m)
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"max_age_days": 7,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "total=0") {
		t.Errorf("result should show total=0: %s", result)
	}
}

func TestAlertSummaryRunner_Execute_WithRecords(t *testing.T) {
	m := alerting.NewManager()
	m.SendInfo(alerting.AlertTypeTamper, "t1", "tamper detected", nil, "s", "u1")
	m.SendWarning(alerting.AlertTypeSystem, "t2", "system alert", nil, "s", "u2")

	r := NewAlertSummaryRunner(m)
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"max_age_days": 7,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "total=2") {
		t.Errorf("result should show total=2: %s", result)
	}
}

// ===== BaselineRefreshRunner Execute tests =====

func TestBaselineRefreshRunner_Execute_NilService(t *testing.T) {
	r := NewBaselineRefreshRunner(nil)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

// ===== URLImportRunner Execute tests =====

func TestURLImportRunner_Execute_Unconfigured(t *testing.T) {
	r := NewURLImportRunner("")
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for unconfigured import dir")
	}
}

func TestURLImportRunner_Execute_NoFiles(t *testing.T) {
	dir := t.TempDir()
	r := NewURLImportRunner(dir)
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"file_pattern": "*.txt",
		"max_lines":    1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "no files") {
		t.Errorf("result should mention no files: %s", result)
	}
}

func TestURLImportRunner_Execute_WithFile(t *testing.T) {
	dir := t.TempDir()
	content := "http://example.com\nhttp://example.org\n# this is a comment\n\nhttp://example.net"
	err := os.WriteFile(filepath.Join(dir, "urls.txt"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	r := NewURLImportRunner(dir)
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"file_pattern": "*.txt",
		"max_lines":    10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "3 URL") {
		t.Errorf("result should mention 3 URLs: %s", result)
	}
}

func TestURLImportRunner_Execute_MaxLines(t *testing.T) {
	dir := t.TempDir()
	lines := ""
	for i := 0; i < 100; i++ {
		lines += "http://example.com/page\n"
	}
	err := os.WriteFile(filepath.Join(dir, "urls.txt"), []byte(lines), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	r := NewURLImportRunner(dir)
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"file_pattern": "*.txt",
		"max_lines":    5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "5 URL") {
		t.Errorf("result should mention 5 URLs (limited by max_lines): %s", result)
	}
}

func TestReadURLsFromFile_EdgeCases(t *testing.T) {
	dir := t.TempDir()

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(dir, "empty.txt")
		os.WriteFile(path, []byte(""), 0644)
		urls, err := readURLsFromFile(path, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(urls) != 0 {
			t.Errorf("expected 0 URLs, got %d", len(urls))
		}
	})

	t.Run("comments and blank lines", func(t *testing.T) {
		path := filepath.Join(dir, "comments.txt")
		os.WriteFile(path, []byte("# comment\n\nhttp://a.com\n\n# another comment\nhttp://b.com\n"), 0644)
		urls, err := readURLsFromFile(path, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(urls) != 2 {
			t.Errorf("expected 2 URLs, got %d", len(urls))
		}
	})

	t.Run("max lines", func(t *testing.T) {
		path := filepath.Join(dir, "many.txt")
		content := "http://a.com\nhttp://b.com\nhttp://c.com\nhttp://d.com\n"
		os.WriteFile(path, []byte(content), 0644)
		urls, err := readURLsFromFile(path, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(urls) != 2 {
			t.Errorf("expected 2 URLs, got %d", len(urls))
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := readURLsFromFile(filepath.Join(dir, "does_not_exist.txt"), 100)
		if err == nil {
			t.Fatal("expected error for non-existent file")
		}
	})
}

// ===== PluginHealthRunner Execute tests =====

func TestPluginHealthRunner_Execute_NilService(t *testing.T) {
	r := NewPluginHealthRunner(nil)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

// ===== BridgeTokenRotateRunner Execute tests =====

func TestBridgeTokenRotateRunner_Execute_NilService(t *testing.T) {
	r := NewBridgeTokenRotateRunner(nil)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestBridgeTokenRotateRunner_Execute_NotStarted(t *testing.T) {
	client := &mockBridgeSchedulerClient{}
	svc := screenshot.NewBridgeService(client, 5, 5*time.Second)
	r := NewBridgeTokenRotateRunner(svc)
	_, err := r.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for not-started bridge")
	}
	if !strings.Contains(err.Error(), "not started") {
		t.Errorf("error should mention 'not started': %v", err)
	}
}

func TestBridgeTokenRotateRunner_Execute_Started(t *testing.T) {
	client := &mockBridgeSchedulerClient{}
	svc := screenshot.NewBridgeService(client, 5, 5*time.Second)
	svc.Start(context.Background())
	defer svc.Stop()

	r := NewBridgeTokenRotateRunner(svc)
	result, err := r.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "started=true") {
		t.Errorf("result should mention started=true: %s", result)
	}
}

// ===== AlertSilenceRunner Execute tests =====

func TestAlertSilenceRunner_Execute_NilManager(t *testing.T) {
	r := NewAlertSilenceRunner(nil)
	_, err := r.Execute(context.Background(), map[string]interface{}{"alert_type": "tamper"})
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestAlertSilenceRunner_Execute_WithAlertType(t *testing.T) {
	m := alerting.NewManager()
	m.SendInfo(alerting.AlertTypeTamper, "t1", "msg", nil, "s", "u1")

	r := NewAlertSilenceRunner(m)
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"alert_type":       "tamper",
		"duration_minutes": 30,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "silenced") {
		t.Errorf("result should mention 'silenced': %s", result)
	}
	if !strings.Contains(result, "30") {
		t.Errorf("result should mention duration: %s", result)
	}
}

func TestAlertSilenceRunner_Execute_CleanupOldRecords(t *testing.T) {
	m := alerting.NewManager()
	r := NewAlertSilenceRunner(m)
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"max_age_days": 30,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "cleaned up") {
		t.Errorf("result should mention 'cleaned up': %s", result)
	}
}

// ===== CacheWarmupRunner Execute tests =====

func TestCacheWarmupRunner_Execute_NoURLs(t *testing.T) {
	r := NewCacheWarmupRunner()
	result, err := r.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "no warmup URLs") {
		t.Errorf("result should mention no URLs: %s", result)
	}
}

func TestCacheWarmupRunner_Execute_WithInvalidURL(t *testing.T) {
	r := NewCacheWarmupRunner()
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"warmup_urls": []string{"://invalid"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "0/1") {
		t.Errorf("result should show 0/1: %s", result)
	}
}

func TestCacheWarmupRunner_Execute_WithUnreachableURL(t *testing.T) {
	r := NewCacheWarmupRunner()
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"warmup_urls": []string{"http://localhost:65535/unreachable"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "0/1") {
		t.Errorf("result should show 0/1: %s", result)
	}
}

// ===== extractStrings edge cases =====

func TestExtractStrings_InterfaceSliceWithNoStrings(t *testing.T) {
	payload := map[string]interface{}{"items": []interface{}{1, 2, 3}}
	got := extractStrings(payload, "items", []string{"default"})
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// ===== distributedIDCounter monotonicity =====

func TestDistributedTaskIDMonotonic(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateDistributedTaskID()
		if ids[id] {
			t.Errorf("duplicate ID: %s", id)
		}
		ids[id] = true
	}
}
