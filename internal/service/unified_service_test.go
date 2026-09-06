package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/unimap/project/internal/adapter"
	"github.com/unimap/project/internal/config"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/utils"
)

func TestNewUnifiedServiceWithConfig_Defaults(t *testing.T) {
	svc := NewUnifiedServiceWithConfig(nil)

	if svc == nil {
		t.Fatalf("expected service instance")
	}
	if svc.cacheTTL != 30*time.Minute {
		t.Fatalf("expected default cacheTTL=30m, got %s", svc.cacheTTL)
	}
	if svc.cacheMaxSize != 1000 {
		t.Fatalf("expected default cacheMaxSize=1000, got %d", svc.cacheMaxSize)
	}
	if svc.cacheCleanup != 5*time.Minute {
		t.Fatalf("expected default cacheCleanup=5m, got %s", svc.cacheCleanup)
	}
	if svc.orchestrator == nil {
		t.Fatalf("expected orchestrator instance")
	}
	if svc.orchestrator.GetConcurrency() != adapter.DefaultConcurrency {
		t.Fatalf("expected default orchestrator concurrency=%d, got %d", adapter.DefaultConcurrency, svc.orchestrator.GetConcurrency())
	}

	stats := svc.cache.GetStats()
	if stats.MaxSize != 1000 {
		t.Fatalf("expected cache max size 1000, got %d", stats.MaxSize)
	}
}

func TestHandleCachedQueryResultPreservesResponseMetadata(t *testing.T) {
	svc := NewUnifiedServiceWithConfig(nil)
	defer svc.Shutdown()
	req := QueryRequest{Query: `port="443"`, Engines: []string{"fofa", "hunter"}, PageSize: 10}
	key := svc.buildQueryCacheKey(req)
	assets := []model.UnifiedAsset{{IP: "192.0.2.10", Source: "fofa"}}
	metadataCache, ok := svc.cache.(utils.QuerySnapshotCache)
	if !ok {
		t.Fatal("query cache does not support metadata")
	}
	metadataCache.SetQuerySnapshot(key, assets, utils.QueryCacheMetadata{
		EngineStats: map[string]int{"fofa": 1, "hunter": 0},
		Errors:      []string{"hunter unavailable"},
	}, time.Minute)

	response, hit := svc.handleCachedQueryResult(context.Background(), req, key)
	if !hit || response.EngineStats["fofa"] != 1 || len(response.Errors) != 1 {
		t.Fatalf("cache hit changed response semantics: hit=%v response=%+v", hit, response)
	}
}

func TestNewUnifiedServiceWithConfig_Overrides(t *testing.T) {
	var cfg config.Config
	cfg.System.MaxConcurrent = 7
	cfg.System.CacheTTL = 120
	cfg.System.CacheMaxSize = 77
	cfg.System.CacheCleanupInterval = 11

	svc := NewUnifiedServiceWithConfig(&cfg)

	if svc.cacheTTL != 120*time.Second {
		t.Fatalf("expected cacheTTL=120s, got %s", svc.cacheTTL)
	}
	if svc.cacheMaxSize != 77 {
		t.Fatalf("expected cacheMaxSize=77, got %d", svc.cacheMaxSize)
	}
	if svc.cacheCleanup != 11*time.Second {
		t.Fatalf("expected cacheCleanup=11s, got %s", svc.cacheCleanup)
	}
	if svc.orchestrator.GetConcurrency() != 7 {
		t.Fatalf("expected orchestrator concurrency=7, got %d", svc.orchestrator.GetConcurrency())
	}

	stats := svc.cache.GetStats()
	if stats.MaxSize != 77 {
		t.Fatalf("expected cache max size 77, got %d", stats.MaxSize)
	}
}

