package adapter

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

type fakeEngineAdapter struct {
	name        string
	mu          sync.Mutex
	searchCalls int
}

func (f *fakeEngineAdapter) Name() string {
	return f.name
}

func (f *fakeEngineAdapter) Translate(ast *model.UQLAST) (string, error) {
	return "q", nil
}

func (f *fakeEngineAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	f.mu.Lock()
	f.searchCalls++
	f.mu.Unlock()

	return &model.EngineResult{
		EngineName: f.name,
		RawData:    []interface{}{},
		Total:      0,
		Page:       page,
		HasMore:    false,
	}, nil
}

func (f *fakeEngineAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
	return []model.UnifiedAsset{}, nil
}

func (f *fakeEngineAdapter) GetQuota() (*model.QuotaInfo, error) {
	return &model.QuotaInfo{}, nil
}

func (f *fakeEngineAdapter) SearchCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.searchCalls
}

func TestSearchEnginesWithPagination_CacheHit(t *testing.T) {
	orchestrator := NewEngineOrchestrator()
	adapter := &fakeEngineAdapter{name: "fofa"}
	orchestrator.RegisterAdapter(adapter)

	query := model.EngineQuery{EngineName: "fofa", Query: "title=admin"}
	pageSize := 2
	maxPages := 2

	for page := 1; page <= maxPages; page++ {
		assets := []model.UnifiedAsset{{
			IP:     fmt.Sprintf("1.1.1.%d", page),
			Port:   80,
			Source: "fofa",
		}}
		key := utils.GenerateCacheKey(adapter.name, query.Query, page, pageSize)
		orchestrator.cache.Set(key, assets, time.Minute)
	}

	results, err := orchestrator.SearchEnginesWithPagination([]model.EngineQuery{query}, pageSize, maxPages)
	if err != nil {
		t.Fatalf("SearchEnginesWithPagination() error = %v", err)
	}

	if adapter.SearchCallCount() != 0 {
		t.Fatalf("Search() called %d times, want 0", adapter.SearchCallCount())
	}

	if len(results) != maxPages {
		t.Fatalf("results = %d, want %d", len(results), maxPages)
	}

	byPage := make(map[int]*model.EngineResult)
	for _, result := range results {
		if result == nil {
			continue
		}
		byPage[result.Page] = result
	}

	for page := 1; page <= maxPages; page++ {
		result, ok := byPage[page]
		if !ok {
			t.Fatalf("missing result for page %d", page)
		}
		if !result.Cached {
			t.Fatalf("page %d cached = false, want true", page)
		}
		if len(result.NormalizedData) != 1 {
			t.Fatalf("page %d normalized = %d, want 1", page, len(result.NormalizedData))
		}
		if result.HasMore != (page < maxPages) {
			t.Fatalf("page %d hasMore = %v, want %v", page, result.HasMore, page < maxPages)
		}
	}
}
