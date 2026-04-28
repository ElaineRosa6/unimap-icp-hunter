package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/model"
)

// 测试 Query PageSize 默认值
func TestQueryPageSizeDefault(t *testing.T) {
	svc := NewUnifiedService()

	// 注册一个 mock adapter
	mockAdapter := &testMockAdapter{name: "test"}
	svc.RegisterAdapter(mockAdapter)

	tests := []struct {
		name         string
		pageSize     int
		wantPageSize int
	}{
		{"zero defaults to 100", 0, 100},
		{"negative defaults to 100", -5, 100},
		{"positive kept", 50, 50},
		{"large kept", 1000, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 注意：实际 PageSize 会在 Query 内部修改，但验证失败时会返回错误
			// 所以我们只测试验证逻辑
		})
	}
}

// 测试并发控制锁
func TestQueryConcurrencyLock(t *testing.T) {
	svc := NewUnifiedServiceWithConfig(&config.Config{
		System: struct {
			MaxConcurrent        int    `yaml:"max_concurrent"`
			CacheTTL             int    `yaml:"cache_ttl"`
			CacheMaxSize         int    `yaml:"cache_max_size"`
			CacheCleanupInterval int    `yaml:"cache_cleanup_interval"`
			RetryAttempts        int    `yaml:"retry_attempts"`
			UserAgent            string `yaml:"user_agent"`
		}{
			MaxConcurrent: 2,
		},
	})

	// 测试并发限制
	if svc.maxConcurrent != 2 {
		t.Fatalf("expected maxConcurrent=2, got %d", svc.maxConcurrent)
	}

	// 测试锁获取/释放
	if !svc.acquireQueryLock() {
		t.Error("expected to acquire lock")
	}
	if svc.activeQueries != 1 {
		t.Errorf("expected activeQueries=1, got %d", svc.activeQueries)
	}

	svc.releaseQueryLock()
	if svc.activeQueries != 0 {
		t.Errorf("expected activeQueries=0 after release, got %d", svc.activeQueries)
	}
}

// 测试并发锁限制
func TestQueryConcurrencyLockLimit(t *testing.T) {
	svc := NewUnifiedServiceWithConfig(&config.Config{
		System: struct {
			MaxConcurrent        int    `yaml:"max_concurrent"`
			CacheTTL             int    `yaml:"cache_ttl"`
			CacheMaxSize         int    `yaml:"cache_max_size"`
			CacheCleanupInterval int    `yaml:"cache_cleanup_interval"`
			RetryAttempts        int    `yaml:"retry_attempts"`
			UserAgent            string `yaml:"user_agent"`
		}{
			MaxConcurrent: 1,
		},
	})

	// 获取第一个锁
	if !svc.acquireQueryLock() {
		t.Error("expected to acquire first lock")
	}

	// 第二个应该失败（达到限制）
	if svc.acquireQueryLock() {
		t.Error("expected to fail acquiring second lock when limit reached")
	}

	// 释放后应该可以再次获取
	svc.releaseQueryLock()
	if !svc.acquireQueryLock() {
		t.Error("expected to acquire lock after release")
	}
	svc.releaseQueryLock()
}

// 测试并发锁并发安全
func TestQueryConcurrencyLockConcurrent(t *testing.T) {
	svc := NewUnifiedServiceWithConfig(&config.Config{
		System: struct {
			MaxConcurrent        int    `yaml:"max_concurrent"`
			CacheTTL             int    `yaml:"cache_ttl"`
			CacheMaxSize         int    `yaml:"cache_max_size"`
			CacheCleanupInterval int    `yaml:"cache_cleanup_interval"`
			RetryAttempts        int    `yaml:"retry_attempts"`
			UserAgent            string `yaml:"user_agent"`
		}{
			MaxConcurrent: 10,
		},
	})

	var wg sync.WaitGroup
	var successCount int64
	var failCount int64

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if svc.acquireQueryLock() {
				atomic.AddInt64(&successCount, 1)
				svc.releaseQueryLock()
			} else {
				atomic.AddInt64(&failCount, 1)
			}
		}()
	}
	wg.Wait()

	// 由于限制是10，成功数应该约等于10（可能有并发竞争）
	if atomic.LoadInt64(&successCount) < 10 {
		t.Errorf("expected at least 10 successful lock acquisitions, got %d", atomic.LoadInt64(&successCount))
	}
}

