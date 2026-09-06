package adapter

import (
	"context"
	"sync"
	"testing"

	"github.com/unimap/project/internal/model"
)

type pageAuditAdapter struct {
	mockAdapter
	calls int
	more  bool
}

func (a *pageAuditAdapter) Search(_ context.Context, _ string, page, _ int) (*model.EngineResult, error) {
	a.calls++
	return &model.EngineResult{EngineName: a.name, Page: page, Total: 37, HasMore: a.more, NormalizedData: []model.UnifiedAsset{{Title: "fixture"}}}, nil
}
func (a *pageAuditAdapter) Normalize(r *model.EngineResult) ([]model.UnifiedAsset, error) {
	return r.NormalizedData, nil
}

func TestAuditSinglePageCacheSemantics(t *testing.T) {
	o := NewEngineOrchestrator()
	defer o.cache.Close()
	a := &pageAuditAdapter{mockAdapter: mockAdapter{name: "fixture"}, more: true}
	o.RegisterAdapter(a)
	var first *model.EngineResult
	for pass := 0; pass < 2; pass++ {
		ch := make(chan *model.EngineResult, 1)
		var wg sync.WaitGroup
		wg.Add(1)
		task := &SearchTask{orchestrator: o, ctx: context.Background(), query: model.EngineQuery{EngineName: "fixture", Query: "fixture", Page: 3}, pageSize: 10, resultChan: ch, errorChan: make(chan error, 1), wg: &wg}
		if err := task.Execute(); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		result := <-ch
		if pass == 0 {
			first = result
			if result.Page != 3 || result.Total != 37 || !result.HasMore {
				t.Fatal("fixture fresh result invalid")
			}
		} else {
			if !result.Cached {
				t.Fatal("expected cache hit")
			}
			if result.Page != first.Page || result.Total != first.Total || result.HasMore != first.HasMore {
				t.Fatalf("fresh page/total/more=%d/%d/%v cached=%d/%d/%v", first.Page, first.Total, first.HasMore, result.Page, result.Total, result.HasMore)
			}
		}
	}
	if a.calls != 1 {
		t.Fatalf("calls=%d want 1", a.calls)
	}
}

func TestAuditPaginatedCacheTerminalPage(t *testing.T) {
	o := NewEngineOrchestrator()
	defer o.cache.Close()
	a := &pageAuditAdapter{mockAdapter: mockAdapter{name: "fixture"}, more: false}
	o.RegisterAdapter(a)
	for pass := 0; pass < 2; pass++ {
		ch := make(chan *model.EngineResult, 1)
		task := &PaginatedSearchTask{orchestrator: o, ctx: context.Background(), query: model.EngineQuery{EngineName: "fixture", Query: "fixture"}, pageSize: 10, maxPages: 3, resultChan: ch}
		stop := task.fetchPaginatedPage(a, 1)
		result := <-ch
		if result.Cached != (pass == 1) {
			t.Fatalf("cached=%v pass=%d", result.Cached, pass)
		}
		if !stop || result.HasMore || result.Total != 37 {
			t.Errorf("terminal page pass=%d stop=%v more=%v total=%d", pass, stop, result.HasMore, result.Total)
		}
	}
	if a.calls != 1 {
		t.Fatalf("calls=%d want 1", a.calls)
	}
}

func TestCachedPaginationRespectsUpstreamAndLimit(t *testing.T) {
	for _, more := range []bool{false, true} {
		for _, limit := range []int{1, 3} {
			o := NewEngineOrchestrator()
			a := &pageAuditAdapter{mockAdapter: mockAdapter{name: "fixture"}, more: more}
			o.RegisterAdapter(a)
			for pass := 0; pass < 2; pass++ {
				ch := make(chan *model.EngineResult, 1)
				task := &PaginatedSearchTask{orchestrator: o, ctx: context.Background(), query: model.EngineQuery{EngineName: "fixture", Query: "fixture"}, pageSize: 10, maxPages: limit, resultChan: ch}
				stop := task.fetchPaginatedPage(a, 1)
				result := <-ch
				if stop != (!more || limit == 1) || result.HasMore != more || result.Total != 37 {
					t.Errorf("pass=%d limit=%d upstreamMore=%v stop=%v result=%+v", pass, limit, more, stop, result)
				}
			}
			o.cache.Close()
		}
	}
}