// 表驱动测试：配置覆盖
func TestNewUnifiedServiceWithConfig_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.Config
		wantCacheTTL   time.Duration
		wantCacheSize  int
		wantCleanup    time.Duration
		wantConcurrent int
	}{
		{
			name:           "nil config uses defaults",
			config:         nil,
			wantCacheTTL:   30 * time.Minute,
			wantCacheSize:  1000,
			wantCleanup:    5 * time.Minute,
			wantConcurrent: adapter.DefaultConcurrency,
		},
		{
			name: "partial config",
			config: &config.Config{
				System: struct {
					MaxConcurrent        int    `yaml:"max_concurrent"`
					CacheTTL             int    `yaml:"cache_ttl"`
					CacheMaxSize         int    `yaml:"cache_max_size"`
					CacheCleanupInterval int    `yaml:"cache_cleanup_interval"`
					RetryAttempts        int    `yaml:"retry_attempts"`
					UserAgent            string `yaml:"user_agent"`
				}{
					MaxConcurrent: 5,
				},
			},
			wantCacheTTL:   30 * time.Minute,
			wantCacheSize:  1000,
			wantCleanup:    5 * time.Minute,
			wantConcurrent: 5,
		},
		{
			name: "full config",
			config: &config.Config{
				System: struct {
					MaxConcurrent        int    `yaml:"max_concurrent"`
					CacheTTL             int    `yaml:"cache_ttl"`
					CacheMaxSize         int    `yaml:"cache_max_size"`
					CacheCleanupInterval int    `yaml:"cache_cleanup_interval"`
					RetryAttempts        int    `yaml:"retry_attempts"`
					UserAgent            string `yaml:"user_agent"`
				}{
					MaxConcurrent:        8,
					CacheTTL:             300,
					CacheMaxSize:         500,
					CacheCleanupInterval: 60,
				},
			},
			wantCacheTTL:   300 * time.Second,
			wantCacheSize:  500,
			wantCleanup:    60 * time.Second,
			wantConcurrent: 8,
		},
		{
			name: "zero values use defaults",
			config: &config.Config{
				System: struct {
					MaxConcurrent        int    `yaml:"max_concurrent"`
					CacheTTL             int    `yaml:"cache_ttl"`
					CacheMaxSize         int    `yaml:"cache_max_size"`
					CacheCleanupInterval int    `yaml:"cache_cleanup_interval"`
					RetryAttempts        int    `yaml:"retry_attempts"`
					UserAgent            string `yaml:"user_agent"`
				}{
					MaxConcurrent:        0,
					CacheTTL:             0,
					CacheMaxSize:         0,
					CacheCleanupInterval: 0,
				},
			},
			wantCacheTTL:   30 * time.Minute,
			wantCacheSize:  1000,
			wantCleanup:    5 * time.Minute,
			wantConcurrent: adapter.DefaultConcurrency,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewUnifiedServiceWithConfig(tt.config)

			if svc.cacheTTL != tt.wantCacheTTL {
				t.Errorf("cacheTTL: got %v, want %v", svc.cacheTTL, tt.wantCacheTTL)
			}
			if svc.cacheMaxSize != tt.wantCacheSize {
				t.Errorf("cacheMaxSize: got %d, want %d", svc.cacheMaxSize, tt.wantCacheSize)
			}
			if svc.cacheCleanup != tt.wantCleanup {
				t.Errorf("cacheCleanup: got %v, want %v", svc.cacheCleanup, tt.wantCleanup)
			}
			if svc.orchestrator.GetConcurrency() != tt.wantConcurrent {
				t.Errorf("concurrency: got %d, want %d", svc.orchestrator.GetConcurrency(), tt.wantConcurrent)
			}
		})
	}
}

// 测试 Query 验证
func TestQueryValidation(t *testing.T) {
	svc := NewUnifiedService()

	tests := []struct {
		name        string
		req         QueryRequest
		wantErr     bool
		errContains string
	}{
		{"empty query", QueryRequest{Query: "", Engines: []string{"fofa"}}, true, "query cannot be empty"},
		{"empty engines", QueryRequest{Query: "test", Engines: []string{}}, true, "at least one engine"},
		{"nil engines", QueryRequest{Query: "test", Engines: nil}, true, "at least one engine"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Query(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Query() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			}
		})
	}
}

