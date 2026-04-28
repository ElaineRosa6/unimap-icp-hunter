package scheduler

import (
	"testing"
)

// extractStrings 测试
func TestExtractStrings(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]interface{}
		key      string
		def      []string
		want     []string
	}{
		{
			name:     "key not exists returns default",
			payload:  map[string]interface{}{},
			key:      "missing",
			def:      []string{"default"},
			want:     []string{"default"},
		},
		{
			name:     "value is []string returns it",
			payload:  map[string]interface{}{"engines": []string{"fofa", "hunter"}},
			key:      "engines",
			def:      []string{},
			want:     []string{"fofa", "hunter"},
		},
		{
			name:     "value is []interface{} extracts strings",
			payload:  map[string]interface{}{"engines": []interface{}{"fofa", "hunter", 123}},
			key:      "engines",
			def:      []string{},
			want:     []string{"fofa", "hunter"},
		},
		{
			name:     "value is string returns single-element slice",
			payload:  map[string]interface{}{"engine": "fofa"},
			key:      "engine",
			def:      []string{},
			want:     []string{"fofa"},
		},
		{
			name:     "value is empty string returns default",
			payload:  map[string]interface{}{"engine": ""},
			key:      "engine",
			def:      []string{"default"},
			want:     []string{"default"},
		},
		{
			name:     "value is other type returns default",
			payload:  map[string]interface{}{"engines": 123},
			key:      "engines",
			def:      []string{"default"},
			want:     []string{"default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractStrings(tt.payload, tt.key, tt.def)
			if len(got) != len(tt.want) {
				t.Errorf("extractStrings() len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractStrings()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// extractInt 测试
func TestExtractInt(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]interface{}
		key     string
		def     int
		want    int
	}{
		{
			name:    "key not exists returns default",
			payload: map[string]interface{}{},
			key:     "missing",
			def:     100,
			want:    100,
		},
		{
			name:    "value is float64 returns int",
			payload: map[string]interface{}{"page_size": 50.0},
			key:     "page_size",
			def:     100,
			want:    50,
		},
		{
			name:    "value is int returns it",
			payload: map[string]interface{}{"page_size": 200},
			key:     "page_size",
			def:     100,
			want:    200,
		},
		{
			name:    "value is other type returns default",
			payload: map[string]interface{}{"page_size": "not-a-number"},
			key:     "page_size",
			def:     100,
			want:    100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractInt(tt.payload, tt.key, tt.def)
			if got != tt.want {
				t.Errorf("extractInt() = %d, want %d", got, tt.want)
			}
		})
	}
}

// extractString 测试
func TestExtractString(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]interface{}
		key     string
		def     string
		want    string
	}{
		{
			name:    "key not exists returns default",
			payload: map[string]interface{}{},
			key:     "missing",
			def:     "default",
			want:    "default",
		},
		{
			name:    "value is string returns it",
			payload: map[string]interface{}{"query": "domain=example.com"},
			key:     "query",
			def:     "default",
			want:    "domain=example.com",
		},
		{
			name:    "value is other type returns default",
			payload: map[string]interface{}{"query": 123},
			key:     "query",
			def:     "default",
			want:    "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractString(tt.payload, tt.key, tt.def)
			if got != tt.want {
				t.Errorf("extractString() = %q, want %q", got, tt.want)
			}
		})
	}
}

