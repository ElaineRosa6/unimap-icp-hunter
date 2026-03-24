package adapter

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/model"
)

type mockAdapter struct {
	name      string
	translate string
	err       error
	searchErr error
	results   []model.UnifiedAsset
}

func (m *mockAdapter) Name() string { return m.name }

func (m *mockAdapter) Translate(ast *model.UQLAST) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.translate, nil
}

func (m *mockAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return &model.EngineResult{
		EngineName: m.name,
		Total:      len(m.results),
		Page:       page,
		HasMore:    false,
	}, nil
}

func (m *mockAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func (m *mockAdapter) GetQuota() (*model.QuotaInfo, error) {
	return &model.QuotaInfo{}, nil
}

func (m *mockAdapter) IsWebOnly() bool { return false }

// 表驱动测试：TranslateQuery
func TestTranslateQuery(t *testing.T) {
	tests := []struct {
		name          string
		adapters      []EngineAdapter
		engineNames   []string
		wantErr       bool
		wantCount     int
		wantErrCount  int
	}{
		{
			name:          "empty engines",
			adapters:      nil,
			engineNames:   []string{},
			wantErr:       true,
			wantCount:     0,
			wantErrCount:  0,
		},
		{
			name:          "single adapter success",
			adapters:      []EngineAdapter{&mockAdapter{name: "fofa", translate: "fofa_query"}},
			engineNames:   []string{"fofa"},
			wantErr:       false,
			wantCount:     1,
			wantErrCount:  0,
		},
		{
			name: "multiple adapters partial success",
			adapters: []EngineAdapter{
				&mockAdapter{name: "fofa", translate: "fofa_query"},
				&mockAdapter{name: "hunter", err: fmt.Errorf("translate error")},
			},
			engineNames:  []string{"fofa", "hunter"},
			wantErr:      false,
			wantCount:    1,
			wantErrCount: 1,
		},
		{
			name: "all adapters fail",
			adapters: []EngineAdapter{
				&mockAdapter{name: "fofa", err: fmt.Errorf("error1")},
				&mockAdapter{name: "hunter", err: fmt.Errorf("error2")},
			},
			engineNames:  []string{"fofa", "hunter"},
			wantErr:      true,
			wantCount:    0,
			wantErrCount: 2,
		},
		{
			name:          "missing adapter",
			adapters:      []EngineAdapter{&mockAdapter{name: "fofa", translate: "fofa_query"}},
			engineNames:   []string{"fofa", "missing"},
			wantErr:       false,
			wantCount:     1,
			wantErrCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewEngineOrchestrator()
			for _, a := range tt.adapters {
				o.RegisterAdapter(a)
			}

			ast := &model.UQLAST{Root: &model.UQLNode{Type: "condition", Value: "test"}}
			queries, err := o.TranslateQuery(ast, tt.engineNames)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(queries) != tt.wantCount {
					t.Errorf("expected %d queries, got %d", tt.wantCount, len(queries))
				}
			}
		})
	}
}

func TestTranslateQueryPartialFailure(t *testing.T) {
	o := NewEngineOrchestrator()
	o.RegisterAdapter(&mockAdapter{name: "ok", translate: "ok_query"})
	o.RegisterAdapter(&mockAdapter{name: "bad", err: fmt.Errorf("translate failed")})

	ast := &model.UQLAST{Root: &model.UQLNode{Type: "condition", Value: "country"}}
	queries, err := o.TranslateQuery(ast, []string{"ok", "bad", "missing"})
	if err != nil {
		t.Fatalf("expected partial success, got error: %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("expected 1 successful query, got %d", len(queries))
	}
	if queries[0].EngineName != "ok" || queries[0].Query != "ok_query" {
		t.Fatalf("unexpected query result: %+v", queries[0])
	}
}

func TestTranslateQueryAllFail(t *testing.T) {
	o := NewEngineOrchestrator()
	o.RegisterAdapter(&mockAdapter{name: "bad", err: fmt.Errorf("translate failed")})

	ast := &model.UQLAST{Root: &model.UQLNode{Type: "condition", Value: "country"}}
	_, err := o.TranslateQuery(ast, []string{"bad", "missing"})
	if err == nil {
		t.Fatalf("expected error when all engines fail")
	}
}

// 表驱动测试：SetConcurrency
func TestSetConcurrency(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"zero defaults", 0, DefaultConcurrency},
		{"negative defaults", -5, DefaultConcurrency},
		{"normal value", 5, 5},
		{"at max", MaxConcurrency, MaxConcurrency},
		{"over max caps", MaxConcurrency + 10, MaxConcurrency},
		{"one", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewEngineOrchestrator()
			o.SetConcurrency(tt.input)
			if o.concurrency != tt.expected {
				t.Errorf("expected concurrency %d, got %d", tt.expected, o.concurrency)
			}
		})
	}
}

