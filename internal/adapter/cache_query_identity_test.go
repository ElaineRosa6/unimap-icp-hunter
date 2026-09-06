package adapter

import (
	"context"
	"sync"
	"testing"

	"github.com/unimap/project/internal/model"
)

type queryIdentityAdapter struct {
	mockAdapter
	calls int
}

func (a *queryIdentityAdapter) Search(_ context.Context, query string, page, _ int) (*model.EngineResult, error) {
	a.calls++
	return &model.EngineResult{EngineName: a.name, Page: page, Total: 1, NormalizedData: []model.UnifiedAsset{{Title: query}}}, nil
}
func (a *queryIdentityAdapter) Normalize(r *model.EngineResult) ([]model.UnifiedAsset, error) {
	return r.NormalizedData, nil
}

func TestSearchCachePreservesQueryIdentity(t *testing.T) {
	for _, paginated := range []bool{false, true} {
		name := "single"
		if paginated {
			name = "paginated"
		}
		t.Run(name, func(t *testing.T) {
			o := NewEngineOrchestrator()
			defer o.cache.Close()
			a := &queryIdentityAdapter{mockAdapter: mockAdapter{name: "fixture"}}
			o.RegisterAdapter(a)
			queries := []string{`title="Admin"`, `title="admin"`, `body="hello  world"`, `body="hello world"`}
			for pass := 0; pass < 2; pass++ {
				for _, q := range queries {
					ch := make(chan *model.EngineResult, 1)
					query := model.EngineQuery{EngineName: "fixture", Query: q, Page: 1}
					if paginated {
						task := &PaginatedSearchTask{orchestrator: o, ctx: context.Background(), query: query, pageSize: 10, maxPages: 1, resultChan: ch}
						task.fetchPaginatedPage(a, 1)
					} else {
						var wg sync.WaitGroup
						wg.Add(1)
						task := &SearchTask{orchestrator: o, ctx: context.Background(), query: query, pageSize: 10, resultChan: ch, errorChan: make(chan error, 1), wg: &wg}
						if err := task.Execute(); err != nil {
							t.Fatal(err)
						}
						wg.Wait()
					}
					select {
					case result := <-ch:
						if result.Cached != (pass == 1) {
							t.Errorf("query %q pass %d cached=%v", q, pass, result.Cached)
						}
						if len(result.NormalizedData) != 1 || result.NormalizedData[0].Title != q {
							t.Errorf("query %q got assets %+v", q, result.NormalizedData)
						}
					default:
						t.Fatal("missing result")
					}
				}
			}
			if a.calls != len(queries) {
				t.Errorf("adapter calls=%d want %d", a.calls, len(queries))
			}
		})
	}
}