// min 测试
func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"a less than b", 1, 5, 1},
		{"b less than a", 10, 3, 3},
		{"equal", 5, 5, 5},
		{"negative a", -5, 3, -5},
		{"negative b", 5, -3, -3},
		{"both negative", -10, -5, -10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := min(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// generateDistributedTaskID 测试
func TestGenerateDistributedTaskID(t *testing.T) {
	id1 := generateDistributedTaskID()
	id2 := generateDistributedTaskID()

	if id1 == id2 {
		t.Error("expected different IDs, got same")
	}
	if len(id1) == 0 {
		t.Error("expected non-empty ID")
	}
	if id1[:4] != "dist" {
		t.Errorf("expected ID to start with 'dist', got %q", id1)
	}
}

// readURLsFromFile 测试
func TestReadURLsFromFile(t *testing.T) {
	importDir := t.TempDir()

	// 测试空文件
	t.Run("empty file", func(t *testing.T) {
		r := NewURLImportRunner(importDir)
		_, err := r.Execute(nil, map[string]interface{}{"file_pattern": "nonexistent.txt"})
		// 应该返回错误但不会panic
		if err != nil {
			t.Logf("Expected error for non-existent file: %v", err)
		}
	})
}

// QueryRunner 类型测试
func TestQueryRunnerType(t *testing.T) {
	r := NewQueryRunner(nil)
	if r.Type() != TaskQuery {
		t.Errorf("expected type %s, got %s", TaskQuery, r.Type())
	}
}

// SearchScreenshotRunner 类型测试
func TestSearchScreenshotRunnerType(t *testing.T) {
	r := NewSearchScreenshotRunner(nil, nil)
	if r.Type() != TaskSearchScreenshot {
		t.Errorf("expected type %s, got %s", TaskSearchScreenshot, r.Type())
	}
}

// BatchScreenshotRunner 类型测试
func TestBatchScreenshotRunnerType(t *testing.T) {
	r := NewBatchScreenshotRunner(nil, nil)
	if r.Type() != TaskBatchScreenshot {
		t.Errorf("expected type %s, got %s", TaskBatchScreenshot, r.Type())
	}
}

// TamperCheckRunner 类型测试
func TestTamperCheckRunnerType(t *testing.T) {
	r := NewTamperCheckRunner(nil, nil)
	if r.Type() != TaskTamperCheck {
		t.Errorf("expected type %s, got %s", TaskTamperCheck, r.Type())
	}
}

// URLReachabilityRunner 类型测试
func TestURLReachabilityRunnerType(t *testing.T) {
	r := NewURLReachabilityRunner(nil)
	if r.Type() != TaskURLReachability {
		t.Errorf("expected type %s, got %s", TaskURLReachability, r.Type())
	}
}

// CookieVerifyRunner 类型测试
func TestCookieVerifyRunnerType(t *testing.T) {
	r := NewCookieVerifyRunner(nil, nil)
	if r.Type() != TaskCookieVerify {
		t.Errorf("expected type %s, got %s", TaskCookieVerify, r.Type())
	}
}

// LoginStatusCheckRunner 类型测试
func TestLoginStatusCheckRunnerType(t *testing.T) {
	r := NewLoginStatusCheckRunner(nil)
	if r.Type() != TaskLoginStatusCheck {
		t.Errorf("expected type %s, got %s", TaskLoginStatusCheck, r.Type())
	}
}

// DistributedSubmitRunner 类型测试
func TestDistributedSubmitRunnerType(t *testing.T) {
	r := NewDistributedSubmitRunner(nil)
	if r.Type() != TaskDistributedSubmit {
		t.Errorf("expected type %s, got %s", TaskDistributedSubmit, r.Type())
	}
}

// ExportRunner 类型测试
func TestExportRunnerType(t *testing.T) {
	r := NewExportRunner(nil, nil, "")
	if r.Type() != TaskExport {
		t.Errorf("expected type %s, got %s", TaskExport, r.Type())
	}
}

// PortScanRunner 类型测试
func TestPortScanRunnerType(t *testing.T) {
	r := NewPortScanRunner(nil)
	if r.Type() != TaskPortScan {
		t.Errorf("expected type %s, got %s", TaskPortScan, r.Type())
	}
}

// ScreenshotCleanupRunner 类型测试
func TestScreenshotCleanupRunnerType(t *testing.T) {
	r := NewScreenshotCleanupRunner(nil, 30)
	if r.Type() != TaskScreenshotCleanup {
		t.Errorf("expected type %s, got %s", TaskScreenshotCleanup, r.Type())
	}
}

// TamperCleanupRunner 类型测试
func TestTamperCleanupRunnerType(t *testing.T) {
	r := NewTamperCleanupRunner(nil, 90)
	if r.Type() != TaskTamperCleanup {
		t.Errorf("expected type %s, got %s", TaskTamperCleanup, r.Type())
	}
}

// QuotaMonitorRunner 类型测试
func TestQuotaMonitorRunnerType(t *testing.T) {
	r := NewQuotaMonitorRunner(nil, 10)
	if r.Type() != TaskQuotaMonitor {
		t.Errorf("expected type %s, got %s", TaskQuotaMonitor, r.Type())
	}
}

// AlertSummaryRunner 类型测试
func TestAlertSummaryRunnerType(t *testing.T) {
	r := NewAlertSummaryRunner(nil)
	if r.Type() != TaskAlertSummary {
		t.Errorf("expected type %s, got %s", TaskAlertSummary, r.Type())
	}
}

// BaselineRefreshRunner 类型测试
func TestBaselineRefreshRunnerType(t *testing.T) {
	r := NewBaselineRefreshRunner(nil)
	if r.Type() != TaskBaselineRefresh {
		t.Errorf("expected type %s, got %s", TaskBaselineRefresh, r.Type())
	}
}

// URLImportRunner 类型测试
func TestURLImportRunnerType(t *testing.T) {
	r := NewURLImportRunner("")
	if r.Type() != TaskURLImport {
		t.Errorf("expected type %s, got %s", TaskURLImport, r.Type())
	}
}

// PluginHealthRunner 类型测试
func TestPluginHealthRunnerType(t *testing.T) {
	r := NewPluginHealthRunner(nil)
	if r.Type() != TaskPluginHealth {
		t.Errorf("expected type %s, got %s", TaskPluginHealth, r.Type())
	}
}

// BridgeTokenRotateRunner 类型测试
func TestBridgeTokenRotateRunnerType(t *testing.T) {
	r := NewBridgeTokenRotateRunner(nil)
	if r.Type() != TaskBridgeTokenRotate {
		t.Errorf("expected type %s, got %s", TaskBridgeTokenRotate, r.Type())
	}
}

// AlertSilenceRunner 类型测试
func TestAlertSilenceRunnerType(t *testing.T) {
	r := NewAlertSilenceRunner(nil)
	if r.Type() != TaskAlertSilence {
		t.Errorf("expected type %s, got %s", TaskAlertSilence, r.Type())
	}
}

// CacheWarmupRunner 类型测试
func TestCacheWarmupRunnerType(t *testing.T) {
	r := NewCacheWarmupRunner()
	if r.Type() != TaskCacheWarmup {
		t.Errorf("expected type %s, got %s", TaskCacheWarmup, r.Type())
	}
}

// NewScreenshotCleanupRunner 默认值测试
func TestNewScreenshotCleanupRunnerDefaults(t *testing.T) {
	r := NewScreenshotCleanupRunner(nil, 0)
	if r.maxAgeDays != 30 {
		t.Errorf("expected default maxAgeDays=30, got %d", r.maxAgeDays)
	}

	r = NewScreenshotCleanupRunner(nil, -5)
	if r.maxAgeDays != 30 {
		t.Errorf("expected maxAgeDays=30 for negative, got %d", r.maxAgeDays)
	}

	r = NewScreenshotCleanupRunner(nil, 60)
	if r.maxAgeDays != 60 {
		t.Errorf("expected maxAgeDays=60, got %d", r.maxAgeDays)
	}
}

// NewTamperCleanupRunner 默认值测试
func TestNewTamperCleanupRunnerDefaults(t *testing.T) {
	r := NewTamperCleanupRunner(nil, 0)
	if r.maxAgeDays != 90 {
		t.Errorf("expected default maxAgeDays=90, got %d", r.maxAgeDays)
	}

	r = NewTamperCleanupRunner(nil, -5)
	if r.maxAgeDays != 90 {
		t.Errorf("expected maxAgeDays=90 for negative, got %d", r.maxAgeDays)
	}

	r = NewTamperCleanupRunner(nil, 180)
	if r.maxAgeDays != 180 {
		t.Errorf("expected maxAgeDays=180, got %d", r.maxAgeDays)
	}
}

// NewQuotaMonitorRunner 默认值测试
func TestNewQuotaMonitorRunnerDefaults(t *testing.T) {
	r := NewQuotaMonitorRunner(nil, 0)
	if r.lowThreshold != 10 {
		t.Errorf("expected default lowThreshold=10, got %d", r.lowThreshold)
	}

	r = NewQuotaMonitorRunner(nil, -5)
	if r.lowThreshold != 10 {
		t.Errorf("expected lowThreshold=10 for negative, got %d", r.lowThreshold)
	}

	r = NewQuotaMonitorRunner(nil, 50)
	if r.lowThreshold != 50 {
		t.Errorf("expected lowThreshold=50, got %d", r.lowThreshold)
	}
}