func TestSetConcurrencyBounds(t *testing.T) {
	o := NewEngineOrchestrator()

	o.SetConcurrency(0)
	if o.concurrency != DefaultConcurrency {
		t.Fatalf("expected default concurrency %d, got %d", DefaultConcurrency, o.concurrency)
	}

	o.SetConcurrency(MaxConcurrency + 10)
	if o.concurrency != MaxConcurrency {
		t.Fatalf("expected capped concurrency %d, got %d", MaxConcurrency, o.concurrency)
	}
}

// 测试 ListAdapters
func TestListAdapters(t *testing.T) {
	o := NewEngineOrchestrator()

	// 空
	if len(o.ListAdapters()) != 0 {
		t.Error("expected empty list")
	}

	// 添加适配器
	o.RegisterAdapter(&mockAdapter{name: "fofa"})
	o.RegisterAdapter(&mockAdapter{name: "hunter"})

	names := o.ListAdapters()
	if len(names) != 2 {
		t.Errorf("expected 2 adapters, got %d", len(names))
	}
}

// 测试 GetAdapter
func TestGetAdapter(t *testing.T) {
	o := NewEngineOrchestrator()
	o.RegisterAdapter(&mockAdapter{name: "fofa"})

	adapter, exists := o.GetAdapter("fofa")
	if !exists || adapter.Name() != "fofa" {
		t.Error("expected to find fofa adapter")
	}

	_, exists = o.GetAdapter("missing")
	if exists {
		t.Error("expected not to find missing adapter")
	}
}

// 测试并发安全
func TestOrchestratorConcurrentAccess(t *testing.T) {
	o := NewEngineOrchestrator()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("engine_%d", i%10)
			o.RegisterAdapter(&mockAdapter{name: name})
			o.GetAdapter(name)
			o.ListAdapters()
		}(i)
	}
	wg.Wait()
}

// 测试 SearchEnginesWithContext 取消
func TestSearchEnginesCancellation(t *testing.T) {
	o := NewEngineOrchestrator()
	o.RegisterAdapter(&mockAdapter{name: "fofa", translate: "test"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	queries := []model.EngineQuery{{EngineName: "fofa", Query: "test"}}
	_, err := o.SearchEnginesWithContext(ctx, queries, 10)

	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

// 测试 NormalizeResults
func TestNormalizeResults(t *testing.T) {
	tests := []struct {
		name     string
		results  []*model.EngineResult
		adapters []EngineAdapter
		wantLen  int
	}{
		{
			name:     "empty results",
			results:  nil,
			adapters: nil,
			wantLen:  0,
		},
		{
			name:     "nil result",
			results:  []*model.EngineResult{nil},
			adapters: nil,
			wantLen:  0,
		},
		{
			name:     "result with error",
			results:  []*model.EngineResult{{EngineName: "fofa", Error: "some error"}},
			adapters: []EngineAdapter{&mockAdapter{name: "fofa"}},
			wantLen:  0,
		},
		{
			name: "cached normalized data",
			results: []*model.EngineResult{{
				EngineName:     "fofa",
				Cached:         true,
				NormalizedData: []model.UnifiedAsset{{IP: "1.2.3.4"}},
			}},
			adapters: nil,
			wantLen:  1,
		},
		{
			name: "successful normalize",
			results: []*model.EngineResult{{
				EngineName: "fofa",
			}},
			adapters: []EngineAdapter{&mockAdapter{name: "fofa", results: []model.UnifiedAsset{{IP: "1.1.1.1"}, {IP: "2.2.2.2"}}}},
			wantLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewEngineOrchestrator()
			for _, a := range tt.adapters {
				o.RegisterAdapter(a)
			}

			assets, err := o.NormalizeResults(tt.results)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(assets) != tt.wantLen {
				t.Errorf("expected %d assets, got %d", tt.wantLen, len(assets))
			}
		})
	}
}

// 测试 SearchEngines 空查询
func TestSearchEnginesEmptyQueries(t *testing.T) {
	o := NewEngineOrchestrator()

	_, err := o.SearchEngines(nil, 10)
	if err == nil {
		t.Error("expected error for empty queries")
	}

	_, err = o.SearchEngines([]model.EngineQuery{}, 10)
	if err == nil {
		t.Error("expected error for empty queries")
	}
}

// 测试 TranslateQuery 空 AST
func TestTranslateQueryNilAST(t *testing.T) {
	o := NewEngineOrchestrator()

	_, err := o.TranslateQuery(nil, []string{"fofa"})
	if err == nil {
		t.Error("expected error for nil AST")
	}
}

// 基准测试
func BenchmarkOrchestratorRegisterAdapter(b *testing.B) {
	o := NewEngineOrchestrator()
	for i := 0; i < b.N; i++ {
		o.RegisterAdapter(&mockAdapter{name: fmt.Sprintf("engine_%d", i%10)})
	}
}

func BenchmarkOrchestratorGetAdapter(b *testing.B) {
	o := NewEngineOrchestrator()
	o.RegisterAdapter(&mockAdapter{name: "fofa"})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o.GetAdapter("fofa")
	}
}
