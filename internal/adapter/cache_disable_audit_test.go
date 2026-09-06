package adapter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/utils"
)

type disableAuditCache struct {
	*utils.MemoryCache
	reads, writes int
}

func (c *disableAuditCache) Get(k string) ([]model.UnifiedAsset, bool) {
	c.reads++
	return c.MemoryCache.Get(k)
}
func (c *disableAuditCache) Set(k string, v []model.UnifiedAsset, d time.Duration) {
	c.writes++
	c.MemoryCache.Set(k, v, d)
}

func TestAuditEngineCacheDisabled(t *testing.T) {
	for _, mode := range []string{"enabled_control", "disabled_cold", "disabled_warm"} {
		for _, paged := range []bool{false, true} {
			name := mode + "/single"
			if paged {
				name = mode + "/paged"
			}
			t.Run(name, func(t *testing.T) {
				o := NewEngineOrchestrator()
				o.cache.Close()
				c := &disableAuditCache{MemoryCache: utils.NewMemoryCache(2, 0)}
				o.cache = c
				defer c.Close()
				a := &queryIdentityAdapter{mockAdapter: mockAdapter{name: "fixture"}}
				o.RegisterAdapter(a)
				enabled := mode == "enabled_control"
				o.SetEngineCacheTTL("fixture", time.Hour, enabled)
				query := model.EngineQuery{EngineName: "fixture", Query: `title="fixture"`, Page: 1}
				if mode == "disabled_warm" {
					c.MemoryCache.Set(utils.GenerateCacheKey("fixture", query.Query, 1, 10), []model.UnifiedAsset{{Title: "stale"}}, time.Hour)
				}
				for i := 0; i < 2; i++ {
					ch := make(chan *model.EngineResult, 1)
					if paged {
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
					result := <-ch
					if !enabled && result.Cached {
						t.Error("disabled cache returned cached response")
					}
					if len(result.NormalizedData) != 1 || result.NormalizedData[0].Title != query.Query {
						t.Errorf("wrong assets: %+v", result.NormalizedData)
					}
				}
				if enabled {
					if a.calls != 1 || c.reads != 2 || c.writes != 1 {
						t.Fatalf("control calls=%d reads=%d writes=%d", a.calls, c.reads, c.writes)
					}
				} else {
					if a.calls != 2 || c.reads != 0 || c.writes != 0 {
						t.Fatalf("disabled calls=%d reads=%d writes=%d", a.calls, c.reads, c.writes)
					}
				}
			})
		}
	}
}

func (c *disableAuditCache) GetQuerySnapshot(k string) ([]model.UnifiedAsset, utils.QueryCacheMetadata, bool) {
	c.reads++
	return c.MemoryCache.GetQuerySnapshot(k)
}
func (c *disableAuditCache) SetQuerySnapshot(k string, v []model.UnifiedAsset, m utils.QueryCacheMetadata, d time.Duration) {
	c.writes++
	c.MemoryCache.SetQuerySnapshot(k, v, m, d)
}