// 测试 Export 验证
func TestExportValidation(t *testing.T) {
	svc := NewUnifiedService()

	tests := []struct {
		name    string
		req     ExportRequest
		wantErr bool
	}{
		{"empty assets", ExportRequest{Assets: nil, Format: "json", OutputPath: "/tmp/out.json"}, true},
		{"empty format", ExportRequest{Assets: []model.UnifiedAsset{{}}, Format: "", OutputPath: "/tmp/out.json"}, true},
		{"empty output path", ExportRequest{Assets: []model.UnifiedAsset{{}}, Format: "json", OutputPath: ""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Export(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Export() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// 测试 RegisterAdapter
func TestRegisterAdapter(t *testing.T) {
	svc := NewUnifiedService()

	// 初始应该没有适配器
	if len(svc.orchestrator.ListAdapters()) != 0 {
		t.Error("expected no adapters initially")
	}

	// 注册适配器（使用 mock）
	mockAdapter := &testMockAdapter{name: "test-engine"}
	svc.RegisterAdapter(mockAdapter)

	// 应该有一个适配器
	if len(svc.orchestrator.ListAdapters()) != 1 {
		t.Error("expected one adapter after registration")
	}
}

// 测试 GetOrchestrator
func TestGetOrchestrator(t *testing.T) {
	svc := NewUnifiedService()

	orch := svc.GetOrchestrator()
	if orch == nil {
		t.Error("expected orchestrator to be returned")
	}
}

// 测试 ListEngines
func TestListEngines(t *testing.T) {
	svc := NewUnifiedService()

	// 没有注册引擎时应该返回空列表
	engines := svc.ListEngines()
	if len(engines) != 0 {
		t.Error("expected empty list when no engines registered")
	}
}

// 测试 ListProcessors
func TestListProcessors(t *testing.T) {
	svc := NewUnifiedService()

	// 没有注册处理器时应该返回空列表
	processors := svc.ListProcessors()
	if len(processors) != 0 {
		t.Error("expected empty list when no processors registered")
	}
}

// 测试 HealthCheck
func TestHealthCheck(t *testing.T) {
	svc := NewUnifiedService()

	// 应该返回非 nil 的健康状态
	health := svc.HealthCheck()
	if health == nil {
		t.Error("expected health check result")
	}
}

// 测试 Shutdown
func TestShutdown(t *testing.T) {
	svc := NewUnifiedService()

	err := svc.Shutdown()
	if err != nil {
		t.Errorf("unexpected shutdown error: %v", err)
	}
}

// 测试 GetPluginManager
func TestGetPluginManager(t *testing.T) {
	svc := NewUnifiedService()

	pm := svc.GetPluginManager()
	if pm == nil {
		t.Error("expected plugin manager to be returned")
	}
}

// 基准测试
func BenchmarkNewUnifiedService(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewUnifiedService()
	}
}

func BenchmarkNewUnifiedServiceWithConfig(b *testing.B) {
	cfg := &config.Config{
		System: struct {
			MaxConcurrent        int    `yaml:"max_concurrent"`
			CacheTTL             int    `yaml:"cache_ttl"`
			CacheMaxSize         int    `yaml:"cache_max_size"`
			CacheCleanupInterval int    `yaml:"cache_cleanup_interval"`
			RetryAttempts        int    `yaml:"retry_attempts"`
			UserAgent            string `yaml:"user_agent"`
		}{
			MaxConcurrent:        5,
			CacheTTL:             60,
			CacheMaxSize:         500,
			CacheCleanupInterval: 30,
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewUnifiedServiceWithConfig(cfg)
	}
}

// mock adapter for testing
type testMockAdapter struct {
	name    string
	results []model.UnifiedAsset
}

func (m *testMockAdapter) Name() string                                { return m.name }
func (m *testMockAdapter) Translate(ast *model.UQLAST) (string, error) { return "translated", nil }
func (m *testMockAdapter) Search(ctx context.Context, query string, page, pageSize int) (*model.EngineResult, error) {
	return &model.EngineResult{EngineName: m.name, Total: len(m.results)}, nil
}
func (m *testMockAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
	return m.results, nil
}
func (m *testMockAdapter) GetQuota() (*model.QuotaInfo, error) { return &model.QuotaInfo{}, nil }
func (m *testMockAdapter) IsWebOnly() bool                     { return false }

func TestExecuteAndNormalizeQuery_DeterministicAssetOrder(t *testing.T) {
	svc := NewUnifiedService()
	defer svc.Shutdown()
	svc.RegisterAdapter(&testMockAdapter{
		name: "fofa",
		results: []model.UnifiedAsset{
			{IP: "10.0.0.3", Port: 443, Source: "fofa"},
			{IP: "10.0.0.1", Port: 80, Source: "fofa"},
			{IP: "10.0.0.2", Port: 22, Source: "fofa"},
			{IP: "10.0.0.1", Port: 443, Source: "fofa"},
		},
	})

	assets, _, errs, err := svc.executeAndNormalizeQuery(context.Background(), QueryRequest{
		Query:    `port="80"`,
		Engines:  []string{"fofa"},
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("executeAndNormalizeQuery: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected query errors: %v", errs)
	}
	if len(assets) != 4 {
		t.Fatalf("expected 4 assets, got %d", len(assets))
	}
	want := []struct {
		ip   string
		port int
	}{
		{"10.0.0.1", 80},
		{"10.0.0.1", 443},
		{"10.0.0.2", 22},
		{"10.0.0.3", 443},
	}
	for i, w := range want {
		if assets[i].IP != w.ip || assets[i].Port != w.port {
			t.Errorf("assets[%d] = %s:%d, want %s:%d (full=%v)", i, assets[i].IP, assets[i].Port, w.ip, w.port, assets)
		}
	}
}