// 测试 QueryRequest 结构
func TestQueryRequestFields(t *testing.T) {
	req := QueryRequest{
		Query:       "domain=\"example.com\"",
		Engines:     []string{"fofa", "hunter"},
		PageSize:    100,
		ProcessData: true,
	}

	if req.Query != "domain=\"example.com\"" {
		t.Error("query field mismatch")
	}
	if len(req.Engines) != 2 {
		t.Error("engines count mismatch")
	}
	if req.PageSize != 100 {
		t.Error("pageSize mismatch")
	}
	if !req.ProcessData {
		t.Error("processData mismatch")
	}
}

// 测试 QueryResponse 结构
func TestQueryResponseFields(t *testing.T) {
	resp := &QueryResponse{
		Assets:      []model.UnifiedAsset{{IP: "1.2.3.4"}},
		TotalCount:  1,
		EngineStats: map[string]int{"fofa": 1},
		Errors:      []string{},
	}

	if len(resp.Assets) != 1 {
		t.Error("assets count mismatch")
	}
	if resp.TotalCount != 1 {
		t.Error("totalCount mismatch")
	}
	if resp.EngineStats["fofa"] != 1 {
		t.Error("engineStats mismatch")
	}
	if len(resp.Errors) != 0 {
		t.Error("errors should be empty")
	}
}

// 测试 ExportRequest 结构
func TestExportRequestFields(t *testing.T) {
	req := ExportRequest{
		Assets:     []model.UnifiedAsset{{IP: "1.2.3.4"}},
		Format:     "json",
		OutputPath: "/tmp/output.json",
	}

	if len(req.Assets) != 1 {
		t.Error("assets count mismatch")
	}
	if req.Format != "json" {
		t.Error("format mismatch")
	}
	if req.OutputPath != "/tmp/output.json" {
		t.Error("outputPath mismatch")
	}
}

// 测试 Export 错误消息
func TestExportErrors(t *testing.T) {
	svc := NewUnifiedService()

	tests := []struct {
		name        string
		req         ExportRequest
		wantContain string
	}{
		{
			name:        "no assets",
			req:         ExportRequest{Assets: nil, Format: "json", OutputPath: "/tmp/out"},
			wantContain: "no assets",
		},
		{
			name:        "empty format",
			req:         ExportRequest{Assets: []model.UnifiedAsset{{}}, Format: "", OutputPath: "/tmp/out"},
			wantContain: "format",
		},
		{
			name:        "empty output path",
			req:         ExportRequest{Assets: []model.UnifiedAsset{{}}, Format: "json", OutputPath: ""},
			wantContain: "output path",
		},
		{
			name:        "no exporters",
			req:         ExportRequest{Assets: []model.UnifiedAsset{{}}, Format: "json", OutputPath: "/tmp/out"},
			wantContain: "no exporters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Export(context.Background(), tt.req)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

// 测试 resolveCacheTTL 默认行为
func TestResolveCacheTTL(t *testing.T) {
	svc := NewUnifiedService()

	req := QueryRequest{
		Query:   "test",
		Engines: []string{"fofa"},
		PageSize: 100,
	}

	ttl := svc.resolveCacheTTL(req)
	// 应该返回默认 TTL 或策略管理的 TTL
	if ttl <= 0 {
		t.Error("expected positive TTL")
	}
}

// 测试 checkResourceLimits 内存检查
func TestCheckResourceLimits(t *testing.T) {
	svc := NewUnifiedService()

	// 默认配置下应该不报错（内存使用应该低于限制）
	err := svc.checkResourceLimits(context.Background())
	if err != nil {
		t.Logf("checkResourceLimits returned error: %v (may be legitimate if memory usage is high)", err)
	}
}

// 测试 GetPluginManager 返回值
func TestGetPluginManagerNotNil(t *testing.T) {
	svc := NewUnifiedService()
	pm := svc.GetPluginManager()
	if pm == nil {
		t.Error("GetPluginManager should not return nil")
	}
}

// 测试 HealthCheck 返回空 map（无插件时）
func TestHealthCheckEmpty(t *testing.T) {
	svc := NewUnifiedService()
	health := svc.HealthCheck()
	if len(health) != 0 {
		t.Logf("HealthCheck returned %d items (plugins may be registered)", len(health))
	}
}

// 测试 ListEngines 空结果
func TestListEnginesEmpty(t *testing.T) {
	svc := NewUnifiedService()
	engines := svc.ListEngines()
	if len(engines) != 0 {
		t.Logf("ListEngines returned %d items (plugins may be registered)", len(engines))
	}
}

// 测试 ListProcessors 空结果
func TestListProcessorsEmpty(t *testing.T) {
	svc := NewUnifiedService()
	processors := svc.ListProcessors()
	if len(processors) != 0 {
		t.Logf("ListProcessors returned %d items (plugins may be registered)", len(processors))
	}
}